---
name: Go backend binary refresh
description: The Askolo Go run workflow reuses an existing compiled binary.
---

Rebuild the Go backend binary before restarting its run workflow after source changes; the run script only builds when the binary is missing. The build script stops the existing process before running its validation sequence, so a timeout there can leave the API down and the old binary in place. Run the gates first or allow enough time for the full script to finish.

**Why:** Restarting without rebuilding can serve stale routes, while timing out during the build script's repeated test gates can stop the old process before the replacement binary exists.

**How to apply:** Complete the relevant validation, build the service binary successfully, then restart the Go run workflow and verify the route through the API proxy.