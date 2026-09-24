---
name: Development schema application
description: A workspace-specific Drizzle migration behavior observed while validating schema changes.
---

This is historical, development-only Drizzle behavior, not guidance for the current schema-migration path: the old migration command could exit with status 1 without a useful database error, while a legacy force-push command applied the schema.

**Why:** The behavior was observed while validating native-auth tables in a development database; it does not establish that force-pushing is safe for other environments or current schema changes.

**How to apply:** Use this note only when diagnosing the retired Drizzle workflow. Do not carry its force-push workaround into staging or production; follow the current migration plan for new schema changes and verify the resulting development schema.