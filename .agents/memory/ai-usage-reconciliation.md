---
name: AI usage reconciliation review
description: Idempotency and safe admin handling for estimated or missing provider usage.
---

- Reconciliation replay must compare the provider-reported usage, request ID, actor, and reason against the original event. Do not recompute and compare the original estimate from the reservation after reconciliation, because the settled amount has already been replaced by measured usage.
  **Why:** The first successful reconciliation changes the reservation's settled amount; recomputing the estimate from that amount creates a false idempotency conflict.
  **How to apply:** Keep the original estimate in the audit event and treat matching measured evidence as a no-op replay.
- Keep non-Azure reservations with no provider usage evidence pending while their reservation is active. Allow an administrator to release the hold only after it expires; only use an explicit verified-usage settlement path when reliable evidence exists.
  **Why:** Releasing an active hold can make an in-flight provider operation free, while missing usage must not be billed as if it were measured.
  **How to apply:** Enforce expiry on the server and expose the same condition in the admin review UI.
