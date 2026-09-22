---
name: Publish path smoke isolation
description: Release smoke checks must coexist with the managed preview and API process.
---

The publish-path smoke check must select ports immediately before launching its own
servers, validate explicit overrides, and isolate the backend PID file while
building so the active development API is never stopped.

**Why:** The managed preview owns fixed frontend/API ports, and the API build
script otherwise interprets its PID file as permission to stop the running
development backend.

**How to apply:** Keep artifact development ports unchanged; make validation
processes self-contained and restore the original PID file after the build.