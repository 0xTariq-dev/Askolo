---
name: Same-origin auth routing
description: Same-host public/app navigation and the requested root-domain migration.
---

When the public and app origins resolve to the same host, use Wouter for
same-origin links and keep the public/auth route branch subscribed to router
location changes. App paths such as `/sign-in` and `/dashboard` must render
locally instead of hard-redirecting to the same URL.

**Why:** Plain anchors reload the document even when both destinations share a
host, and a hard redirect to the identical URL can create a refresh loop.

**How to apply:** Keep normal document navigation across different origins.
For same-origin CTAs and footer links, use Wouter links and ensure components
that choose between public and authenticated routes react to `useLocation()`.

**Product direction:** The user wants the application migrated back to the root domain, `askolo.app`.

**Why:** The user requested the root-domain migration while preparing Microsoft app verification.

**How to apply:** Treat the root domain as the requested migration target, but clarify landing-page behavior and obtain publishing approval before changing live domain settings.