# Go-only migration boundary

This document records the cleanup boundary for the native Go backend migration.
It is an ownership and retention record, not a replacement for the API
contract or database migration history.

## Removed verified remnants

| Area | Evidence | Disposition |
| --- | --- | --- |
| `lib/replit-auth-web` | No source, package, workflow, or generated consumer imports it. Its helper still pointed at the removed TypeScript `/api/login` and `/api/logout` surface. | Removed. Go owns browser authentication and sessions. |
| `lib/integrations-openai-ai-server` | No runtime, script, artifact, or generated consumer imports it. The package constructed an OpenAI client at module load and duplicated an unreferenced provider mirror. | Removed. Provider access remains behind active Go-owned routes and the AssemblyAI frontend flow. |
| `lib/integrations/openai_ai_integrations` | No package manifest or repository consumer references this source mirror. Its server files constructed OpenAI clients but were not reachable from a workflow or build. | Removed with the unused server package mirror. |
| Appwrite startup ping | `main.tsx` invoked the ping on every browser startup, but no product feature consumed its result and the endpoint was not part of the current runtime boundary. | Removed the startup side effect, client module, and frontend dependency. |

## Retained by design

| Area | Owner | Reason and removal condition |
| --- | --- | --- |
| `lib/db/drizzle/` and `lib/db/src/schema/` | Go backend migration owner | Historical Drizzle migrations and schema snapshots are recovery and comparison evidence, not runtime authority. Keep them in the protected archive below; do not use their commands against production. They may be retired from the working tree only after the Go migration baseline is versioned, production backups have been restored successfully to a disposable PostgreSQL instance, the restore is usable without Drizzle, and the archive location and retention owner are recorded. |
| `lib/db` package and Drizzle commands | Go backend migration owner | Offline comparison tooling is not required by the Go runtime. Retire the package only together with the archived schema after the recovery gate above passes; preserve the SQL migrations and schema snapshots even if the executable/tooling wrapper is removed. |
| `lib/api-client-react`, `lib/api-zod`, and OpenAPI outputs | Frontend/API contract owner | Retained because active frontend pages import the generated client and schemas. |
| `/api/google/*` aliases | Go Google integration owner | Retained until active callers are migrated and route smoke tests prove the compatibility surface is no longer needed. |

## Verified database-history archive

The historical Drizzle file set is retained in the private GitHub repository
`0xTariq-dev/Askolo` at
`refs/heads/archive/db-history/2026-09-25`. The branch is pinned to commit
`1ce174d19cc62d03b27f55bd54ceef3fa4c60dea`. Its active repository ruleset,
`Immutable historical Drizzle schema archive` (ID `24013168`), blocks ref
updates and deletion with no bypass actors. The Go backend migration owner is
responsible for retention.

`scripts/database-history-archive.manifest` records the expected path and Git
blob ID for every archived migration, journal, snapshot, and schema module.
After fetching the protected ref, verify both its pinned commit and the complete
file inventory with:

```sh
git fetch github refs/heads/archive/db-history/2026-09-25
bash scripts/verify-db-history-archive.sh FETCH_HEAD
```

The pre-push hook also rejects pushes targeting this archive ref. Repository
administrators can change repository rulesets, so changing or removing this
protection is an explicit retention-policy action, never unrelated cleanup.
Production backups and tested restore procedures remain separate recovery
artifacts and must not be removed with the archive.

## Database history retirement gate

The Go backend is the sole owner of production database access, schema
changes, and migrations. The historical Drizzle files have no runtime
authority and must not be imported into new product code. Their absence from
the active working tree does not authorize deleting the recovery record: the
versioned SQL migrations and schema snapshots must remain recoverable from a
protected Git ref or an equivalent immutable archive.

Retire the archived Drizzle history only when all of these conditions are
confirmed by the backend owner:

1. The Go migration baseline and all subsequent migrations are versioned and
   can describe the production schema without relying on Drizzle.
2. A current production backup has been restored successfully to a disposable
   PostgreSQL instance, and the Go service can start and exercise its critical
   database paths against that restore.
3. The restore procedure, backup retention policy, archive location, and
   named retention owner are documented outside the retired package.
4. The immutable archive has been checked for completeness, including every
   SQL migration, migration journal, schema snapshot, and schema module needed
   to compare the historical database shape.
5. The backend owner has made and recorded the explicit data-retention decision
   to retire the working-tree copy and any Drizzle tooling.

Until every condition is met, preserve the archive and do not remove its
migration history as part of unrelated cleanup. Retiring the working-tree
copy never permits deleting production backups, tested restore procedures, or
the immutable historical archive.

## Authority checks

- The Go service owns authentication, authorization, sessions, database writes,
  and provider boundaries.
- The frontend calls same-origin `/api` routes and contains no competing
  TypeScript session issuer.
- No TypeScript backend process or provider startup workflow remains.
- Archived Drizzle files, when accessed from recovery history, do not grant
  runtime authority and must not be imported into new product code.