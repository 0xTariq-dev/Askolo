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
readonly FRESHNESS_REQUIRED_EXIT_STATUS=2

apply_changes=false
check_upstream=false
require_fresh=false
pattern="$DEFAULT_PATTERN"
requested_branch=""

if [[ -t 1 && -z "${NO_COLOR+x}" ]]; then
  readonly COLOR_BOLD=$'\033[1m'
  readonly COLOR_CYAN=$'\033[36m'
  readonly COLOR_DIM=$'\033[2m'
  readonly COLOR_GREEN=$'\033[32m'
  readonly COLOR_MAGENTA=$'\033[35m'
  readonly COLOR_YELLOW=$'\033[33m'
  readonly COLOR_RESET=$'\033[0m'
else
  readonly COLOR_BOLD=''
  readonly COLOR_CYAN=''
  readonly COLOR_DIM=''
  readonly COLOR_GREEN=''
  readonly COLOR_MAGENTA=''
  readonly COLOR_YELLOW=''
  readonly COLOR_RESET=''
fi

if [[ -t 2 && -z "${NO_COLOR+x}" ]]; then
  readonly ERROR_COLOR=$'\033[31m'
  readonly ERROR_RESET=$'\033[0m'
else
  readonly ERROR_COLOR=''
  readonly ERROR_RESET=''
fi

die() {
  printf '%sError:%s %s\n' "$ERROR_COLOR" "$ERROR_RESET" "$*" >&2
  exit 1
}

warn() {
  printf '%sWarning:%s %s\n' "$COLOR_YELLOW" "$COLOR_RESET" "$*" >&2
}

styled() {
  local style="$1"
  local text="$2"
  printf '%s%s%s' "$style" "$text" "$COLOR_RESET"
}

check_upstream_freshness() {
  local remote_name
  local merge_ref
  local remote_output
  local remote_tip
  local local_tip
  local refresh_command

  remote_name="$(git config --get "branch.$branch.remote" || true)"
  merge_ref="$(git config --get "branch.$branch.merge" || true)"

  if [[ -z "$remote_name" || -z "$merge_ref" || "$remote_name" == "." ]]; then
    warn "could not compare configured upstream '$upstream_ref' with a remote tip; it may be stale."
    warn "This check did not fetch or change refs. Configure a remote-tracking upstream, then retry with --check-upstream."
    return 1
  fi

  refresh_command="git fetch $remote_name"
  if ! remote_output="$(git ls-remote --heads "$remote_name" "$merge_ref" 2>/dev/null)"; then
    warn "could not read remote tip '$remote_name/$merge_ref' for configured upstream '$upstream_ref'; it may be stale."
    warn "This check did not fetch or change refs. Refresh it with '$refresh_command', then retry with --check-upstream."
    return 1
  fi

  remote_tip="$(printf '%s\n' "$remote_output" | awk -v expected_ref="$merge_ref" '$2 == expected_ref { print $1; exit }')"
  if [[ -z "$remote_tip" ]]; then
    warn "remote '$remote_name' has no tip for '$merge_ref'; configured upstream '$upstream_ref' may no longer be current."
    warn "This check did not fetch or change refs. Confirm the remote branch, refresh with '$refresh_command', then retry with --check-upstream."
    return 1
  fi

  local_tip="$(git rev-parse "$upstream_ref^{commit}")"
  if [[ "$local_tip" != "$remote_tip" ]]; then
    warn "configured upstream '$upstream_ref' may be stale."
    warn "Local upstream tip:  $local_tip"
    warn "Current remote tip: $remote_tip"
    warn "No refs were fetched or changed. Refresh it with '$refresh_command', then retry with --check-upstream."
    return 1
  fi

  printf '%s\n' "$(styled "$COLOR_GREEN" "Upstream freshness check passed: '$upstream_ref' matches the current '$remote_name' tip.")"
  return 0
}

usage() {
  cat <<EOF
Usage:
  $SCRIPT_NAME [--dry-run] [--apply] [--check-upstream|--require-fresh] [--branch BRANCH] [--pattern REGEX]

Options:
  --dry-run          Inspect candidates only. This is the default.
  --apply            Build the detached rewrite and prompt to update BRANCH.
  --check-upstream   Read the configured remote tip and warn if the local
                     upstream may be stale. The check never fetches or updates refs.
  --require-fresh    Require the configured upstream to match the current
                     remote tip. Implies --check-upstream and exits with status
                     $FRESHNESS_REQUIRED_EXIT_STATUS when freshness cannot be
                     verified. The check never fetches or updates refs.
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
    --check-upstream)
      check_upstream=true
      ;;
    --require-fresh)
      check_upstream=true
      require_fresh=true
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

upstream_ref="$(git for-each-ref --format='%(upstream)' "refs/heads/$branch")"
if [[ -z "$upstream_ref" ]]; then
  if $require_fresh; then
    printf '%sError:%s freshness is required, but %s has no configured upstream.\n' \
      "$ERROR_COLOR" "$ERROR_RESET" "'$branch'" >&2
    exit "$FRESHNESS_REQUIRED_EXIT_STATUS"
  fi

  die "'$branch' has no configured upstream; configure one before inspecting unpushed commits"
fi

if ! git rev-parse --verify "$upstream_ref^{commit}" >/dev/null 2>&1; then
  if $require_fresh; then
    printf '%sError:%s freshness is required, but configured upstream %s is unavailable locally.\n' \
      "$ERROR_COLOR" "$ERROR_RESET" "'$upstream_ref'" >&2
    exit "$FRESHNESS_REQUIRED_EXIT_STATUS"
  fi

  die "configured upstream '$upstream_ref' for '$branch' is unavailable locally; fetch it before retrying"
fi

if $check_upstream && ! check_upstream_freshness; then
  if $require_fresh; then
    printf '%sError:%s freshness is required; refresh the configured upstream and retry.\n' \
      "$ERROR_COLOR" "$ERROR_RESET" >&2
    exit "$FRESHNESS_REQUIRED_EXIT_STATUS"
  fi

  $apply_changes &&
    die "refusing --apply while the configured upstream may be stale; refresh it and retry with --check-upstream"
fi

git rev-parse --verify "refs/heads/$branch^{commit}" >/dev/null 2>&1 ||
  die "could not resolve the tip of local branch '$branch'"

if printf '%s' '' | grep -Eq -- "$pattern"; then
  :
else
  regex_status=$?
  ((regex_status == 1)) ||
    die "invalid commit-subject regular expression: $pattern"
fi

mapfile -t unpushed_hashes < <(
  git rev-list --topo-order "refs/heads/$branch" --not "$upstream_ref"
)

declare -a hashes=()
declare -a subjects=()
declare -a commit_types=()
declare -a commit_dates=()

for candidate_hash in "${unpushed_hashes[@]}"; do
  # Keep each metadata field NUL-delimited. Commit subjects can contain tabs
  # and other whitespace, so tab-delimited rows are not safe to parse here.
  mapfile -d '' -t candidate_fields < <(
    git show -s --date=iso-strict \
      --format='%H%x00%P%x00%cI%x00%s%x00' "$candidate_hash"
  )

  hash="${candidate_fields[0]}"
  parents="${candidate_fields[1]}"
  commit_date="${candidate_fields[2]}"
  subject="${candidate_fields[3]}"

  [[ "$subject" =~ $pattern ]] || continue

  hashes+=("$hash")
  subjects+=("$subject")
  commit_dates+=("$commit_date")

  if [[ "$parents" == *" "* ]]; then
    commit_types+=(merge)
  elif git diff-tree --no-commit-id --quiet -r "$hash"; then
    commit_types+=(linear-empty)
  else
    commit_types+=(linear-changes)
  fi
done

if ((${#hashes[@]} == 0)); then
  if ((${#unpushed_hashes[@]} == 0)); then
    printf '%s\n' "$(styled "$COLOR_YELLOW" "No selectable commits matching /$pattern/ were found: local branch '$branch' has no commits outside upstream '$upstream_ref'. Matching commits already reachable from that upstream are excluded.")"
  else
    mapfile -t local_subjects < <(
      git log --topo-order --format='%s' "refs/heads/$branch"
    )
    local_match_count=0
    for local_subject in "${local_subjects[@]}"; do
      if [[ "$local_subject" =~ $pattern ]]; then
        ((local_match_count += 1))
      fi
    done

    if ((local_match_count == 0)); then
      printf '%s\n' "$(styled "$COLOR_YELLOW" "No commits matching /$pattern/ exist on local branch '$branch'; compared it against upstream '$upstream_ref' and found no selectable candidates.")"
    else
      printf '%s\n' "$(styled "$COLOR_YELLOW" "No selectable commits matching /$pattern/ were found on local-only history for '$branch' versus upstream '$upstream_ref'; matching commits are already reachable from that upstream.")"
    fi
  fi
  exit 0
fi

printf '%s\n\n' "$(styled "$COLOR_BOLD$COLOR_CYAN" "Unpushed matching commits on $branch:")"
printf '%-4s %-25s %-16s %-10s %s\n' '#' 'date' 'type' 'commit' 'subject'
printf '%-4s %-25s %-16s %-10s %s\n' '----' '-------------------------' '----------------' '----------' '-------'

for index in "${!hashes[@]}"; do
  hash="${hashes[$index]}"
  subject="${subjects[$index]}"
  commit_date="${commit_dates[$index]}"
  type="${commit_types[$index]}"

  styled_date="$(styled "$COLOR_DIM" "$commit_date")"
  styled_type="$(styled "$COLOR_YELLOW" "$type")"
  styled_hash="$(styled "$COLOR_MAGENTA" "${hash:0:8}")"
  printf '%-4s %-25s %-16s %-10s %s\n' \
    "$((index + 1))" "$styled_date" "$styled_type" "$styled_hash" "$subject"
done

printf '\n%s' "$(styled "$COLOR_BOLD" 'Choose exactly one commit number to drop, or [n]one: ')"
read -r selection

case "${selection,,}" in
  n|none|'')
    printf '%s\n' "$(styled "$COLOR_YELLOW" 'No changes made.')"
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
selected_date="${commit_dates[$index]}"

[[ "$selected_type" == linear-empty ]] ||
  die "selected commit $selected_hash is $selected_type; only linear empty commits are safe to drop automatically"

parent_hash="$(git rev-parse "$selected_hash^")"
branch_tip="$(git rev-parse "refs/heads/$branch")"
descendant_count="$(git rev-list --count "$selected_hash..$branch")"

printf '\n%s\n' "$(styled "$COLOR_BOLD$COLOR_CYAN" 'Selected commit:')"
printf '  %s %s %s\n' \
  "$(styled "$COLOR_MAGENTA" "$selected_hash")" \
  "$(styled "$COLOR_DIM" "$selected_date")" \
  "$selected_subject"
printf '%s\n' "$(styled "$COLOR_BOLD$COLOR_CYAN" 'Parent commit:')"
printf '  %s %s\n' "$parent_hash" "$(git show -s --format=%s "$parent_hash")"
printf 'Descendant commits to replay: %s\n' "$descendant_count"

if ((descendant_count > 0)); then
  printf '\n%s\n' "$(styled "$COLOR_YELLOW" 'Only descendants receive new commit IDs because their parent changes.')"
fi

if ! $apply_changes; then
  printf '\n%s\n' "$(styled "$COLOR_GREEN" 'Dry run complete. Re-run with --apply to build the detached rewrite.')"
  exit 0
fi

original_head="$(git rev-parse HEAD)"
backup_ref="refs/backup/drop-published-app/$branch/$(date -u +%Y%m%dT%H%M%SZ)"

printf '\n%s\n' "$(styled "$COLOR_YELLOW" 'The requested branch will not move during the rebase.')"
printf 'HEAD will detach at %s, then run:\n' "$branch"
printf '  git rebase --onto %s %s HEAD\n' "$parent_hash" "$selected_hash"
printf '%s' "$(styled "$COLOR_BOLD" 'Continue? Type REWRITE to build the detached rewrite: ')"
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
  printf '\n%sThe detached rebase failed; no branch was updated.%s\n' \
    "$ERROR_COLOR" "$ERROR_RESET" >&2
  exit 1
fi

printf '\n%s\n' "$(styled "$COLOR_BOLD$COLOR_CYAN" 'Rewritten detached HEAD:')"
git log --oneline -n 10 HEAD

printf '\n%s' "$(styled "$COLOR_BOLD" "Move local branch $branch to this detached HEAD? Type MERGE to continue: ")"
read -r merge_confirmation
if [[ "$merge_confirmation" != MERGE ]]; then
  git update-ref -d "$backup_ref"
  printf '%s\n' "$(styled "$COLOR_YELLOW" 'Declined. The requested branch was not changed.')"
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

printf '\n%s\n' "$(styled "$COLOR_GREEN" "Updated local branch: $branch")"
printf '%s\n' "$(styled "$COLOR_GREEN" "New tip: $(git rev-parse "$branch")")"
printf '%s\n' "$(styled "$COLOR_DIM" "Backup ref: $backup_ref")"
printf '%s\n' "$(styled "$COLOR_DIM" "Recover the previous tip with: git reset --hard $backup_ref")"