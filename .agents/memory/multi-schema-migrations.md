---
name: Multi-schema migration fingerprints
description: Safe search_path handling and disposable namespace rules when a migration manages more than one PostgreSQL schema.
---

When a fingerprint reads multiple PostgreSQL schemas by changing `search_path`, restore the exact original path before the migration writes its history row or performs any further unqualified SQL.

**Why:** Leaving the path on the final inspected schema redirects unqualified writes such as the migration-ledger insert. This caused successful DDL to roll back at the history insert.

**How to apply:** Capture `current_setting('search_path')`, inspect each schema with a local path override, and restore the captured value before returning a successful combined fingerprint. Disposable test cleanup must drop both the source namespace and its derived companion namespace; all database-backed Go tests must use the disposable PostgreSQL harness, not a shared application database.

Keep the runtime API health gate on the active pre-v2 migration prefix while `public_v2` remains staged; run the full migration readiness check only against disposable migration targets until promotion.

PostgreSQL creates indexes in their table's schema; `CREATE INDEX` does not accept a schema-qualified index name. Qualify the indexed table, not the index identifier.
