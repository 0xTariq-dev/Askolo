---
name: Same-origin auth routing
description: Development previews may use one host for both public and authenticated app routes.
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