---
name: AssemblyAI endpoint split
description: The official SDK uses separate HTTPS and WSS bases for US streaming temporary tokens and browser sessions.
---

Use the US HTTPS streaming base for the Node SDK temporary-token request and return the separate US WSS `/v3/ws` URL to browser or mobile clients.

**Why:** The SDK’s streaming token factory builds an HTTP `GET /v3/token`, while the streaming transcriber requires a `wss:` URL; using the WSS URL for both fails before token issuance.

**How to apply:** Keep region validation fail-closed and preserve the HTTPS/WSS split whenever the AssemblyAI streaming boundary is reused.