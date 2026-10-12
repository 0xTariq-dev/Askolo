---
name: Microsoft integration OAuth
description: Agreed callback-path and environment-variable conventions for future Microsoft Graph integration.
---

**Rule:** use `/api/integrations/microsoft/callback` for the Microsoft integration callback, with `MICROSOFT_INTEGRATION_CLIENT_ID` and `MICROSOFT_INTEGRATION_CLIENT_SECRET`. Derive the public redirect origin from `ASKOLO_CANONICAL_ORIGIN`, as the Google integration does. The callback route is planned but is not implemented yet.

**Why:** this matches the existing integration callback and client-credential naming pattern while avoiding proxy-derived OAuth origins.

**How to apply:** when implementing Microsoft Graph authorization, register the callback against the environment's canonical origin and do not advertise the redirect as live until the backend route exists.
