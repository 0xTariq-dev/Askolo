---
name: Assistant message ordering
description: Preserve chronological order for assistant messages and audit evidence created in one transaction.
---

When one transaction inserts multiple assistant messages that must render chronologically, use `clock_timestamp()` instead of PostgreSQL `NOW()`. `NOW()` is fixed at transaction start, so same-transaction inserts tie and a secondary ID sort can scramble their visible order. Use statement-time timestamps for audit events when their relative ordering matters as well.

**Why:** Persisting the user's transcript and the assistant's response in one transaction with transaction-start timestamps made the conversation's order depend on generated IDs.

**How to apply:** Use `clock_timestamp()` for ordered assistant message and audit inserts, and cover same-run ordering in integration tests.