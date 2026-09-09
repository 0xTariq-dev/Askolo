#!/usr/bin/env bash

# Remove one selected empty linear commit from a local branch.
#
# The cleaned history is created on a new local branch in an isolated
# temporary worktree. The source branch is never moved by default.
#
# Replit deployment commits are history entries, not files. They cannot be
# excluded with .gitignore or with .replit.

set -Eeuo pipefail

readonly SCRIPT_NAME="$(basename "$0")"
readonly DEFAULT_PATTERN='^Published your App$'

apply_changes=false
pattern="$DEFAULT_PATTERN"
requested_branch=""
requested_new_branch=""

die() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<EOF
Usage:
  $SCRIPT_NAME [--dry-run] [--apply] [--branch BRANCH]
              [--new-branch BRANCH] [--pattern REGEX]

Options:
  --dry-run          Inspect candidates only. This is the default.
  --apply            Create a cleaned branch after confirmation.
  --branch BRANCH   Branch to inspect. Defaults to the current branch.
  --new-branch NAME Name for the cleaned branch. Defaults to an automatic
                     drop-published/<commit>-<timestamp> name.
  --pattern REGEX   Commit-subject regex. Defaults to:
                     $DEFAULT_PATTERN
  -h, --help         Show this help.

Safety:
  - Requires a clean worktree.
  - The source branch is never changed; a cleaned branch is created.
  - The rewrite runs in an isolated temporary worktree.
  - Exactly one linear empty commit may be selected.
  - Merge and non-empty commits are rejected.
  - A backup Git ref is created before the rewrite.
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
    --new-branch)
      (($# >= 2)) || die "--new-branch requires a branch name"
      requested_new_branch="$2"
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
  die "'$branch' is not a local branch; this script never rewrites remote-tracking refs"

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
printf 'Parent kept unchanged:\n'
printf '  %s %s\n' "$parent_hash" "$(git show -s --format=%s "$parent_hash")"
printf 'Descendant commits to replay: %s\n' "$descendant_count"

if ((descendant_count > 0)); then
  printf '\nOnly descendants will receive new commit IDs because their parent changes.\n'
else
  printf '\nThis is the branch tip; no descendant commits need to be replayed.\n'
fi

if ! $apply_changes; then
  printf '\nDry run complete. Re-run with --apply to create a separate cleaned branch.\n'
  exit 0
fi

if [[ -n "$requested_new_branch" ]]; then
  new_branch="$requested_new_branch"
else
  new_branch="drop-published/${selected_hash:0:8}-$(date -u +%Y%m%dT%H%M%SZ)"
fi
git show-ref --verify --quiet "refs/heads/$new_branch" &&
  die "local branch '$new_branch' already exists"

printf '\nA new local branch will be created: %s\n' "$new_branch"
printf 'Source branch will remain unchanged: %s\n' "$branch"
printf 'Continue? Type REWRITE to continue: '
read -r confirmation
[[ "$confirmation" == REWRITE ]] ||
  die "confirmation did not match REWRITE; no changes made"

backup_ref="refs/backup/drop-published-app/$branch/$(date -u +%Y%m%dT%H%M%SZ)"
git update-ref "$backup_ref" "$branch_tip"

worktree="$(mktemp -d "${TMPDIR:-/tmp}/drop-published-worktree.XXXXXX")"
cleanup() {
  git worktree remove --force "$worktree" >/dev/null 2>&1 || true
  rmdir "$worktree" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if ((descendant_count == 0)); then
  new_tip="$parent_hash"
else
  git worktree add --detach --quiet "$worktree" "$branch_tip"
  if git -C "$worktree" rebase --rebase-merges --onto "$parent_hash" "$selected_hash"; then
    new_tip="$(git -C "$worktree" rev-parse HEAD)"
  else
    printf '\nThe isolated targeted rebase stopped or failed.\n' >&2
    printf 'The source branch was not changed. Backup ref: %s\n' "$backup_ref" >&2
    printf 'The temporary worktree will be removed.\n' >&2
    exit 1
  fi
fi

git branch "$new_branch" "$new_tip"

printf '\nCreated cleaned branch: %s\n' "$new_branch"
printf 'Source branch unchanged: %s\n' "$branch"
printf 'Backup ref: %s\n' "$backup_ref"
printf 'Compare with: git log --oneline %s..%s\n' "$branch" "$new_branch"