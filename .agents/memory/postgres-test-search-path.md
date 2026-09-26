---
name: PostgreSQL test search paths
description: URL encoding and bootstrap-role requirements for disposable PostgreSQL integration tests using pgx.
---

When a pgx/libpq connection URL uses the `options` parameter to set a schema search path, encode spaces as `%20` rather than relying on `url.Values.Encode`'s `+` representation.

**Why:** PostgreSQL can receive the plus sign literally in startup options and interpret `+search_path` as an unknown configuration parameter.

**How to apply:** For disposable schema fixtures, create the schema first, then pass an explicitly percent-encoded `options=-c%20search_path=...` value to both the store and assertion pool.

For a disposable cluster initialized with `initdb -U <role>`, include that role explicitly in the pgx URL (for example, `postgresql://<role>@/postgres?...`). A hostless URL without a username may default to `postgres`, which can fail even though the server is healthy.

**Why:** `initdb` creates the selected bootstrap role; it does not guarantee a separate role named `postgres`.

**How to apply:** Keep integration tests pointed only at the temporary socket and name its actual bootstrap role in the URL. Do not fall back to the application's `DATABASE_URL`.