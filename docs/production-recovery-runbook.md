# Production database recovery runbook

## Scope and current status

This runbook covers Askolo's current Replit-managed production PostgreSQL
database. A separate externally managed PostgreSQL instance is a future
environment; no provider or target has been selected or connected.

This is an initial, non-operational runbook. Production backup settings have not
been inspected, no restore has been performed, and neither zero RPO nor a
one-hour RTO has been demonstrated. Do not treat this document as evidence that
either objective is met.

## Replit-managed recovery behavior

Replit's documentation describes these capabilities for production PostgreSQL:

- Point-in-time restore (PITR) to a selected time.
- Scheduled daily backups can be enabled.
- A seven-day default recovery window, extendable up to 28 days for Pro and
  Enterprise plans.
- A production restore switches the existing database to the selected point in
  time; it is not a separate restored database for rehearsal.
- Database recovery does not restore application code. Restore the matching
  application checkpoint separately and republish it.

The documentation does not specify guaranteed RPO or RTO values. PITR or daily
backups alone therefore do not prove zero data loss or restoration within one
hour. Confirm the effective plan, retention, backup schedule, and available
restore options in the production project before setting recovery expectations.

References:

- [Replit data recovery](https://docs.replit.com/features/data-and-storage/data-recovery)
- [Replit development and production databases](https://docs.replit.com/features/data-and-storage/development-and-production)

## Safety boundaries

- This development Repl is not a production recovery environment. Do not attach
  production credentials or perform a restore from it.
- Do not initiate PITR or scheduled-backup restore on the live production
  database as a rehearsal. The documented operation changes the live database.
- Any live recovery operation requires explicit operator approval, verified
  target identity, a current pre-restore backup, an agreed maintenance window,
  and a rollback and communications plan.
- Use sanitized fixtures for application-level restore checks. Do not copy
  credentials, provider tokens, email bodies, transcripts, or raw personal data
  into development or test systems.
- Replit Publish owns schema synchronization for Replit-managed production.
  Never run the Go migration runner against that production database. The Go
  runner remains appropriate for disposable/development databases and a future
  externally managed PostgreSQL target after it is selected and approved.

## Recovery procedure to validate

The following is a planning checklist, not authorization to operate on
production:

1. Identify the production Repl and database, effective plan, configured
   retention, enabled backup schedule, and most recent available restore point.
2. Define the incident start and completion timestamps, the committed
   transaction boundary, acceptable data-loss measurement, and excluded
   downtime.
3. Establish an isolated, privacy-safe restore target through a supported
   procedure. Replit's documented production restore is in-place, so do not
   assume it can produce a disposable clone.
4. If an isolated restore target cannot be established, record the safe
   rehearsal and RTO proof as blocked. Do not substitute estimates or a live
   production restore without explicit operator approval.
5. For an approved recovery, record the selected restore point and timestamps.
   Restore the matching application-code checkpoint separately and republish it
   as required; database restore and application-code rollback are separate
   operations.
6. Validate service startup, readiness, authentication, sessions, credit
   settlement, connected-provider state, durable automations, webhook
   deduplication, and audit history. Use privacy-safe test data.
7. Record the observed recovery point, any missing acknowledged transactions,
   restore duration, application recovery duration, validation result, operator,
   code checkpoint, and unresolved gaps.
8. Report zero RPO and one-hour RTO as met only when the defined boundary is
   supported by repeatable measured evidence. Otherwise state the exact unmet
   condition and remediation.

## Open validation items

| Item | Current evidence |
| --- | --- |
| Effective production retention and plan | Not inspected |
| Scheduled daily backup configuration and freshness | Not inspected |
| Production PITR availability and restore-point granularity | Feature is documented; project settings are not inspected |
| Isolated restore target for a safe rehearsal | Not established |
| Zero-RPO guarantee | Not documented by Replit and not measured |
| One-hour RTO | Not documented by Replit and not measured |
| Matching application-code rollback and republish | Not tested |
| Go API readiness against the managed production schema | Not verified; do not resolve by running Go DDL against production |

## Schema and migration boundary

Replit Publish synchronizes the managed production schema from development.
This task must validate recovery without bypassing that mechanism. The Go
migration runner is for disposable/development databases and, in the future,
an explicitly selected external PostgreSQL target. It must not be used to
restore or migrate Replit-managed production.

Before claiming that a restored managed database is application-ready, verify
the Go API's readiness expectations against a safe non-production schema. If
the managed Publish flow and the Go migration-ledger/fingerprint checks are
incompatible, record and resolve that gap without applying production DDL from
this development Repl.