---
name: Go module installer fallback
description: A workspace-specific fallback when the package-management callback cannot install Go modules.
---

If the package-management callback returns `no such language: go`, install the pinned module from the service module with Go's module command, then run `go mod tidy`.

**Why:** the callback rejected the documented Go language even though the Go toolchain and module registry were available; the standard Go module command succeeded.

**How to apply:** use this only after the callback fails, pin the selected dependency version, and verify the resulting module files and tests.
