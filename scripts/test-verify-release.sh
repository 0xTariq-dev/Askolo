#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/.." && pwd)"
fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/askolo-release-verification.XXXXXX")"

cleanup() {
  rm -rf -- "$fixture_root"
}
trap cleanup EXIT INT TERM

fixture_scripts="$fixture_root/scripts"
fixture_bin="$fixture_root/bin"
curl_log="$fixture_root/curl.log"
mkdir -p "$fixture_scripts" "$fixture_bin"
cp "$repo_root/scripts/verify-release.sh" "$fixture_scripts/verify-release.sh"

cat >"$fixture_bin/curl" <<'STUB'
#!/usr/bin/env bash
set -Eeuo pipefail

output=""
url=""
while (($# > 0)); do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    --*)
      shift
      ;;
    *)
      url="$1"
      shift
      ;;
  esac
done

printf '%s\n' "$url" >>"${CURL_LOG:?}"
if [[ "$url" == "${FAIL_URL:-}" ]]; then
  exit 22
fi

case "$url" in
  https://staging.example/)
    printf '<!doctype html><html></html>\n' >"$output"
    ;;
  https://staging.example/dashboard)
    printf '<!doctype html><html><div id="app"></div></html>\n' >"$output"
    ;;
  https://staging.example/api/healthz)
    printf '{"status":"ok"}\n' >"$output"
    ;;
  *)
    exit 22
    ;;
esac
STUB
chmod +x "$fixture_bin/curl"

run_release_verification() {
  (
    cd "$fixture_root"
    PATH="$fixture_bin:$PATH" \
      CURL_LOG="$curl_log" \
      ASKOLO_ENVIRONMENT=staging \
      ASKOLO_CANONICAL_ORIGIN=https://staging.example/ \
      ASKOLO_DATABASE_ID=staging-database \
      ASKOLO_COOKIE_NAMESPACE=askolo_staging \
      ASKOLO_COMMIT_SHA=fixture-commit \
      ASKOLO_RELEASE_TAG=fixture-release \
      ASKOLO_INTERNAL_TOKEN=fixture-token \
      bash "$fixture_scripts/verify-release.sh" "$@"
  )
}

output="$(run_release_verification)"
grep -Fq "published path verified: /" <<<"$output" ||
  { echo "release verification did not check the published root path" >&2; exit 1; }
grep -Fq "published path verified: /dashboard" <<<"$output" ||
  { echo "release verification did not check the client-side route" >&2; exit 1; }
grep -Fq "published path verified: /api/healthz" <<<"$output" ||
  { echo "release verification did not check the API health endpoint" >&2; exit 1; }

expected_urls=$'https://staging.example/\nhttps://staging.example/dashboard\nhttps://staging.example/api/healthz'
[[ "$(cat "$curl_log")" == "$expected_urls" ]] ||
  { echo "release verification did not use the configured deployment origin:" >&2; cat "$curl_log" >&2; exit 1; }

set +e
failure_output="$(
  FAIL_URL=https://staging.example/dashboard \
    run_release_verification 2>&1
)"
failure_status=$?
set -e

[[ "$failure_status" -ne 0 ]] ||
  { echo "expected a failed published route check" >&2; exit 1; }
grep -Fq \
  "published client-side route failed at https://staging.example/dashboard" \
  <<<"$failure_output" ||
  { echo "published route failure did not identify the broken path:" >&2; echo "$failure_output" >&2; exit 1; }

echo "Release routing verification regression check passed"