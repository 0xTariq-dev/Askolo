---
name: Go toolchain path
description: The workspace exposes Go through a managed wrapper where gofmt may not be on PATH.
---

Use `go fmt ./...` rather than calling `gofmt` directly when formatting Go services in this workspace.

**Why:** The managed Go command is available, but the standalone formatter binary was not discoverable on PATH during sidecar setup.

**How to apply:** Run formatting through the `go` command before Go tests, vet, or builds.