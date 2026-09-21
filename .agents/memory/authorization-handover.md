---
name: Authorization handover
description: Go is the authorization decision owner for TypeScript compatibility requests and provider/transport boundaries.
---

The TypeScript API must derive identity from the native session and ask Go for an authorization decision; workspace headers are scope hints only and never establish identity. Production should use a dedicated internal handover token, while development may derive one from the shared session secret when no dedicated token is configured.

**Why:** Keeping the decision in Go prevents the staged TypeScript/Go migration from creating competing membership or role authorities, while the development fallback keeps local workflows usable without adding another secret.

**How to apply:** Preserve generic client denials, session-derived actors, explicit capability checks, and correlation-safe logs whenever adding a new route, provider operation, job, or WebSocket feature.