---
name: MFA recovery throttling
description: Cross-instance abuse limits for MFA recovery support endpoints.
---

MFA recovery request and verification throttles must be consumed from shared PostgreSQL state, not process-local maps. The shared bucket should contain only a one-way client/stage key, window timestamp, and count; never store account identifiers, emails, or submitted codes in that state.

**Why:** Requests can move between backend instances, and recovery endpoints must keep generic account responses while preventing distributed brute force and request flooding.

**How to apply:** Use an atomic row-locked bucket update for each recovery stage, keep email challenge cooldowns and challenge attempt counters in their existing database transactions, and cover concurrent requests across separate handler instances in integration tests.