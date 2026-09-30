---
name: Auth email delivery retry safety
description: How native auth challenge persistence should behave when provider handoff fails.
---

Release a newly persisted email challenge only when the sender classifies the failure as retry-safe; retain it when handoff may have succeeded so cooldowns prevent duplicate sends. Keep anonymous signup/resend feedback indistinguishable across addresses, and expose exact asynchronous provider events only through trusted operational tooling.

**Why:** Provider connection and delivery failures can block a user if their pending challenge remains, while retrying after an uncertain handoff can send duplicate messages. Per-recipient delivery events exposed to anonymous callers can also reveal whether an address maps to an account.

**How to apply:** Keep delivery classification explicit at the sender boundary, bound retry cleanup independently of the request context, and expose only aggregate outcome counts and categories to public callers.