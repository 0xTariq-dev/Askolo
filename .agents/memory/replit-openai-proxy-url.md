---
name: Replit-managed OpenAI loopback proxy
description: The Replit AI Integrations OpenAI base URL may point to an HTTP loopback proxy on port 1106.
---

Replit-managed OpenAI integration in this workspace can expose its proxy as `http://localhost:1106`. A blanket HTTPS-only check rejects this valid managed endpoint.

**Why:** The backend planner was unavailable even though the managed integration was configured. The endpoint is a local proxy, not an ordinary remote HTTP service.

**How to apply:** Permit HTTP only for loopback hosts on port 1106; continue requiring HTTPS for other hosts. Do not log or expose the configured URL or integration key.