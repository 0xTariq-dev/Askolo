#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cleanup_script="$script_dir/drop-published-app-commits.sh"
fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/askolo-publish-cleanup.XXXXXX")"

cleanup() {
  rm -rf -- "$fixture_root"
}
trap cleanup EXIT INT TERM

fail() {
  echo "publish cleanup regression: $*" >&2
  exit 1
}

assert_contains() {
  local haystack="$1"
  local needle="$2"

  grep -Fq -- "$needle" <<<"$haystack" ||
    {
      echo "expected output to contain: $needle" >&2
      echo "$haystack" >&2
      exit 1
    }
}

assert_refs_unchanged() {
  local repo="$1"
  local before="$2"
  local after

  after="$(git -C "$repo" for-each-ref \
    --format='%(refname) %(objectname)' refs/heads refs/remotes)"
  [[ "$before" == "$after" ]] ||
    {
      echo "dry-run changed local branch or remote-tracking refs" >&2
      diff -u <(printf '%s\n' "$before") <(printf '%s\n' "$after") >&2 || true
      exit 1
    }
}

init_repo() {
  local repo="$1"

  git init -q -b main "$repo"
  git -C "$repo" config user.name "Publish cleanup test"
  git -C "$repo" config user.email "publish-cleanup-test@example.invalid"
  printf 'fixture\n' >"$repo/fixture.txt"
  git -C "$repo" add fixture.txt
  git -C "$repo" commit -q -m "Base commit"
}

add_candidate_commit() {
  local repo="$1"

  git -C "$repo" commit --allow-empty -q -m "Published your App"
}

setup_remote_fixture() {
  local name="$1"
  local case_root="$fixture_root/$name"
  local repo="$case_root/repo"
  local remote="$case_root/remote.git"

  mkdir -p "$case_root"
  git init -q --bare "$remote"
  init_repo "$repo"
  git -C "$repo" remote add origin "$remote"
  git -C "$repo" push -q -u origin main
  add_candidate_commit "$repo"

  printf '%s\n' "$repo"
}

run_dry_run() {
  local repo="$1"

  set +e
  test_output="$(
    cd "$repo"
    printf '1\n' |
      NO_COLOR=1 bash "$cleanup_script" \
        --dry-run --check-upstream --pattern '^Published your App$' \
        2>&1
  )"
  test_status=$?
  set -e
}

run_apply_check() {
  local repo="$1"

  set +e
  test_output="$(
    cd "$repo"
    NO_COLOR=1 bash "$cleanup_script" \
      --apply --check-upstream --pattern '^Published your App$' \
      </dev/null 2>&1
  )"
  test_status=$?
  set -e
}

test_matching_tips() {
  local repo
  local before_refs

  repo="$(setup_remote_fixture matching-tips)"
  before_refs="$(git -C "$repo" for-each-ref \
    --format='%(refname) %(objectname)' refs/heads refs/remotes)"

  run_dry_run "$repo"
  [[ "$test_status" -eq 0 ]] ||
    fail "matching tips dry run failed:
$test_output"
  assert_contains "$test_output" \
    "Upstream freshness check passed: 'refs/remotes/origin/main' matches the current 'origin' tip."
  assert_contains "$test_output" "Dry run complete."
  assert_refs_unchanged "$repo" "$before_refs"
}

test_remote_ahead_tips() {
  local case_root="$fixture_root/remote-ahead-tips"
  local repo
  local remote_clone="$case_root/remote-clone"
  local before_refs

  repo="$(setup_remote_fixture remote-ahead-tips)"
  git clone -q --branch main "$case_root/remote.git" "$remote_clone"
  git -C "$remote_clone" config user.name "Publish cleanup remote test"
  git -C "$remote_clone" config user.email "publish-cleanup-remote@example.invalid"
  git -C "$remote_clone" commit --allow-empty -q -m "Remote-only commit"
  git -C "$remote_clone" push -q origin main

  before_refs="$(git -C "$repo" for-each-ref \
    --format='%(refname) %(objectname)' refs/heads refs/remotes)"
  run_dry_run "$repo"
  [[ "$test_status" -eq 0 ]] ||
    fail "remote-ahead dry run failed:
$test_output"
  assert_contains "$test_output" \
    "configured upstream 'refs/remotes/origin/main' may be stale."
  assert_contains "$test_output" \
    "Refresh it with 'git fetch origin', then retry with --check-upstream."
  assert_contains "$test_output" "Dry run complete."
  assert_refs_unchanged "$repo" "$before_refs"

  run_apply_check "$repo"
  [[ "$test_status" -ne 0 ]] ||
    fail "apply continued after remote-ahead freshness check failed"
  assert_contains "$test_output" \
    "refusing --apply while the configured upstream may be stale"
  assert_refs_unchanged "$repo" "$before_refs"
}

test_unavailable_remote() {
  local case_root="$fixture_root/unavailable-remote"
  local repo
  local before_refs

  repo="$(setup_remote_fixture unavailable-remote)"
  rm -rf -- "$case_root/remote.git"
  before_refs="$(git -C "$repo" for-each-ref \
    --format='%(refname) %(objectname)' refs/heads refs/remotes)"

  run_dry_run "$repo"
  [[ "$test_status" -eq 0 ]] ||
    fail "unavailable remote dry run failed:
$test_output"
  assert_contains "$test_output" "could not read remote tip 'origin/refs/heads/main'"
  assert_contains "$test_output" \
    "Refresh it with 'git fetch origin', then retry with --check-upstream."
  assert_contains "$test_output" "Dry run complete."
  assert_refs_unchanged "$repo" "$before_refs"

  run_apply_check "$repo"
  [[ "$test_status" -ne 0 ]] ||
    fail "apply continued when the remote was unavailable"
  assert_contains "$test_output" \
    "refusing --apply while the configured upstream may be stale"
  assert_refs_unchanged "$repo" "$before_refs"
}

test_local_only_upstream() {
  local case_root="$fixture_root/local-only-upstream"
  local repo="$case_root/repo"
  local before_refs

  mkdir -p "$case_root"
  init_repo "$repo"
  git -C "$repo" branch upstream
  git -C "$repo" config branch.main.remote .
  git -C "$repo" config branch.main.merge refs/heads/upstream
  add_candidate_commit "$repo"

  before_refs="$(git -C "$repo" for-each-ref \
    --format='%(refname) %(objectname)' refs/heads refs/remotes)"
  run_dry_run "$repo"
  [[ "$test_status" -eq 0 ]] ||
    fail "local-only upstream dry run failed:
$test_output"
  assert_contains "$test_output" \
    "Configure a remote-tracking upstream, then retry with --check-upstream."
  assert_contains "$test_output" "Dry run complete."
  assert_refs_unchanged "$repo" "$before_refs"

  run_apply_check "$repo"
  [[ "$test_status" -ne 0 ]] ||
    fail "apply continued with a local-only upstream"
  assert_contains "$test_output" \
    "refusing --apply while the configured upstream may be stale"
  assert_refs_unchanged "$repo" "$before_refs"
}

test_matching_tips
test_remote_ahead_tips
test_unavailable_remote
test_local_only_upstream

echo "Published app cleanup freshness regression checks passed"