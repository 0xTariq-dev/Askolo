---
name: Isolated OAuth preview hosts
description: Task-specific development hosts may be rejected by OAuth and anti-abuse provider configuration.
---

**Rule:** Before real OAuth or Turnstile testing on an isolated task preview, confirm the exact callback URI is registered with the provider and the active hostname is accepted by both the Turnstile site key and backend allowlist. A backend allowlist entry cannot make Cloudflare issue a widget token for an unregistered host.

**Why:** Task-specific hosts can trigger `redirect_uri_mismatch` or fail inside the Turnstile widget before a token reaches the backend. Neither response alone proves an application-code defect.

**How to apply:** Prefer an approved canonical Development origin for real provider verification. Do not bypass authentication or anti-abuse checks to make an isolated preview work.
