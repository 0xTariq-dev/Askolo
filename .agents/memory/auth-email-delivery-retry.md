---
name: Auth email delivery retry safety
description: How native auth challenge persistence should behave when SMTP handoff fails.
---

Release a newly persisted email challenge only when the sender classifies the failure as retry-safe; retain it when handoff may have succeeded so cooldowns prevent duplicate sends.

**Why:** SMTP connection and provider failures can block a user if their pending challenge remains, while retrying after an uncertain post-DATA failure can send duplicate messages.

**How to apply:** Keep delivery classification explicit at the sender boundary, bound retry cleanup independently of the request context, and expose only aggregate outcome counts and categories.