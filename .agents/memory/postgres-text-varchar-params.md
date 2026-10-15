---
name: PostgreSQL text and varchar parameters
description: Avoid inferred-parameter type conflicts in INSERT ... SELECT statements that mix text and varchar columns.
---

When one bind parameter is reused in an `INSERT ... SELECT` expression and a predicate, explicitly cast each use to the corresponding column type if the schema mixes `text` and `varchar`. PostgreSQL can otherwise report SQLSTATE 42P08 because the same parameter is inferred as both types.

**Why:** A disposable PostgreSQL integration test exposed this when a group member ID was selected into a `text`-adjacent insert expression and compared against a `varchar` membership column.

**How to apply:** Check the declared types of every target and predicate column when building parameterized `INSERT ... SELECT` statements; add explicit casts and verify with a database-backed test rather than compile-only tests.
