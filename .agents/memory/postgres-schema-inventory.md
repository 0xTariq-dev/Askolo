---
name: PostgreSQL schema inventory fingerprints
description: Durable guidance for versioned schema fingerprints built from PostgreSQL system catalogs.
---

Treat a PostgreSQL catalog inventory as a versioned, engine-specific contract. Validate the complete inventory query and deparser output against a disposable instance of the exact supported PostgreSQL major before pinning a digest. Go compilation and unit tests do not validate catalog columns, catalog type coercions, or server-generated SQL definitions. Preserve readers for older stored fingerprint formats so an inventory upgrade does not strand existing migration histories.

An exact migration ledger can still disagree with a live inventory when the fingerprint includes physical column order or schema-owned extension objects. Before treating that as application-schema drift, compare logical application objects after normalizing those inventory-only differences. Reconcile only the active ledger tip after verifying structural equivalence and the complete migration prefix.

**Why:** Catalog shapes and deparser output vary between PostgreSQL majors; several inventory query issues only surfaced when executed against the actual PostgreSQL 16 catalog.

**How to apply:** Pin the supported major with the baseline contract and run its inventory tests. If an unrelated migration gate blocks the full suite, run the targeted schema-compatibility test before updating a digest; never infer acceptance from a hash alone. For ledger reconciliation, confirm the logical schema diff and exact applied checksums before changing the active fingerprint.