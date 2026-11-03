---
name: Isolated OAuth preview hosts
description: Task-specific development hosts may be rejected by OAuth and anti-abuse provider configuration.
---

**Rule:** Before real OAuth testing on an isolated task preview, confirm the exact callback URI is registered with the provider and the host is accepted by Turnstile. If not, treat the provider rejection as an environment blocker and run the real flow on the canonical Development origin after merge.

**Why:** A task-specific host can trigger `redirect_uri_mismatch`, while a Turnstile widget may not accept a newly created host. Neither response alone proves an application-code defect.

**How to apply:** Prefer the configured canonical Development origin for real OAuth verification. Do not bypass authentication or anti-abuse checks to make an isolated preview work.
