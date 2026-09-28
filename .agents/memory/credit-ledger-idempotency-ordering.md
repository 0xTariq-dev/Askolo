---
name: Credit ledger idempotency ordering
description: Safe ordering for reservation and refund idempotency under PostgreSQL concurrency.
---

For credit reservations, retain a fast initial idempotency lookup but repeat it after acquiring the user's account lock and before inserting. For refunds, resolve an existing matching refund event before enforcing the remaining-refundable-balance limit. Keep the unique database index as the final concurrency guard.

**Why:** Concurrent same-key reservations can all miss the first lookup, and a valid refund retry may exceed the remaining balance after the original refund has committed.

**How to apply:** When changing these transaction paths, cover concurrent same-key reservations and exact refund retries with PostgreSQL integration tests; assert one reservation and one refund effect.