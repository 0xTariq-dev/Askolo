---
name: Same-origin auth routing
description: Development previews may use one host for both public and authenticated app routes.
---

When the public and app origins resolve to the same host, app paths such as
`/sign-in` and `/dashboard` must render the native app router locally instead
of hard-redirecting to the same URL.

**Why:** A public-host redirect to an identical app-host URL creates an
infinite browser refresh loop, especially in development previews where both
origins default to the current host.

**How to apply:** Keep cross-host redirects for separated production hosts,
but detect the same-host case before redirecting non-public paths.