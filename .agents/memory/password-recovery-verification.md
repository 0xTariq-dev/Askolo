---
name: Password recovery verification
description: Primary-email recovery for active unverified accounts confirms the address and sets the first password atomically.
---

The recovery flow starts with the primary account email. A selected method enum determines whether the backend sends to that primary email or resolves an already enrolled recovery email; the recovery address is never accepted as the account identifier. Unverified accounts qualify only through the primary-email method and only while active; pending signups and recovery-email codes do not verify the primary address. Recovery does not grant signup credits.

**Why:** Letting users enter a recovery address directly can target the wrong account or bypass the intended account-ownership flow. A primary-email code is proof of mailbox access; consuming it, setting the password, and confirming the email in one transaction prevents partial resets and code reuse.

**How to apply:** Keep the primary email and selected method in every recovery request, resolve delivery server-side, validate without consuming on the code page, then atomically consume during reset. Preserve generic responses, attempt counting, expiry, eligibility checks, no signup-credit grant, and no sensitive logging.