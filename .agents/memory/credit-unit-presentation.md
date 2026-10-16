---
name: Credit-unit presentation
description: User-facing unit boundaries for AI credits while preserving USD pricing and ledger storage.
---

Present grants, credit balances, and account-ledger counters in credits. Keep provider rate cards, estimates, and operation receipts in USD; do not disclose the conversion rate to users. Convert only at the UI/input boundary and keep the existing integer USD-microunit API and ledger contract unchanged.

**Why:** the product uses credits for grants and balances while operators continue to manage real provider costs in dollars.

**How to apply:** use exact fixed-point conversion helpers for grant and balance display/input; keep USD formatting and validation for provider rates and usage charges.
