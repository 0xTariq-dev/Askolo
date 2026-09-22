# Askolo later staging and production attachment plan

This workspace remains the development Repl. Staging and production must be
created and configured as separate Replit projects when release work resumes.
Do not turn this workspace into either environment.

## Current decision

- Development work stays here.
- Local workflows use the development database, development cookie namespace,
  development provider clients, and development-only secrets.
- Staging and production resources are not copied into this Repl and are not
  used for local verification.
- The existing three-Repl release runbook remains the source of truth for
  promotion, rollback, and environment identity.

## Later attachment sequence

### 1. Prepare staging as a separate Replit project

1. Create or import a separate Replit project from the central repository.
2. Check out an immutable release-candidate tag from the intended `main`
   commit; do not attach staging to an arbitrary development checkout.
3. Configure staging-only values for `ASKOLO_ENVIRONMENT`,
   `ASKOLO_CANONICAL_ORIGIN`, `ASKOLO_DATABASE_ID`,
   `ASKOLO_COOKIE_NAMESPACE`, `ASKOLO_COMMIT_SHA`, `ASKOLO_RELEASE_TAG`,
   `ASKOLO_RELEASE_MODE`, and `ASKOLO_INTERNAL_TOKEN`.
4. Provision a separate staging database, object-storage path, provider target,
   cookie namespace, and Google OAuth clients.
5. Register exactly
   `<staging-canonical-origin>/api/auth/google/callback` on the staging Google
   login client. Keep login and Google integration clients separate.
6. Add the staging tester allowlist without blocking OAuth callbacks or
   provider webhooks.
7. Run `scripts/verify-release.sh`, then perform the disposable-account Google
   sign-in, signup, denial, unsafe-return, expiry, and replay checks.

### 2. Prepare production after staging approval

1. Create or use a separate production Replit project.
2. Create the stable production tag from the exact commit verified in staging.
3. Configure production-only database, storage, provider targets, OAuth
   clients, cookie namespace, internal token, and deployment secrets.
4. Register only the production canonical callback URI on the production
   Google login client.
5. Deploy the approved stable tag and run the same environment, health,
   readiness, routing, OAuth, and provenance checks against production.

### 3. Keep environments isolated

- Never share database URLs, OAuth client secrets, session secrets, internal
  tokens, cookies, webhook destinations, or provider credentials.
- Never use a development preview host as a staging or production callback.
- Never promote the current development checkout directly to production.
- Record only safe release metadata: environment, canonical origin, release
  tag, commit, result status, and timestamp. Do not record credentials,
  authorization codes, tokens, or disposable-account details.

## Handoff checklist

Before attaching either later Repl, confirm:

- the target Replit project is separate from this development workspace;
- the target environment identity and canonical origin agree;
- the target database and provider resources are isolated;
- the exact release tag and commit are known;
- the matching Google OAuth client and callback allowlist are registered;
- the target has its own secrets and cookie namespace; and
- the release verification command and account-level smoke checks are ready.

See [`docs/three-repl-release-runbook.md`](three-repl-release-runbook.md) for
the authoritative promotion and rollback procedure.