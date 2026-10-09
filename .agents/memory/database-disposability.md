---
name: Database disposability
description: Records the user's scope clarification for Askolo development databases and migration-runner test schemas.
---

Development and Production databases are disposable. `migration_test` is a disposable migration-runner test schema intended for later migrations outside Replit. Prioritize product development over preserving these databases.

**Why:** the user clarified that preserving these databases is less important than progressing toward the product goal.

**How to apply:** do not treat preservation of these databases or `migration_test` as a blocker to product work. Disposability alone does not authorize applying a specific migration or performing a destructive operation; follow the user's current request.
