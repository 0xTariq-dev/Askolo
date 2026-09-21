---
name: Go backend binary refresh
description: The Askolo Go run workflow reuses an existing compiled binary.
---

Rebuild the Go backend binary before restarting its run workflow after source changes; the run script only builds when the binary is missing.

**Why:** Restarting without rebuilding can serve stale routes and make valid source changes appear to return 404s.

**How to apply:** Run the service build script, then restart the Go run workflow and verify the route through the API proxy.