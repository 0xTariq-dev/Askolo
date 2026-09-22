---
name: Artifact-routed Go service
description: Replit web artifacts can proxy a second Go service for API paths.
---

The frontend artifact can own both static web delivery and a Go API service by
declaring a second service with API paths in its artifact manifest. Managed
artifact commands run from the artifact directory, so commands that reach
workspace-level Go files must explicitly move to the repository root.

**Why:** Removing the standalone API artifact otherwise leaves the production
static router without a public API edge, and relative service paths fail during
managed workflow startup.

**How to apply:** Keep the Go service on its own local port and route `/api`,
health, websocket, and webhook paths to it. Validate the manifest through the
artifact replacement flow and verify the managed service logs after restart.