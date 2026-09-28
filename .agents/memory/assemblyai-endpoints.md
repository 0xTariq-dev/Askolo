---
name: AssemblyAI endpoint split
description: Temporary-token requests use HTTPS and browser sessions use WSS; global-edge routing does not guarantee country residency.
---

Use an HTTPS token endpoint and a separate WSS session endpoint for the same selected AssemblyAI region. Do not treat the global Edge endpoint as a US or EU residency guarantee.

**Why:** Temporary tokens are minted over HTTP while realtime sessions connect over WebSocket, and endpoint geography determines the processing-residency claim. The global Edge route may select a nearby region but is not country-pinned.

**How to apply:** Preserve the HTTPS/WSS split and label the selected region accurately. Add user-specific country routing only after the country contract and missing-country behavior are decided.