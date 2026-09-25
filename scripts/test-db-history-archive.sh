#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

manifest="scripts/database-history-archive.manifest"
archive_ref="$(sed -n 's/^archive_ref=//p' "$manifest")"
archive_commit="$(sed -n 's/^archive_commit=//p' "$manifest")"
zero_sha='0000000000000000000000000000000000000000'
current_sha="$(git rev-parse HEAD)"
hook=".githooks/pre-push"

bash scripts/verify-db-history-archive.sh "$archive_commit"

assert_archive_push_blocked() {
  local label="$1"
  local local_sha="$2"
  local remote_sha="$3"
  local output

  if output="$(printf '%s %s %s %s\n' \
    refs/heads/local-test "$local_sha" "$archive_ref" "$remote_sha" |
    "$hook" 2>&1)"; then
    echo "expected the pre-push hook to reject archive $label" >&2
    exit 1
  fi
  if [[ "$output" != *"protected database history archive"* ]]; then
    echo "archive $label failed for an unexpected reason:" >&2
    printf '%s\n' "$output" >&2
    exit 1
  fi
}

assert_archive_push_blocked update "$current_sha" "$archive_commit"
assert_archive_push_blocked deletion "$zero_sha" "$archive_commit"

printf '%s %s %s %s\n' \
  refs/heads/local-test "$current_sha" refs/heads/main "$current_sha" |
  "$hook"

echo "Database-history archive verification and pre-push protection checks passed."