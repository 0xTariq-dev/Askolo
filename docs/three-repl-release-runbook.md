# Askolo Three-Repl environment and release runbook

This runbook is the source of truth for environment identity and promotion. The
current Repl is the development environment. Staging and production are
separate Replit projects and are not created or configured from this workspace.

## Environment contract

Each Repl must have a unique, manually provisioned value for every item below.
Do not copy secrets, databases, OAuth clients, cookies, webhook destinations,
or provider targets between environments.

| Input | Development | Staging | Production |
| --- | --- | --- | --- |
| `ASKOLO_ENVIRONMENT` | `development` | `staging` | `production` |
| `ASKOLO_CANONICAL_ORIGIN` | local/`dev.askolo.app` after DNS | **manual input** | **manual input** |
| `ASKOLO_DATABASE_ID` | `development-database` | **manual input** | **manual input** |
| `ASKOLO_COOKIE_NAMESPACE` | `askolo_dev` | unique manual value | unique manual value |
| `ASKOLO_COMMIT_SHA` | local or current commit | promoted tag commit | promoted tag commit |
| `ASKOLO_RELEASE_TAG` | `unreleased` | immutable `vX.Y.Z-rcN` | immutable `vX.Y.Z`/hotfix tag |
| `ASKOLO_RELEASE_MODE` | `development` | `normal` or `hotfix` | `normal` or `hotfix` |
| `ASKOLO_INTERNAL_TOKEN` | local development value | staging-only secret | production-only secret |

Non-development Node and Go processes fail during startup when the required
identity, origin, database identity, cookie namespace, release provenance, or
internal token is absent. The browser build also fails closed if its
environment-specific canonical origin is absent.

The final staging and production domains are intentionally unresolved inputs.
The root `askolo.app` domain is not an app-routing default. Once DNS is
provisioned, development should use `dev.askolo.app`; staging and production
must use their separately approved single-host origins. Both the landing page
and app should be routed behind that one host in each Repl.

## Standard release

1. Work on a short-lived feature branch and merge small structural changes into
   protected `main`. Keep incomplete behavior unexposed or behind a
   server-enforced flag.
2. From the intended `main` commit, create a new immutable release-candidate
   tag, for example `v1.2.0-rc1`. Never move or reuse a tag.
3. Configure the staging Repl with that exact tag, staging-only resources, and
   its tester allowlist. Run `scripts/verify-release.sh`.
4. Verify the staging host, `/api/healthz`, Go `/healthz` and `/readyz`,
   database target, OAuth callback, cookies, webhooks, provider targets,
   access controls, and visible footer provenance.
5. Create the stable production tag from the exact verified commit. Promote
   that tag to production and run the same verification against production
   resources.
6. Record the release tag, commit, staging verification, production approval,
   and any migration notes. There is no staging-to-main merge.

## Emergency production hotfix

When `main` contains unfinished work, do not tag its tip.

1. Identify the exact immutable tag currently running in production.
2. Create a temporary branch from that tag:
   `git switch --detach <production-tag>` then create a short-lived hotfix
   branch from the detached commit.
3. Apply only the critical fix. Validate it in staging with staging resources.
4. Create a new immutable hotfix tag from the verified hotfix commit and set
   `ASKOLO_RELEASE_MODE=hotfix` plus
   `ASKOLO_PARENT_PRODUCTION_TAG=<production-tag>`.
5. Deploy that hotfix tag to production. Do not promote the current `main` tip.
6. After production verification succeeds, switch to `main`, cherry-pick the
   hotfix commit, push the reconciled trunk, then delete the temporary branch
   locally and remotely.

The release record must include the parent production tag, hotfix commit,
staging verification, production deployment, and reconciliation commit.

## Rollback

Rollback is a redeploy of a previously approved immutable stable or hotfix tag.
It is not a mutable tag move, an environment branch merge, or a rebuild from
the current workspace. Re-run the environment and data-target checks after the
rollback, and confirm that any database change was designed as
expand-and-contract so the older tag remains compatible.

## Manual account-level provisioning

These steps require Replit or Google Cloud account access and are deliberately
not automated here:

1. Create/import separate staging and production Replit projects from the
   central repository.
2. Configure each project to use its intended release tag and its own
   deployment secrets.
3. Provision DNS and custom domains, including the development
   `dev.askolo.app` record and approved final staging/production origins.
4. Create separate Google OAuth clients and callback allowlists for each
   origin. The callback is `<canonical-origin>/api/auth/google/callback`.
5. Provision separate databases, object storage paths, internal tokens,
   provider credentials, webhook destinations, and external API targets.
6. Configure branch protection so `main` is the only persistent source branch,
   tags are immutable, and release approvals are recorded.
7. Restrict staging with a server-enforced tester allowlist that does not block
   Google callbacks or provider webhooks. Mark staging/development as
   `noindex, nofollow`.

## Verification checklist

- environment name and canonical host agree; no guessed fallback is active
- tag resolves to the checked-out commit and source commit is visible
- database identity and provider target belong to the current Repl
- Google client, callback allowlist, webhook target, and cookie namespace are
  environment-specific
- health and readiness endpoints report environment, commit, and release
- non-production hosts are not indexable and staging tester access is enforced
- root-domain app routing is not assumed
- missing required configuration fails closed before serving traffic