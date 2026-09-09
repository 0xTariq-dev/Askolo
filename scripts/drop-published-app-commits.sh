#!/usr/bin/env bash

# Remove one selected empty linear commit from a local branch.
#
# Applied rewrites intentionally follow this flow:
#   1. Detach HEAD at the requested branch tip.
#   2. Run: git rebase --onto <parent> <commit> HEAD
#   3. Show the rewritten detached HEAD.
#   4. Ask whether the requested branch should be moved to HEAD.
#   5. If confirmed, run: git branch -f <requested-branch> HEAD
#
# Replit deployment commits are history entries, not files. They cannot be
# excluded with .gitignore or with .replit.

set -Eeuo pipefail

readonly SCRIPT_NAME="$(basename "$0")"
readonly DEFAULT_PATTERN='^Published your App$'

apply_changes=false
pattern="$DEFAULT_PATTERN"
requested_branch=""

die() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<EOF
Usage:
  $SCRIPT_NAME [--dry-run] [--apply] [--branch BRANCH] [--pattern REGEX]

Options:
  --dry-run          Inspect candidates only. This is the default.
  --apply            Build the detached rewrite and prompt to update BRANCH.
  --branch BRANCH   Branch to inspect and optionally update. Defaults to the
                     current branch; detached HEAD requires this option.
  --pattern REGEX   Commit-subject regex. Defaults to:
                     $DEFAULT_PATTERN
  -h, --help         Show this help.

Safety:
  - Requires a clean worktree.
  - Only local branches can be updated.
  - Exactly one linear empty commit may be selected.
  - The requested branch is not moved until after the rewritten HEAD is shown
    and the user confirms with MERGE.
  - A backup Git ref is created before the detached rewrite.
EOF
}

while (($# > 0)); do
  case "$1" in
    --dry-run)
      apply_changes=false
      ;;
    --apply)
      apply_changes=true
      ;;
    --branch)
      (($# >= 2)) || die "--branch requires a branch name"
      requested_branch="$2"
      shift
      ;;
    --pattern)
      (($# >= 2)) || die "--pattern requires a regular expression"
      pattern="$2"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown option: $1"
      ;;
  esac
  shift
done

repo_root="$(git rev-parse --show-toplevel 2>/dev/null)" ||
  die "run this script inside a Git repository"
cd "$repo_root"

git diff --quiet && git diff --cached --quiet ||
  die "the worktree or index has changes; commit or stash them before rewriting history"

if [[ -n "$(git ls-files --others --exclude-standard)" ]]; then
  die "untracked files are present; move or remove them before rewriting history"
fi

current_branch="$(git branch --show-current)"
if [[ -n "$requested_branch" ]]; then
  branch="$requested_branch"
else
  [[ -n "$current_branch" ]] ||
    die "HEAD is detached; pass --branch with a local branch name"
  branch="$current_branch"
fi

git show-ref --verify --quiet "refs/heads/$branch" ||
  die "'$branch' is not a local branch; this script never updates remote-tracking refs"

if [[ "$branch" != "$current_branch" ]] &&
   git worktree list --porcelain | grep -Fqx "branch refs/heads/$branch"; then
  die "'$branch' is checked out in another worktree; detach that worktree first"
fi

mapfile -t matching_rows < <(
  git log --topo-order --reverse --format='%H%x09%P%x09%s' "$branch" |
    awk -F '\t' -v pattern="$pattern" '$3 ~ pattern { print }'
)

if ((${#matching_rows[@]} == 0)); then
  printf "No commits matching /%s/ were found on %s.\n" "$pattern" "$branch"
  exit 0
fi

printf "Matching commits on %s:\n\n" "$branch"
printf '%-4s %-16s %-8s %s\n' '#' 'type' 'commit' 'subject'
printf '%-4s %-16s %-8s %s\n' '----' '----------------' '--------' '-------'

declare -a hashes=()
declare -a subjects=()
declare -a commit_types=()

for row in "${matching_rows[@]}"; do
  IFS=$'\t' read -r hash parents subject <<<"$row"
  hashes+=("$hash")
  subjects+=("$subject")

  if [[ "$parents" == *" "* ]]; then
    commit_types+=(merge)
    type="merge"
  elif git diff-tree --no-commit-id --quiet -r "$hash"; then
    commit_types+=(linear-empty)
    type="linear-empty"
  else
    commit_types+=(linear-changes)
    type="linear-changes"
  fi

  printf '%-4s %-16s %-8s %s\n' "${#hashes[@]}" "$type" "${hash:0:8}" "$subject"
done

printf '\nChoose exactly one commit number to drop, or [n]one: '
read -r selection

case "${selection,,}" in
  n|none|'')
    printf 'No changes made.\n'
    exit 0
    ;;
esac

[[ "$selection" =~ ^[0-9]+$ ]] ||
  die "enter one numeric commit selection, such as 4"
((selection >= 1 && selection <= ${#hashes[@]})) ||
  die "selection '$selection' is outside the displayed range"

index=$((selection - 1))
selected_hash="${hashes[$index]}"
selected_subject="${subjects[$index]}"
selected_type="${commit_types[$index]}"

[[ "$selected_type" == linear-empty ]] ||
  die "selected commit $selected_hash is $selected_type; only linear empty commits are safe to drop automatically"

parent_hash="$(git rev-parse "$selected_hash^")"
branch_tip="$(git rev-parse "refs/heads/$branch")"
descendant_count="$(git rev-list --count "$selected_hash..$branch")"

printf '\nSelected commit:\n'
printf '  %s %s\n' "$selected_hash" "$selected_subject"
printf 'Parent commit:\n'
printf '  %s %s\n' "$parent_hash" "$(git show -s --format=%s "$parent_hash")"
printf 'Descendant commits to replay: %s\n' "$descendant_count"

if ((descendant_count > 0)); then
  printf '\nOnly descendants receive new commit IDs because their parent changes.\n'
fi

if ! $apply_changes; then
  printf '\nDry run complete. Re-run with --apply to build the detached rewrite.\n'
  exit 0
fi

original_head="$(git rev-parse HEAD)"
backup_ref="refs/backup/drop-published-app/$branch/$(date -u +%Y%m%dT%H%M%SZ)"

printf '\nThe requested branch will not move during the rebase.\n'
printf 'HEAD will detach at %s, then run:\n' "$branch"
printf '  git rebase --onto %s %s HEAD\n' "$parent_hash" "$selected_hash"
printf 'Continue? Type REWRITE to build the detached rewrite: '
read -r confirmation
[[ "$confirmation" == REWRITE ]] ||
  die "confirmation did not match REWRITE; no changes made"

git update-ref "$backup_ref" "$branch_tip"
checkout_restored=false
cleanup() {
  if [[ "$checkout_restored" != true ]]; then
    if [[ -n "$current_branch" ]]; then
      git switch "$current_branch" >/dev/null 2>&1 || true
    else
      git switch --detach "$original_head" >/dev/null 2>&1 || true
    fi
  fi
}
trap cleanup EXIT

if ! git switch --detach "$branch"; then
  git update-ref -d "$backup_ref"
  die "could not detach HEAD at $branch"
fi

if ! git rebase --onto "$parent_hash" "$selected_hash" HEAD; then
  git rebase --abort >/dev/null 2>&1 || true
  git update-ref -d "$backup_ref"
  printf '\nThe detached rebase failed; no branch was updated.\n' >&2
  exit 1
fi

printf '\nRewritten detached HEAD:\n'
git log --oneline -n 10 HEAD

printf '\nMove local branch %s to this detached HEAD? Type MERGE to continue: ' "$branch"
read -r merge_confirmation
if [[ "$merge_confirmation" != MERGE ]]; then
  git update-ref -d "$backup_ref"
  printf 'Declined. The requested branch was not changed.\n'
  exit 0
fi

git branch -f "$branch" HEAD
if [[ -n "$current_branch" ]]; then
  git switch "$current_branch"
  checkout_restored=true
else
  git switch "$branch"
  checkout_restored=true
fi

printf '\nUpdated local branch: %s\n' "$branch"
printf 'New tip: %s\n' "$(git rev-parse "$branch")"
printf 'Backup ref: %s\n' "$backup_ref"
printf 'Recover the previous tip with: git reset --hard %s\n' "$backup_ref"