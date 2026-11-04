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
| `ASKOLO_CANONICAL_ORIGIN` | local/`dev.askolo.app` after DNS | **manual input** | `https://askolo.app` migration target |
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

The staging domain remains a separately approved input. The approved production
migration target is `https://askolo.app`, with the landing page at `/` and
sign-in and authenticated app routes on that same origin. This target is not
confirmation that live DNS or Production settings have been changed.
Development should use its own preview host or `dev.askolo.app` after DNS is
provisioned. Do not point Development OAuth callbacks at the production domain.

## Root-domain migration

This is a Production cutover checklist, not a command to run from Development.
Obtain approval to change Production settings before performing these steps.

1. In the Production project's Publishing domain settings, connect or confirm
   `askolo.app` using the DNS records supplied by Replit. Keep previous domains
   working during verification; do not remove them before the new origin works.
2. Set these non-secret values in the Production project only:

   | Setting | Target value |
   | --- | --- |
   | `ASKOLO_CANONICAL_ORIGIN` | `https://askolo.app` |
   | `VITE_CANONICAL_ORIGIN` | `https://askolo.app` |
   | `VITE_PUBLIC_ORIGIN` | `https://askolo.app` |
   | `VITE_APP_ORIGIN` | `https://askolo.app` |

   Vite values are build-time inputs: rebuild and publish after changing them.
   Do not change database identity, cookie namespace, credentials, or storage
   targets as part of this domain-only migration.
3. Register the exact Production callbacks with the matching Production
   provider clients before testing:

   - Google login: `https://askolo.app/api/auth/google/callback`
   - Google sign-in linking: `https://askolo.app/api/auth/google/link/callback`
   - Google integrations: `https://askolo.app/api/integrations/google/callback`
   - Microsoft: `https://askolo.app/api/integrations/microsoft/callback`

   Keep previous registered callbacks until the cutover is verified.
   Microsoft publisher-domain verification is separate from redirect-URI
   registration; completing one does not configure the other.
4. Publish an approved release containing
   `public/.well-known/microsoft-identity-association.json`. The file must be
   publicly readable without signing in at
   `https://askolo.app/.well-known/microsoft-identity-association.json`, with
   HTTP 200, JSON content, and application ID
   `3766b5bb-ce42-4733-96e7-51655a8a298b`. Confirm the response is the file,
   not the SPA's HTML fallback, then complete Microsoft's domain check.
5. Verify the landing page remains at `/`, legal pages remain reachable, app
   links stay on `askolo.app`, and direct sign-in/settings routes work after
   reload. Check OAuth callbacks, connection status, and the release health
   checks against the approved Production target.

Host-only session cookies do not transfer from a previous subdomain to the root
domain; users may need to sign in again. Do not delete accounts or connections
to accomplish the migration. Existing same-origin routing supports this layout;
no landing-page replacement is required.

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
6. Configure Datadog `DD_API_KEY` as a secret and the matching non-secret
   `DD_SITE` value in each environment that needs telemetry. The current
   operational On-Call monitor is Production-only; the editor preview remains
   tagged as Development. Never copy a key into source control or release notes.
7. Route the Production MFA monitor to the existing `askolo-team` Datadog
   On-Call handle. Its current escalation policy targets a responder directly,
   so a schedule is optional. The backend submits privacy-safe HTTP latency,
   request-volume, Go runtime, and PostgreSQL pool metrics through the official
   Datadog Go API client; a Datadog Agent is not required for those metrics.
8. Configure branch protection so `main` is the only persistent source branch,
   tags are immutable, and release approvals are recorded.
9. Restrict staging with a server-enforced tester allowlist that does not block
   Google callbacks or provider webhooks. Mark staging/development as
   `noindex, nofollow`.

## Verification checklist

- environment name and canonical host agree; no guessed fallback is active
- tag resolves to the checked-out commit and source commit is visible
- database identity and provider target belong to the current Repl
- Google client, callback allowlist, webhook target, and cookie namespace are
  environment-specific
- health and readiness endpoints report environment, commit, and release
- native auth email readiness distinguishes missing/invalid Resend or challenge
  settings from unknown, transient, and persistent provider delivery failures;
  aggregate counts and latency contain no user data
- the Production MFA alert monitor is grouped by environment, operation, and
  fixed reason set and routes through `askolo-team`
- the MFA recovery event submits a zero gauge and resolves the matching monitor
  group without manual intervention
- non-production hosts are not indexable and staging tester access is enforced
- root-domain app routing is not assumed
- missing required configuration fails closed before serving traffic

## Native auth email delivery checks

Before staging or production promotion, verify the target environment has
`RESEND_API_KEY`, `AUTH_EMAIL_FROM`, and `AUTH_CHALLENGE_SECRET` (or
`SESSION_SECRET`). Do not include credential values, addresses, codes, message
bodies, or raw provider errors in release records.

Use `/readyz` to check `emailDelivery.resendConfiguration`,
`emailDelivery.challengeConfiguration`, and `emailDelivery.status`. `unknown`
means configuration is present but no message has been attempted; it is not
evidence of delivery. After a staging test, confirm one `handoff`, bounded
latency, and an incremented aggregate handoff count. A resend during the
60-second cooldown must be rate-limited.

To exercise failure handling safely, use a disposable staging account and a
provider rejection or unreachable Resend endpoint. The sender classifies
configuration, connection, authentication, provider rejection, and timeout
failures without logging private data. Retry-safe failures release the
persisted pending challenge so a later resend can proceed; uncertain handoff
failures retain it and the cooldown prevents duplicate sends. One or two
consecutive failures are transient; three are persistent and require provider
investigation before promotion.