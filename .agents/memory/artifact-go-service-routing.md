---
name: Artifact-routed Go service
description: Replit web artifacts can proxy a second Go service for API paths.
---

The frontend artifact can own both static web delivery and a Go API service by
declaring a second service with API paths in its artifact manifest. Development
commands run from the artifact directory or its `.replit-artifact` context, so
workspace-level Go commands must explicitly change to the repository root before
invoking `services/...` paths. The Go build script must still resolve its own
location and change into the module root.

**Why:** Removing the standalone API artifact otherwise leaves the production
static router without a public API edge, and relative service paths fail during
managed workflow startup or publishing when they assume the wrong working
directory.

**How to apply:** Keep the Go service on its own local port and route `/api`,
health, websocket, and webhook paths to it. Validate the manifest through the
artifact replacement flow, prefix root-relative production commands with
`cd ../..`, and make the Go build script `cd` to its service root before
running module commands.