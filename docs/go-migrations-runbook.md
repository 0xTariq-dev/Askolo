# Go migration runbook

## Scope

This runbook covers disposable and development databases plus externally
managed PostgreSQL targets. It does not authorize running Go migrations against
Replit-managed production PostgreSQL. Replit Publish synchronizes structural
changes from development to production, and Replit documents no supported
setting to disable that behavior. See
[Development and production databases](https://docs.replit.com/features/data-and-storage/development-and-production).

Production currently uses Replit-managed PostgreSQL. A separate external
PostgreSQL target is a future environment and is not selected or connected yet.
For current production recovery constraints, see
[`production-recovery-runbook.md`](production-recovery-runbook.md).

The API service never executes DDL. Run the explicit migration CLI against a
disposable PostgreSQL database with `-target development`. PostgreSQL 14+ is
supported for migration execution (the release validation target is PostgreSQL
16). Baseline adoption is narrower: inventory contract version 1 was generated
and validated on PostgreSQL 16.10, so `-adopt-existing` accepts PostgreSQL
16.x only.

## Authoring a schema change

When a backend change needs a schema update:

1. Add the next sequentially numbered SQL file under
   `services/askolo-backend/internal/migrations/sql/`, using the existing
   `NNNN_short_description.sql` naming convention. The Go runner embeds these
   files automatically.
2. Do not edit a migration that has been committed or applied to a shared
   database. Make corrections with a new forward migration.
3. Keep DDL and its migration-ledger update within the runner's transaction.
   Review data-preservation, locking, and compatibility effects; do not rely on
   `CREATE TABLE IF NOT EXISTS` at application startup to repair a schema.
4. Add or update integration coverage for the new schema behavior. The tests
   must use the disposable PostgreSQL instance created by
   `scripts/test-migrations.sh`, not the application `DATABASE_URL`.
5. Run
   `GOSUMDB=sum.golang.org bash ./scripts/test-migrations.sh` from
   `services/askolo-backend` and run the relevant backend tests before handing
   off the change.

Use this Go runner for external staging or production only after the target
database has been identified and the release owner has verified a backup and a
successful restore. A production run is a separate, explicitly approved release
step, never part of API startup or the ordinary application build. This
development Repl must not be given a live production database URL.

The versioned `inventory-v1` fingerprint is scoped to `current_schema()` and
does not depend on that schema's name. It inventories relations, columns and
defaults, constraints, indexes, sequences and ownership, views and rules,
partitioning, triggers, row-security policies, custom types, routines,
aggregates, operators and their families, collations, conversions, text-search
objects, extensions in the selected schema, and extended statistics. It
excludes application data, the migration ledger itself, object owners, grants,
comments, database-wide settings, and other database-wide objects such as casts
and publications. Adoption refuses any database-wide event trigger because it
could intercept ledger DDL. Other excluded target-specific properties require
separate review. Never treat a matching schema fingerprint as evidence that
roles, privileges, extensions outside the selected schema, or live data are
safe.

The CLI embeds numbered SQL, checks SHA-256 checksums, rejects gaps, renamed or
edited applied migrations, and serializes operators with an advisory lock.
Every migration and its history row commit in one transaction. A failed
migration is visible and leaves no history row.

The API's `/readyz` endpoint returns 503 until every embedded migration is
recorded in order with matching names and checksums and the current schema
fingerprint matches the latest migration record. It reports only a readiness
boolean to callers; detailed failures are logged server-side only when the
readiness state changes. The check is read-only and uses the health request's
bounded database context.

The backend build runs `scripts/test-migrations.sh`. It creates a temporary
PostgreSQL 16.x instance with a private Unix socket, runs the full migration
integration suite (including the restricted-role runner test), and exercises
the development and approved restore CLI targets. The script removes the
temporary cluster on exit and clears `DATABASE_URL` before integration tests,
so it cannot use a live application database.

For external staging, production, and restore targets, set
`ASKOLO_MIGRATION_APPROVED=yes` only after backup verification, change review,
and target identity verification. The current Repl is development-only;
production credentials, target identity, release ownership, and a tested restore
are unresolved blockers. Do not use a live URL from this workspace. Restore a
backup to disposable PostgreSQL first, run the CLI with `-target restore`, then
exercise the service before release. Do not use these commands to bypass
Replit-managed production schema synchronization.

Forward fixes are new numbered migrations; never edit an applied file. The
clean baseline is strict and rejects an already-populated database. Existing
schema adoption is available only for the exact archived baseline: contract
version 1 pins archive commit
`1ce174d19cc62d03b27f55bd54ceef3fa4c60dea`, the checksums of migrations 0000
and 0001, and their schema inventory fingerprint. The target must have no
migration ledger and must match exactly. Do not supply an operator-computed
digest as proof. Adoption creates the ledger and records both migration rows
inside the caller's transaction without replaying DDL or changing application
data; the ordinary runner then applies later migrations such as 0002.

New history rows carry a versioned fingerprint. Older unversioned histories
retain a legacy drift check only so the runner can advance them; they are not
accepted for baseline adoption. Unexpected drift must be investigated and
	repaired by a reviewed migration. The AI ledger migration preserves existing
balances and only adds absent zero-default counters; it does not activate live
session-duration billing.