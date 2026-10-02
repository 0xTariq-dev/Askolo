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

setup_merge_with_equivalent_patches_fixture() {
  local case_root="$fixture_root/merge-with-equivalent-patches"
  local repo="$case_root/repo"
  local remote="$case_root/remote.git"

  mkdir -p "$case_root"
  git init -q --bare "$remote"
  init_repo "$repo"
  printf 'before\n' >"$repo/ui.txt"
  git -C "$repo" add ui.txt
  git -C "$repo" commit -q -m "Add UI baseline"
  git -C "$repo" remote add origin "$remote"
  git -C "$repo" push -q -u origin main

  local upstream_tip
  upstream_tip="$(git -C "$repo" rev-parse main)"
  add_candidate_commit "$repo"

  printf 'after\n' >"$repo/ui.txt"
  git -C "$repo" add ui.txt
  git -C "$repo" commit -q -m "Refactor assistant UI"
  printf 'main branch\n' >"$repo/main-only.txt"
  git -C "$repo" add main-only.txt
  git -C "$repo" commit -q -m "Continue main work"

  git -C "$repo" switch -q -c equivalent-patch-side "$upstream_tip"
  printf 'after\n' >"$repo/ui.txt"
  git -C "$repo" add ui.txt
  git -C "$repo" commit -q -m "Refactor assistant UI"
  printf 'side branch\n' >"$repo/side-only.txt"
  git -C "$repo" add side-only.txt
  git -C "$repo" commit -q -m "Add side-branch work"

  git -C "$repo" switch -q main
  git -C "$repo" merge --no-ff -q -m "Merge equivalent UI branch" equivalent-patch-side
  printf 'after merge\n' >"$repo/after-merge.txt"
  git -C "$repo" add after-merge.txt
  git -C "$repo" commit -q -m "Continue after merge"

  printf '%s\n' "$repo"
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

run_required_freshness_check() {
  local repo="$1"

  set +e
  test_output="$(
    cd "$repo"
    printf 'n\n' |
      NO_COLOR=1 bash "$cleanup_script" \
        --dry-run --require-fresh --pattern '^Published your App$' \
        2>&1
  )"
  test_status=$?
  set -e
}

run_complex_history_apply_check() {
  local repo="$1"

  set +e
  test_output="$(
    cd "$repo"
    printf '1\n' |
      NO_COLOR=1 bash "$cleanup_script" \
        --apply --check-upstream --pattern '^Published your App$' \
        2>&1
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

test_required_freshness_matching_tips() {
  local repo
  local before_refs

  repo="$(setup_remote_fixture required-freshness-matching-tips)"
  before_refs="$(git -C "$repo" for-each-ref \
    --format='%(refname) %(objectname)' refs/heads refs/remotes)"

  run_required_freshness_check "$repo"
  [[ "$test_status" -eq 0 ]] ||
    fail "required freshness check failed with matching tips:
$test_output"
  assert_contains "$test_output" \
    "Upstream freshness check passed: 'refs/remotes/origin/main' matches the current 'origin' tip."
  assert_contains "$test_output" "No changes made."
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

test_required_freshness_remote_ahead_tips() {
  local case_root="$fixture_root/required-freshness-remote-ahead-tips"
  local repo
  local remote_clone="$case_root/remote-clone"
  local before_refs

  repo="$(setup_remote_fixture required-freshness-remote-ahead-tips)"
  git clone -q --branch main "$case_root/remote.git" "$remote_clone"
  git -C "$remote_clone" config user.name "Publish cleanup remote test"
  git -C "$remote_clone" config user.email "publish-cleanup-remote@example.invalid"
  git -C "$remote_clone" commit --allow-empty -q -m "Remote-only commit"
  git -C "$remote_clone" push -q origin main

  before_refs="$(git -C "$repo" for-each-ref \
    --format='%(refname) %(objectname)' refs/heads refs/remotes)"
  run_required_freshness_check "$repo"
  [[ "$test_status" -eq 2 ]] ||
    fail "required freshness check did not return status 2 for a stale upstream:
$test_output"
  assert_contains "$test_output" \
    "configured upstream 'refs/remotes/origin/main' may be stale."
  assert_contains "$test_output" \
    "freshness is required; refresh the configured upstream and retry."
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

test_required_freshness_unavailable_remote() {
  local case_root="$fixture_root/required-freshness-unavailable-remote"
  local repo
  local before_refs

  repo="$(setup_remote_fixture required-freshness-unavailable-remote)"
  rm -rf -- "$case_root/remote.git"
  before_refs="$(git -C "$repo" for-each-ref \
    --format='%(refname) %(objectname)' refs/heads refs/remotes)"

  run_required_freshness_check "$repo"
  [[ "$test_status" -eq 2 ]] ||
    fail "required freshness check did not return status 2 when the remote was unavailable:
$test_output"
  assert_contains "$test_output" \
    "could not read remote tip 'origin/refs/heads/main'"
  assert_contains "$test_output" \
    "freshness is required; refresh the configured upstream and retry."
  assert_refs_unchanged "$repo" "$before_refs"
}

test_required_freshness_unavailable_local_upstream() {
  local repo
  local before_refs

  repo="$(setup_remote_fixture required-freshness-unavailable-local-upstream)"
  git -C "$repo" update-ref -d refs/remotes/origin/main
  before_refs="$(git -C "$repo" for-each-ref \
    --format='%(refname) %(objectname)' refs/heads refs/remotes)"

  run_required_freshness_check "$repo"
  [[ "$test_status" -eq 2 ]] ||
    fail "required freshness check did not return status 2 when the local upstream was unavailable:
$test_output"
  assert_contains "$test_output" \
    "freshness is required, but configured upstream 'refs/remotes/origin/main' is unavailable locally."
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

test_required_freshness_local_only_upstream() {
  local case_root="$fixture_root/required-freshness-local-only-upstream"
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
  run_required_freshness_check "$repo"
  [[ "$test_status" -eq 2 ]] ||
    fail "required freshness check did not return status 2 with a local-only upstream:
$test_output"
  assert_contains "$test_output" \
    "Configure a remote-tracking upstream, then retry with --check-upstream."
  assert_contains "$test_output" \
    "freshness is required; refresh the configured upstream and retry."
  assert_refs_unchanged "$repo" "$before_refs"
}

test_merge_and_equivalent_patches_are_refused() {
  local repo
  local before_refs
  local publish_commit
  local main_ui_commit
  local side_ui_commit
  local merge_commit

  repo="$(setup_merge_with_equivalent_patches_fixture)"
  publish_commit="$(git -C "$repo" log --format='%H' --grep='^Published your App$' main)"
  main_ui_commit="$(git -C "$repo" log --first-parent --format='%H' --grep='^Refactor assistant UI$' main | head -n 1)"
  side_ui_commit="$(git -C "$repo" log --first-parent --format='%H' --grep='^Refactor assistant UI$' equivalent-patch-side | head -n 1)"
  merge_commit="$(git -C "$repo" log --format='%H' --merges -1 main)"
  [[ -n "$publish_commit" && -n "$main_ui_commit" && -n "$side_ui_commit" && -n "$merge_commit" ]] ||
    fail "could not construct the merge and equivalent-patch fixture"

  before_refs="$(git -C "$repo" for-each-ref \
    --format='%(refname) %(objectname)' refs/heads refs/remotes)"

  run_complex_history_apply_check "$repo"
  [[ "$test_status" -ne 0 ]] ||
    fail "cleanup script accepted a selected-to-tip range with a merge and equivalent patches"
  assert_contains "$test_output" "merge commit(s):"
  assert_contains "$test_output" "patch-equivalent commit(s):"
  if ! grep -Fq "$main_ui_commit=$side_ui_commit" <<<"$test_output" &&
     ! grep -Fq "$side_ui_commit=$main_ui_commit" <<<"$test_output"; then
    fail "cleanup script did not identify the two patch-equivalent UI commits:
$test_output"
  fi
  assert_contains "$test_output" "refusing before rewriting"
  if grep -Fq "Continue? Type REWRITE" <<<"$test_output"; then
    fail "cleanup script prompted to rewrite before refusing the complex history"
  fi
  assert_refs_unchanged "$repo" "$before_refs"
  [[ "$(git -C "$repo" rev-parse main)" != "$publish_commit" ]] ||
    fail "cleanup script unexpectedly moved main to the selected publish commit"
  [[ -z "$(git -C "$repo" status --porcelain)" ]] ||
    fail "cleanup script left worktree changes after refusing the complex history"
}

test_matching_tips
test_required_freshness_matching_tips
test_remote_ahead_tips
test_required_freshness_remote_ahead_tips
test_unavailable_remote
test_required_freshness_unavailable_remote
test_required_freshness_unavailable_local_upstream
test_local_only_upstream
test_required_freshness_local_only_upstream
test_merge_and_equivalent_patches_are_refused

echo "Published app cleanup history and freshness regression checks passed"
