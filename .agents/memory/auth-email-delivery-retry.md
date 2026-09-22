---
name: Auth email delivery retry safety
description: How native auth challenge persistence should behave when provider handoff fails.
---

Release a newly persisted email challenge only when the sender classifies the failure as retry-safe; retain it when handoff may have succeeded so cooldowns prevent duplicate sends.

**Why:** Provider connection and delivery failures can block a user if their pending challenge remains, while retrying after an uncertain handoff can send duplicate messages.

**How to apply:** Keep delivery classification explicit at the sender boundary, bound retry cleanup independently of the request context, and expose only aggregate outcome counts and categories.