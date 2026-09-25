#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel 2>/dev/null)" || {
  echo "archive verification must run inside a Git repository" >&2
  exit 2
}
cd "$repo_root"

manifest="scripts/database-history-archive.manifest"
if [[ ! -f "$manifest" ]]; then
  echo "archive manifest is missing: $manifest" >&2
  exit 2
fi

archive_ref="$(sed -n 's/^archive_ref=//p' "$manifest")"
expected_commit="$(sed -n 's/^archive_commit=//p' "$manifest")"
expected_ruleset="$(sed -n 's/^ruleset_id=//p' "$manifest")"
if [[ -z "$archive_ref" || -z "$expected_commit" || -z "$expected_ruleset" ]]; then
  echo "archive manifest is missing required metadata" >&2
  exit 2
fi

if (( $# > 1 )); then
  echo "usage: bash scripts/verify-db-history-archive.sh [fetched-ref]" >&2
  exit 2
fi
ref="${1:-$archive_ref}"
commit="$(git rev-parse --verify "${ref}^{commit}" 2>/dev/null)" || {
  echo "cannot resolve archive ref '$ref'; fetch $archive_ref first" >&2
  exit 1
}
if [[ "$commit" != "$expected_commit" ]]; then
  echo "archive ref '$ref' points to $commit, expected $expected_commit" >&2
  exit 1
fi

expected="$(grep -E '^[0-9a-f]{40} ' "$manifest" | LC_ALL=C sort)"
if [[ -z "$expected" ]]; then
  echo "archive manifest contains no file inventory" >&2
  exit 2
fi
actual="$(
  git ls-tree -r --format='%(objectname) %(path)' "$commit" \
    -- lib/db/drizzle lib/db/src/schema |
    LC_ALL=C sort
)"
if [[ "$actual" != "$expected" ]]; then
  echo "archive inventory differs from the verified manifest:" >&2
  diff -u <(printf '%s\n' "$expected") <(printf '%s\n' "$actual") >&2 || true
  exit 1
fi

file_count="$(printf '%s\n' "$expected" | wc -l | tr -d ' ')"
echo "Verified $file_count database-history files at $archive_ref ($commit); ruleset $expected_ruleset."