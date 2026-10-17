---
name: Signup welcome-credit atomicity
description: Rules for verified signup credits and one-time existing-account backfills.
---

Award signup welcome credit only after the account's identity is verified. For external identity providers, create the user and grant within the same transaction. Record the grant in the USD ledger and update the account balance atomically, using one stable per-user idempotency key for both signup and existing-account backfill paths.

Keep the welcome-grant amount independently configurable by an authorized operator; do not hard-code a default. A zero amount means issuance is disabled until configured. Existing grants remain idempotent if the policy changes later.

**Why:** Issuing credit before verification can reward abandoned or unverified accounts, and separating the ledger entry from the balance update can leave audit records inconsistent with usable credit. The operator owns the default amount, and a shared idempotency key makes retries and backfills safe.

**How to apply:** When adding a signup method or backfilling existing accounts, reuse the verified signup boundary and one transaction for the ledger row and balance update. Read the current configured welcome amount and confirm the active database identity before one-off writes.