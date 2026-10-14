---
name: Postgres local-time strings
description: Preserve agenda calculations when PostgreSQL time values are scanned into strings.
---

When a PostgreSQL `time` column is scanned into a Go string, it may include seconds (for example, `23:30:00`) even if the API accepted `HH:mm`. Full-row writes built from PATCH reads must normalize those values before strict API validation. Likewise, normalize a JSONB `null` value back to SQL NULL before preserving it in a full-row update when the column constraint requires an object or SQL NULL.

**Why:** Cross-midnight Agenda overlap checks and otherwise valid task PATCHes failed when persisted local times or JSON nulls were fed back into stricter write validation and database constraints.

**How to apply:** Parsing code for database-backed local times should accept both `HH:mm` and `HH:mm:ss`; keep API input validation strict. Before merging a PATCH into a complete database write, normalize stored time strings and distinguish SQL NULL from JSONB `null`.
