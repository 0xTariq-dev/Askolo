#!/usr/bin/env bash

# Remove selected non-merge commits from a local branch history.
#
# This intentionally does not rewrite history automatically. Run it without
# --apply to inspect the candidates first, then pass --apply and confirm.
#
# Replit deployment commits are history entries, not files. They cannot be
# excluded with .gitignore or with .replit. This script is the explicit,
# reversible cleanup path for local branches.

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
  --apply            Rewrite the selected local branch after confirmation.
  --branch BRANCH   Branch to inspect. Defaults to the current branch.
  --pattern REGEX   Commit-subject regex. Defaults to:
                     $DEFAULT_PATTERN
  -h, --help         Show this help.

Safety:
  - Requires a clean worktree.
  - Only local branches can be rewritten.
  - Merge commits are displayed but cannot be selected automatically.
  - A backup Git ref is created before an applied rewrite.
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
  die "'$branch' is not a local branch; this script never rewrites remote-tracking refs"

if [[ "$branch" != "$current_branch" ]]; then
  printf "Selected branch: %s (current branch: %s)\n" "$branch" "${current_branch:-detached HEAD}"
  printf "The script will switch to the selected branch only after confirmation.\n"
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
printf '%-4s %-14s %-8s %s\n' '#' 'type' 'commit' 'subject'
printf '%-4s %-14s %-8s %s\n' '----' '--------------' '--------' '-------'

declare -a hashes=()
declare -a subjects=()
declare -a is_merge=()
declare -a is_empty=()

for row in "${matching_rows[@]}"; do
  IFS=$'\t' read -r hash parents subject <<<"$row"
  hashes+=("$hash")
  subjects+=("$subject")
  if [[ "$parents" == *" "* ]]; then
    is_merge+=(true)
    is_empty+=(false)
    commit_type="MERGE"
  else
    is_merge+=(false)
    if git diff-tree --no-commit-id --quiet -r "$hash"; then
      is_empty+=(true)
      commit_type="linear-empty"
    else
      is_empty+=(false)
      commit_type="linear-changes"
    fi
  fi
  printf '%-4s %-14s %-8s %s\n' "${#hashes[@]}" "$commit_type" "${hash:0:8}" "$subject"
done

printf '\nChoose commits to drop: [a]ll linear commits, comma-separated numbers, or [n]one: '
read -r selection

case "${selection,,}" in
  n|none|'')
    printf 'No changes made.\n'
    exit 0
    ;;
  a|all)
    selected_indices=()
    for index in "${!hashes[@]}"; do
      [[ "${is_merge[$index]}" == false && "${is_empty[$index]}" == true ]] &&
        selected_indices+=("$index")
    done
    ;;
  *)
    selected_indices=()
    IFS=',' read -r -a requested_indices <<<"$selection"
    for requested_index in "${requested_indices[@]}"; do
      [[ "$requested_index" =~ ^[0-9]+$ ]] ||
        die "invalid selection '$requested_index'; use numbers such as 1,3,4"
      ((requested_index >= 1 && requested_index <= ${#hashes[@]})) ||
        die "selection '$requested_index' is outside the displayed range"
      index=$((requested_index - 1))
      [[ "${is_merge[$index]}" == false ]] ||
        die "commit ${hashes[$index]:0:12} is a merge commit; review and drop it manually"
      [[ "${is_empty[$index]}" == true ]] ||
        die "commit ${hashes[$index]:0:12} changes files; it is not safe to auto-drop"
      selected_indices+=("$index")
    done
    ;;
esac

if ((${#selected_indices[@]} == 0)); then
  printf 'No linear commits were selected. Merge commits are not rewritten automatically.\n'
  exit 0
fi

selected_hashes=()
printf '\nSelected linear commits:\n'
for index in "${selected_indices[@]}"; do
  printf '  %s %s\n' "${hashes[$index]:0:12}" "${subjects[$index]}"
  selected_hashes+=("${hashes[$index]}")
done

if ! $apply_changes; then
  printf '\nDry run complete. Re-run with --apply to create a backup ref and rewrite this branch.\n'
  exit 0
fi

printf '\nThis rewrites local history on %s. Remote branches are not changed.\n' "$branch"
printf 'Continue? Type REWRITE to continue: '
read -r confirmation
[[ "$confirmation" == REWRITE ]] ||
  die "confirmation did not match REWRITE; no changes made"

if [[ "$branch" != "$current_branch" ]]; then
  git switch "$branch"
fi

backup_ref="refs/backup/drop-published-app/$branch/$(date -u +%Y%m%dT%H%M%SZ)"
git update-ref "$backup_ref" "refs/heads/$branch"

hash_file="$(mktemp "${TMPDIR:-/tmp}/drop-published-app-hashes.XXXXXX")"
editor_file="$(mktemp "${TMPDIR:-/tmp}/drop-published-app-editor.XXXXXX")"
cleanup() {
  rm -f "$hash_file" "$editor_file"
}
trap cleanup EXIT

printf '%s\n' "${selected_hashes[@]}" >"$hash_file"

cat >"$editor_file" <<'EDITOR'
#!/usr/bin/env bash
set -Eeuo pipefail

todo_file="$1"
replacement="${todo_file}.drop"

while IFS= read -r line || [[ -n "$line" ]]; do
  if [[ "$line" =~ ^(pick|reword|edit|squash|fixup)[[:space:]]+([0-9a-f]+)(.*)$ ]] &&
     grep -Fq "${BASH_REMATCH[2]}" "$DROP_PUBLISHED_HASH_FILE"; then
    printf 'drop %s%s\n' "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}" >>"$replacement"
  else
    printf '%s\n' "$line" >>"$replacement"
  fi
done <"$todo_file"

mv "$replacement" "$todo_file"
EDITOR

chmod +x "$editor_file"
export DROP_PUBLISHED_HASH_FILE="$hash_file"
export GIT_SEQUENCE_EDITOR="$editor_file"

if git rebase --interactive --rebase-merges --root "$branch"; then
  printf '\nHistory rewrite completed.\n'
  printf 'Backup ref: %s\n' "$backup_ref"
  printf 'Restore with: git reset --hard %s\n' "$backup_ref"
else
  printf '\nThe rebase stopped or failed. The backup ref is still available: %s\n' "$backup_ref" >&2
  printf 'If a rebase is in progress, inspect it with git status and abort with git rebase --abort.\n' >&2
  exit 1
fi