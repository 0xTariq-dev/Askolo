# Go migration runbook

The backend never executes DDL. Run `go run ./cmd/askolo-migrate -target
development` with a disposable PostgreSQL database. PostgreSQL 14+ is
supported for migration execution (the release validation target is PostgreSQL
16). Baseline adoption is narrower: inventory contract version 1 was generated
and validated on PostgreSQL 16.10, so `-adopt-existing` accepts PostgreSQL
16.x only.

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

For staging, production, and restore targets, set `ASKOLO_MIGRATION_APPROVED=yes`
only after backup verification, change review, and target identity verification.
The current Repl is development-only; production credentials, target identity,
release ownership, and a tested restore are unresolved blockers. Do not use a
live URL from this workspace. Restore a backup to disposable PostgreSQL first,
run the CLI with `-target restore`, then exercise the service before release.

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