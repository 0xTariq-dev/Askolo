---
name: Password recovery verification
description: Password recovery uses a non-consuming verification step followed by one-time consumption during the final password reset.
---

The recovery flow starts with the primary account email. A selected method enum determines whether the backend sends to that primary email or resolves an already enrolled recovery email; the recovery address is never accepted as the account identifier.

**Why:** Letting users enter a recovery address directly can target the wrong account or bypass the intended account-ownership flow. A sequential UI also needs a real server-side code step, but consuming the code before the password is submitted would require a separate short-lived authorization record.

**How to apply:** Keep the primary email and selected method in every recovery request, resolve delivery server-side, validate without consuming on the code page, then atomically consume during reset. Preserve generic responses, attempt counting, expiry, and no sensitive logging.