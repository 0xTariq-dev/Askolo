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
fixture_monitoring="$fixture_root/monitoring/askolo-backend"
fixture_bin="$fixture_root/bin"
sentinel="$fixture_root/build-started"

mkdir -p "$fixture_scripts" "$fixture_artifact" "$fixture_api" "$fixture_monitoring" "$fixture_bin"
cp "$repo_root/scripts/verify-publish-paths.sh" "$fixture_scripts/verify-publish-paths.sh"
cp "$repo_root/artifacts/personal-assistant/.replit-artifact/artifact.toml" \
  "$fixture_artifact/artifact.toml"
cp "$repo_root/services/askolo-backend/scripts/build.sh" "$fixture_api/build.sh"
cp "$repo_root/monitoring/askolo-backend/verify-mfa-recovery-alert-policy.sh" \
  "$fixture_monitoring/verify-mfa-recovery-alert-policy.sh"
cp "$repo_root/monitoring/askolo-backend/mfa-recovery-alerts.yaml" \
  "$fixture_monitoring/mfa-recovery-alerts.yaml"

# If either production build starts, leave a marker that the test can detect.
cat >"$fixture_bin/pnpm" <<'STUB'
#!/usr/bin/env bash
touch -- "${BUILD_SENTINEL:?}"
exit 99
STUB
chmod +x "$fixture_bin/pnpm"
cat >"$fixture_bin/go" <<'STUB'
#!/usr/bin/env bash
if [[ "${PUBLISH_ROUTE_REGISTRY_EXTRA-}" == "1" ]]; then
  printf '%s\n' 'paths = ["/api", "/healthz", "/readyz", "/ws", "/webhooks", "/metrics"]'
else
  printf '%s\n' 'paths = ["/api", "/healthz", "/readyz", "/ws", "/webhooks"]'
fi
STUB
chmod +x "$fixture_bin/go"
sed -i "1i touch -- \"\${BUILD_SENTINEL:?}\"" "$fixture_api/build.sh"

# Drift every publish contract at once. The validator must report all of them
# together and must not start either production build while doing so.
sed -i \
  -e 's#build = \[ "pnpm", "--filter", "@workspace/personal-assistant", "run", "build" \]#build = [ "pnpm", "--filter", "@workspace/personal-assistant", "run", "changed-build" ]#' \
  -e 's#publicDir = "artifacts/personal-assistant/dist/public"#publicDir = "artifacts/personal-assistant/dist/changed-public"#' \
  -e 's#build = \["bash", "services/askolo-backend/scripts/build.sh"\]#build = ["bash", "services/askolo-backend/scripts/changed-build.sh"]#' \
  "$fixture_artifact/artifact.toml"

set +e
output="$(
  PATH="$fixture_bin:$PATH" BUILD_SENTINEL="$sentinel" PUBLISH_ROUTE_REGISTRY_EXTRA=1 \
    bash "$fixture_scripts/verify-publish-paths.sh" 2>&1
)"
status=$?
set -e

[[ "$status" -ne 0 ]] ||
  { echo "expected an unpublished backend route to fail validation" >&2; exit 1; }
grep -Fq \
  "frontend production build command changed in artifacts/personal-assistant/.replit-artifact/artifact.toml; expected: build = [ \"pnpm\", \"--filter\", \"@workspace/personal-assistant\", \"run\", \"build\" ]" \
  <<<"$output" ||
  { echo "validation failure did not identify the changed frontend build contract:" >&2; echo "$output" >&2; exit 1; }
grep -Fq \
  "frontend production public directory changed in artifacts/personal-assistant/.replit-artifact/artifact.toml; expected: publicDir = \"artifacts/personal-assistant/dist/public\"" \
  <<<"$output" ||
  { echo "validation failure did not identify the changed frontend public directory contract:" >&2; echo "$output" >&2; exit 1; }
grep -Fq \
  "API production build command changed in artifacts/personal-assistant/.replit-artifact/artifact.toml; expected: build = [\"bash\", \"services/askolo-backend/scripts/build.sh\"]" \
  <<<"$output" ||
  { echo "validation failure did not identify the changed API build contract:" >&2; echo "$output" >&2; exit 1; }
grep -Fq \
  "backend route registry declares published path(s) missing from artifacts/personal-assistant/.replit-artifact/artifact.toml: /metrics; add them to the API service paths list" \
  <<<"$output" ||
  { echo "validation failure did not identify the unpublished backend route:" >&2; echo "$output" >&2; exit 1; }
[[ ! -e "$sentinel" ]] ||
  { echo "a production build started before the publish contracts failed" >&2; exit 1; }

# Change only the frontend production path in the temporary artifact config.
cp "$repo_root/artifacts/personal-assistant/.replit-artifact/artifact.toml" \
  "$fixture_artifact/artifact.toml"
sed -i 's#publicDir = "artifacts/personal-assistant/dist/public"#publicDir = "artifacts/personal-assistant/dist/changed-public"#' \
  "$fixture_artifact/artifact.toml"

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
  "backend route registry declares published path(s) missing from artifacts/personal-assistant/.replit-artifact/artifact.toml: /webhooks; add them to the API service paths list" \
  <<<"$output" ||
  { echo "validation failure did not identify the changed API route contract:" >&2; echo "$output" >&2; exit 1; }
grep -Fq \
  "artifact publishes path(s) absent from the backend route registry: /changed-webhooks; remove them or register the backend routes before publishing" \
  <<<"$output" ||
  { echo "validation failure did not identify the extra API route contract:" >&2; echo "$output" >&2; exit 1; }
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