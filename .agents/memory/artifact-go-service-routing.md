---
name: Artifact-routed Go service
description: Replit web artifacts can proxy a second Go service for API paths.
---

The frontend artifact can own both static web delivery and a Go API service by
declaring a second service with API paths in its artifact manifest. Development
commands run from the artifact directory, while production commands may run
from `.replit-artifact`; workspace-level Go commands must resolve paths from
their own script and change into the Go module root.

**Why:** Removing the standalone API artifact otherwise leaves the production
static router without a public API edge, and relative service paths fail during
managed workflow startup or publishing when they assume the wrong working
directory.

**How to apply:** Keep the Go service on its own local port and route `/api`,
health, websocket, and webhook paths to it. Validate the manifest through the
artifact replacement flow, use manifest-relative production paths, and make
the Go build script `cd` to its service root before running module commands.