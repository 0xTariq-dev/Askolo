---
name: Microsoft integration OAuth
description: User-selected credentials and canonical-origin convention for Microsoft Graph integration.
---

**Rule:** the user has configured the Microsoft Entra app and its permissions. Keep the user-selected `MS_CLIENT_*` credential naming and the canonical-origin convention consistent with Google integration. Never request or record credential values.

**Why:** the user chose existing `MS_CLIENT_*` names for the configured app credentials; the callback path follows the existing Google integration pattern and avoids proxy-derived OAuth origins.

**How to apply:** inspect current code for implemented routes and exact configuration keys; register callbacks against the intended environment's origin rather than assuming an arbitrary preview host is registered with Entra.
