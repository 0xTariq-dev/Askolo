---
name: Lazy credit expiry
description: Credit reservations expire on demand during the next reservation transaction rather than through a process-wide polling loop.
---

The backend must remain the authority for credit consumption. A client balance check is advisory; the actual reservation must atomically lock the relevant ledger state, expire that user's stale reservations, and enforce the resulting balance.

**Why:** A 30-second process-wide reconciliation query kept the production database active even when no AI work was happening. Removing it without lazy expiry would leave abandoned reservations blocking credits.

**How to apply:** Keep reservation/account operations transactional. Acquire stale reservation locks before the account lock so settlement and reservation paths use a consistent lock order and avoid deadlocks under concurrent clients.