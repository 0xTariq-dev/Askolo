# Go migration runbook

The backend never executes DDL. Run `go run ./cmd/askolo-migrate -target
development` with a disposable PostgreSQL database. PostgreSQL 14+ is
supported (the release validation target is PostgreSQL 16). The managed
fingerprint covers public columns, defaults, nullability, constraints, and
indexes; extensions, functions, grants, policies, and non-public objects need
separate review.

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
schema adoption is currently disabled: `-adopt-existing` refuses until an
independently pinned archive contract and target-specific verification are
available. Do not supply an operator-computed digest as proof. Schema
fingerprints are recorded with history and unexpected drift must be investigated
and repaired by a reviewed migration. The AI ledger migration preserves existing
balances and only adds absent zero-default counters; it does not activate live
session-duration billing.