---
name: Production autoscale configuration
description: The Go API must receive its production identity and internal-auth configuration before Autoscale promotion.
---

The API can compile and pass local tests while still failing Autoscale promotion if production environment variables are absent. In particular, production startup requires an explicit environment, canonical HTTPS origin, database identity, cookie namespace, release metadata, normal/hotfix release mode, and a separate internal-auth token.

**Why:** The deployment runner health-checks every API path during service creation; a configuration-exit becomes repeated HTTP 500 responses and is reported only as an Autoscale crash loop.

**How to apply:** Before publishing, verify production environment names and secret existence without printing secret values, then run the binary in production mode and confirm `/healthz` returns 200.