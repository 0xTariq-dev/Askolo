---
name: Workspace migration provenance
description: Historical source and safe migration strategy for workspace authorization tables.
---

The pinned archive's Drizzle security schema is an independent source for the workspace authorization tables; the executable Go migration sequence historically omitted those objects. Incorporate them with a forward migration rather than rewriting migrations 0000–0001. Keep the established archive baseline contract intact, and use a distinct, explicitly verified adoption contract when an existing database already contains schema extensions.

**Why:** Rewriting migration bodies invalidates established checksums, while trusting a live fingerprint alone can bless unreviewed drift.

**How to apply:** Compare the archived schema, executable SQL, and live catalog. Preserve existing fingerprint readers; version any new adoption comparator and verify it against a disposable PostgreSQL instance before relying on it.

For checksum-recorded migrations, scope catalog existence checks to the exact target relation. PostgreSQL constraint names are not globally unique, so a name-only `pg_constraint` lookup can mistake a constraint in another schema or table for the target constraint. Preserve the migration source checksum when repairing execution behavior.

**Why:** Schema-isolated migration runs can coexist with other schemas that contain identically named constraints; a global name-only guard may skip local creation and then fail validation.

**How to apply:** Add an explicit relation-scoped regression case with a same-named constraint in a sibling schema, and keep the recorded migration body unchanged unless a reviewed checksum-compatibility path is provided.