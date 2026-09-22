#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/.." && pwd)"
fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/askolo-publish-paths.XXXXXX")"

cleanup() {
  rm -rf -- "$fixture_root"
}
trap cleanup EXIT INT TERM

before_status="$(git -C "$repo_root" status --porcelain=v1)"

fixture_scripts="$fixture_root/scripts"
fixture_artifact="$fixture_root/artifacts/personal-assistant/.replit-artifact"
fixture_api="$fixture_root/services/askolo-backend/scripts"
fixture_bin="$fixture_root/bin"
sentinel="$fixture_root/build-started"

mkdir -p "$fixture_scripts" "$fixture_artifact" "$fixture_api" "$fixture_bin"
cp "$repo_root/scripts/verify-publish-paths.sh" "$fixture_scripts/verify-publish-paths.sh"
cp "$repo_root/artifacts/personal-assistant/.replit-artifact/artifact.toml" \
  "$fixture_artifact/artifact.toml"
cp "$repo_root/services/askolo-backend/scripts/build.sh" "$fixture_api/build.sh"

# Change only the frontend production path in the temporary artifact config.
sed -i 's#publicDir = "artifacts/personal-assistant/dist/public"#publicDir = "artifacts/personal-assistant/dist/changed-public"#' \
  "$fixture_artifact/artifact.toml"

# If either production build starts, leave a marker that the test can detect.
cat >"$fixture_bin/pnpm" <<'STUB'
#!/usr/bin/env bash
touch -- "${BUILD_SENTINEL:?}"
exit 99
STUB
chmod +x "$fixture_bin/pnpm"
sed -i "1i touch -- \"\${BUILD_SENTINEL:?}\"" "$fixture_api/build.sh"

set +e
output="$(
  PATH="$fixture_bin:$PATH" BUILD_SENTINEL="$sentinel" \
    bash "$fixture_scripts/verify-publish-paths.sh" 2>&1
)"
status=$?
set -e

[[ "$status" -ne 0 ]] ||
  { echo "expected changed artifact path to fail validation" >&2; exit 1; }
grep -Fq \
  "frontend production public directory changed in artifacts/personal-assistant/.replit-artifact/artifact.toml; expected: publicDir = \"artifacts/personal-assistant/dist/public\"" \
  <<<"$output" ||
  { echo "validation failure did not identify the changed path contract:" >&2; echo "$output" >&2; exit 1; }
[[ ! -e "$sentinel" ]] ||
  { echo "a production build started before the path contract failed" >&2; exit 1; }

# Reset the temporary artifact config, then change only the API production
# paths so the API routing contract gets the same pre-build coverage.
rm -f -- "$sentinel"
cp "$repo_root/artifacts/personal-assistant/.replit-artifact/artifact.toml" \
  "$fixture_artifact/artifact.toml"
sed -i 's#paths = \["/api", "/healthz", "/readyz", "/ws", "/webhooks"\]#paths = ["/api", "/healthz", "/readyz", "/ws", "/changed-webhooks"]#' \
  "$fixture_artifact/artifact.toml"

set +e
output="$(
  PATH="$fixture_bin:$PATH" BUILD_SENTINEL="$sentinel" \
    bash "$fixture_scripts/verify-publish-paths.sh" 2>&1
)"
status=$?
set -e

[[ "$status" -ne 0 ]] ||
  { echo "expected changed API routing paths to fail validation" >&2; exit 1; }
grep -Fq \
  "published API routing paths changed in artifacts/personal-assistant/.replit-artifact/artifact.toml; expected: paths = [\"/api\", \"/healthz\", \"/readyz\", \"/ws\", \"/webhooks\"]" \
  <<<"$output" ||
  { echo "validation failure did not identify the changed API route contract:" >&2; echo "$output" >&2; exit 1; }
[[ ! -e "$sentinel" ]] ||
  { echo "a production build started before the API route contract failed" >&2; exit 1; }

# Invalid smoke-port overrides must fail before any production build starts.
rm -f -- "$sentinel"
cp "$repo_root/artifacts/personal-assistant/.replit-artifact/artifact.toml" \
  "$fixture_artifact/artifact.toml"

set +e
output="$(
  PATH="$fixture_bin:$PATH" BUILD_SENTINEL="$sentinel" \
    PUBLISH_SMOKE_FRONTEND_PORT=not-a-port \
    bash "$fixture_scripts/verify-publish-paths.sh" 2>&1
)"
status=$?
set -e

[[ "$status" -ne 0 ]] ||
  { echo "expected invalid smoke frontend port override to fail validation" >&2; exit 1; }
grep -Fq \
  'PUBLISH_SMOKE_FRONTEND_PORT must be a port number between 1 and 65535; received "not-a-port"' \
  <<<"$output" ||
  { echo "invalid smoke frontend port override was not identified:" >&2; echo "$output" >&2; exit 1; }
[[ ! -e "$sentinel" ]] ||
  { echo "a production build started before the invalid smoke port override failed" >&2; exit 1; }

after_status="$(git -C "$repo_root" status --porcelain=v1)"
[[ "$before_status" == "$after_status" ]] ||
  { echo "publish path fixture changed the working tree" >&2; git -C "$repo_root" status --short >&2; exit 1; }

echo "Publish path drift regression check passed"