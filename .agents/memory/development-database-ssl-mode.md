---
name: Development database SSL mode
description: The Development database connection URL explicitly disables PostgreSQL SSL.
---

The Development PostgreSQL connection URL includes `sslmode=disable`. This is a Development-only fact; it does not establish Production's SSL configuration.

**Why:** the user asked us to note it.

**How to apply:** Preserve the explicit setting when diagnosing Development database connections. Verify Production's connection configuration independently instead of extrapolating from Development.
