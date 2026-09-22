#!/usr/bin/env bash
set -Eeuo pipefail

# Run the same production build commands that the artifact publisher runs,
# from the repository root. Keep the contract checks before the builds so a
# path-only artifact change fails with an actionable message.

die() {
  echo "publish path verification failed: $*" >&2
  exit 1
}

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/.." && pwd)"
artifact_config="$repo_root/artifacts/personal-assistant/.replit-artifact/artifact.toml"
api_build_script="$repo_root/services/askolo-backend/scripts/build.sh"

[[ -f "$artifact_config" ]] ||
  die "artifact config not found at artifacts/personal-assistant/.replit-artifact/artifact.toml"
[[ -f "$api_build_script" ]] ||
  die "API build script not found at services/askolo-backend/scripts/build.sh"

require_artifact_line() {
  local expected="$1"
  local description="$2"

  grep -Fqx "$expected" "$artifact_config" ||
    die "$description changed in artifacts/personal-assistant/.replit-artifact/artifact.toml; expected: $expected"
}

# These values are consumed from the publishing repository root. A changed
# path can otherwise leave local development healthy while publishing fails.
require_artifact_line \
  'build = [ "pnpm", "--filter", "@workspace/personal-assistant", "run", "build" ]' \
  "frontend production build command"
require_artifact_line \
  'publicDir = "artifacts/personal-assistant/dist/public"' \
  "frontend production public directory"
require_artifact_line \
  'build = ["bash", "services/askolo-backend/scripts/build.sh"]' \
  "API production build command"

line_number() {
  local needle="$1"
  local line
  line="$(grep -nF "$needle" "$api_build_script" | head -n 1 | cut -d: -f1 || true)"
  [[ -n "$line" ]] || die "API build script no longer contains: $needle"
  printf '%s\n' "$line"
}

script_dir_line="$(line_number 'script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"')"
service_root_line="$(line_number 'service_root="$(cd -- "$script_dir/.." && pwd)"')"
cd_root_line="$(line_number 'cd -- "$service_root"')"
go_test_line="$(line_number 'go test ./...')"
go_vet_line="$(line_number 'go vet ./...')"
go_build_line="$(line_number 'go build -trimpath -o "$tmp_binary" ./cmd/askolo-backend')"

(( script_dir_line < service_root_line )) ||
  die "API build script must resolve script_dir before service_root"
(( service_root_line < cd_root_line )) ||
  die "API build script must resolve service_root before changing directories"
(( cd_root_line < go_test_line && cd_root_line < go_vet_line && cd_root_line < go_build_line )) ||
  die "API build script must enter its module root before running Go commands"

cd -- "$repo_root"

echo "Verifying frontend production build from repository root"
PORT=18131 BASE_PATH=/ pnpm --filter @workspace/personal-assistant run build

echo "Verifying API production build from repository root"
bash services/askolo-backend/scripts/build.sh

echo "Publish path verification passed"