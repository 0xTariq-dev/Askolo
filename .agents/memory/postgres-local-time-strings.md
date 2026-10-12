---
name: Postgres local-time strings
description: Preserve agenda calculations when PostgreSQL time values are scanned into strings.
---

When a PostgreSQL `time` column is scanned into a Go string, it may include seconds (for example, `23:30:00`) even if the API accepted `HH:mm`.

**Why:** Cross-midnight Agenda overlap checks failed when the stored representation did not match the API's minute-only parser.

**How to apply:** Parsing code for database-backed local times should accept both `HH:mm` and `HH:mm:ss`; keep the API's stricter input validation separate.
