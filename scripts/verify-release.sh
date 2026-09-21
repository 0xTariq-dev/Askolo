#!/usr/bin/env bash
set -euo pipefail

# Fail-closed promotion check. It validates the release contract without
# printing any secret values. Run it from the repository root in the target
# Repl before starting a staging or production deployment.

die() {
  echo "release verification failed: $*" >&2
  exit 1
}

environment="${ASKOLO_ENVIRONMENT:-}"
[[ "$environment" == "staging" || "$environment" == "production" ]] ||
  die "ASKOLO_ENVIRONMENT must be staging or production"

for name in ASKOLO_CANONICAL_ORIGIN ASKOLO_DATABASE_ID ASKOLO_COOKIE_NAMESPACE \
  ASKOLO_COMMIT_SHA ASKOLO_RELEASE_TAG ASKOLO_INTERNAL_TOKEN; do
  [[ -n "${!name:-}" ]] || die "$name is required"
done

[[ "$ASKOLO_CANONICAL_ORIGIN" == https://* ]] ||
  die "ASKOLO_CANONICAL_ORIGIN must use HTTPS"
[[ "$ASKOLO_COOKIE_NAMESPACE" =~ ^[a-z][a-z0-9_-]{1,31}$ ]] ||
  die "ASKOLO_COOKIE_NAMESPACE is invalid"
[[ "$ASKOLO_RELEASE_TAG" != *[[:space:]]* ]] ||
  die "ASKOLO_RELEASE_TAG must not contain whitespace"

if [[ -d .git ]]; then
  current_commit="$(git rev-parse HEAD)"
  expected_commit="$(git rev-parse "$ASKOLO_COMMIT_SHA^{commit}" 2>/dev/null || true)"
  [[ -n "$expected_commit" && "$current_commit" == "$expected_commit" ]] ||
    die "checked out commit does not match ASKOLO_COMMIT_SHA"

  release_commit="$(git rev-parse "$ASKOLO_RELEASE_TAG^{commit}" 2>/dev/null || true)"
  [[ -n "$release_commit" && "$release_commit" == "$current_commit" ]] ||
    die "release tag does not resolve to the checked out commit"

  [[ "$(git tag --list "$ASKOLO_RELEASE_TAG" | wc -l)" -eq 1 ]] ||
    die "release tag must already exist locally; create it through the release process"
fi

release_mode="${ASKOLO_RELEASE_MODE:-normal}"
case "$release_mode" in
  normal)
    [[ -z "${ASKOLO_PARENT_PRODUCTION_TAG:-}" ]] ||
      die "normal releases must not declare a production parent tag"
    ;;
  hotfix)
    parent="${ASKOLO_PARENT_PRODUCTION_TAG:-}"
    [[ -n "$parent" && "$parent" != "$ASKOLO_RELEASE_TAG" ]] ||
      die "hotfix releases require a different parent production tag"
    if [[ -d .git ]]; then
      parent_commit="$(git rev-parse "$parent^{commit}" 2>/dev/null || true)"
      [[ -n "$parent_commit" && "$parent_commit" != "$current_commit" ]] ||
        die "hotfix commit must differ from its production baseline"
    fi
    ;;
  *)
    die "ASKOLO_RELEASE_MODE must be normal or hotfix"
    ;;
esac

echo "release verified: environment=$environment tag=$ASKOLO_RELEASE_TAG commit=$ASKOLO_COMMIT_SHA mode=$release_mode"