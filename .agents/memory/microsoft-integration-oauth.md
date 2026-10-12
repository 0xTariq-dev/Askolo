---
name: Microsoft integration OAuth
description: Agreed callback-path and environment-variable conventions for future Microsoft Graph integration.
---

**Rule:** the user has configured the Microsoft Entra app and its permissions. The Replit secret names are `MS_CLIENT_ID` and `MS_CLIENT_SECRET`; use those exact names, never request or record their values. The planned Microsoft Graph integration callback is `/api/integrations/microsoft/callback`, with its public origin derived from `ASKOLO_CANONICAL_ORIGIN` as the Google integration does. The callback route is not implemented yet.

**Why:** the user chose existing `MS_CLIENT_*` names for the configured app credentials; the callback path follows the existing Google integration pattern and avoids proxy-derived OAuth origins.

**How to apply:** when implementing Microsoft Graph authorization, read `MS_CLIENT_ID` and `MS_CLIENT_SECRET`, register the callback against the environment's canonical origin, and do not advertise the redirect as live until the backend route exists.
