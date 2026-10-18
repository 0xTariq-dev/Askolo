---
name: Development migration prefix
description: Ensure the guarded live-Development migration command includes newly added schema migrations.
---

The guarded `development-live` target applies only `RunOperational`'s activated migration prefix. A successful CLI exit does not prove the newest embedded migrations were applied.

**Why:** The operational prefix can intentionally lag behind the migration files, leaving a newly added schema absent while the command reports success.

**How to apply:** Before a live Development migration, verify the active prefix includes every migration required by the change, advance it only after disposable PostgreSQL validation, and confirm API schema readiness afterward.
