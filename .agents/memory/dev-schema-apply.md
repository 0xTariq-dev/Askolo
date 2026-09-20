---
name: Development schema application
description: A workspace-specific Drizzle migration behavior observed while validating schema changes.
---

For development-only schema validation, the checked-in Drizzle migration command may exit with status 1 without reporting a useful database error; the project’s `push-force` script successfully applies the same schema.

**Why:** The native auth runtime needs the new tables present in the development database before authenticated smoke tests can exercise the feature.

**How to apply:** Prefer the project’s documented post-merge schema flow; if a local development verification is explicitly needed, distinguish development from production and inspect the database after applying the schema.