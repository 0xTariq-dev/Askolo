---
name: PostgreSQL test search paths
description: The URL encoding requirement for schema-isolated PostgreSQL integration tests using pgx.
---

When a pgx/libpq connection URL uses the `options` parameter to set a schema search path, encode spaces as `%20` rather than relying on `url.Values.Encode`'s `+` representation.

**Why:** PostgreSQL can receive the plus sign literally in startup options and interpret `+search_path` as an unknown configuration parameter.

**How to apply:** For disposable schema fixtures, create the schema first, then pass an explicitly percent-encoded `options=-c%20search_path=...` value to both the store and assertion pool.