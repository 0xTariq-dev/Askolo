---
name: Three-Repl release architecture
description: Environment identity, release promotion, and production-baseline hotfix guardrails for Askolo.
---

## Environment identity

Askolo uses one central GitHub repository and three separate Replit projects. `main` is the only persistent source-of-truth branch.

Before changing configuration, data, integrations, releases, callbacks, or infrastructure, determine which Repl is active and apply that environment's guardrails:

- **Development Repl:** Automatically follows merged `main` changes. It is used for active feature development, manual verification, and agent task execution. It uses sandbox databases, development integration clients, and test credentials. The current Repl is development.
- **Staging Repl:** Normally checks out immutable release-candidate tags such as `v1.1.0-rc1`. It is used for pre-production and structural verification with an isolated staging database and sandbox configuration. During an emergency hotfix, it may temporarily validate the short-lived hotfix branch before the immutable hotfix tag is created.
- **Production Repl:** Deploys only frozen stable or hotfix tags such as `v1.1.0` or `v1.0.1-hotfix`. It is public and uses only production databases, clients, secrets, callbacks, webhooks, and API targets.

Each Repl serves the full landing page and application from one environment-specific host. The root domain is not used for app routing. Never copy secrets, database connections, OAuth clients, cookie namespaces, callback URLs, webhook destinations, or external API targets between environments.

**Why:** Separate projects prevent development and staging activity from affecting public users, while explicit environment identity prevents a Repl from silently using another environment's resources. A shell can report `REPLIT_ENVIRONMENT=production` while its `DATABASE_URL` actually matches Development; the Go migration CLI trusts its target flag, so relying on the label could direct DDL to the wrong database.

**How to apply:** Treat missing or contradictory environment identity, host, tag provenance, secret set, database target, or integration target as a blocking error. Do not infer an environment only from the branch currently checked out or `REPLIT_ENVIRONMENT`. Before a database write, compare a read-only identity fingerprint from the active connection with the named Development and Production database connections; never print the connection string.

## Standard feature release

1. Break features into small structural micro-PRs and merge them sequentially into protected `main`.
2. Keep unfinished backend behavior dark-launched through unexposed routes or server-enforced feature flags.
3. Create an immutable release-candidate tag from the intended `main` commit.
4. Validate that exact tag in the staging Repl.
5. Create the stable production tag from the same verified commit.
6. Deploy the stable tag in the production Repl.

Never move or reuse an existing release tag. Production promotion means selecting the already verified commit, not rebuilding from an unknown workspace state.

## Emergency production-baseline hotfix

If `main` contains unfinished micro-PRs, never cut an emergency production patch from the tip of `main`; the resulting tag would include every earlier trunk commit.

1. Identify the exact immutable tag currently deployed to production.
2. Create a short-lived hotfix branch from that production tag.
3. Apply and debug only the critical fix on the temporary branch.
4. Validate the hotfix branch in the staging Repl with staging-only resources.
5. Create a new immutable hotfix tag from the verified hotfix commit.
6. Deploy that hotfix tag to the production Repl.
7. Cherry-pick the hotfix commit into `main` and push the reconciled trunk.
8. Delete the temporary hotfix branch locally and remotely after reconciliation succeeds.

The temporary hotfix branch is an emergency isolation mechanism, not a persistent production branch.

**Why:** A tag from the current `main` tip cannot exclude unfinished commits already in trunk history. Starting from the live production tag isolates the patch while preserving immutable provenance.

**How to apply:** Record the parent production tag, hotfix commit, new hotfix tag, staging verification, production deployment, and trunk reconciliation. Stop if the production baseline cannot be proven.

## Absolute guardrails

- No persistent `development`, `staging`, `production`, or hotfix branches.
- `main` is the only persistent source-of-truth branch.
- Release-candidate, stable, and hotfix tags are immutable.
- Staging normally validates release-candidate tags; temporary hotfix-branch validation is the controlled emergency exception.
- Production deploys only a verified immutable stable or hotfix tag.
- Incomplete code must be inert when disabled; a client-only flag is insufficient.
- Database changes must use expand-and-contract migrations so independently deployed tags remain compatible.
- Rollback redeploys a previously approved immutable tag; it does not move a tag or merge an environment branch.
- A hotfix is incomplete until its commit is reconciled into `main` and its temporary branch is deleted.