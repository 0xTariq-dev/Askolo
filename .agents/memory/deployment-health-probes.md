---
name: Deployment health probes
description: Publishing startup probes must use a cold-start-safe health endpoint, not readiness that depends on transient runtime state.
---

Use a liveness-style endpoint such as `/healthz` for publishing startup probes. Keep `/readyz` for operational readiness checks that may depend on database state or a provider handoff that is not available immediately after a fresh process starts.

**Why:** A fresh Autoscale instance can be healthy enough to serve traffic while a transient readiness monitor is still unknown; probing that endpoint causes otherwise successful builds to fail promotion.

**How to apply:** Configure the API service's production startup probe explicitly in its artifact configuration, and preserve the production run command at the `[services.production]` scope before defining nested health tables.