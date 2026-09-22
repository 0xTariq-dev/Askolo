---
name: Static Go binaries for Autoscale
description: Go services published to Autoscale must not depend on a Nix-store dynamic loader.
---

Build production Go executables with `CGO_ENABLED=0` unless the service has a verified native-library requirement and the runtime image explicitly supplies those libraries.

**Why:** A dynamically linked binary built in the workspace can embed a `/nix/store/.../ld-linux-*.so.2` interpreter. The deployment image can contain the executable and mark it executable, yet the shell reports `exec: ...: not found` because that interpreter is absent.

**How to apply:** After the production build, inspect the executable with `file` and `readelf -l`; it should be statically linked and have no `Requesting program interpreter` entry. Then run the production command and verify the health endpoint.