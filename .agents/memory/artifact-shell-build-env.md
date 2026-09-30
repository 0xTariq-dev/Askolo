---
name: Manual artifact build environment
description: Required environment when building an artifact package outside its managed workflow.
---

When a Vite artifact is built directly with `pnpm` from the shell, its managed workflow environment is not inherited. Read the artifact manifest and supply required values such as `PORT` and `BASE_PATH` to the build command.

**Why:** Replit injects manifest-configured values into the managed artifact workflow, while a standalone shell command runs without that environment and Vite config may fail before the build starts.

**How to apply:** Before manually running an artifact's build, inspect its current `[services.env]` values and pass them in for that command. Do not hardcode local build values into app scripts or manifests.