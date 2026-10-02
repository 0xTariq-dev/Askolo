---
name: Workspace-scoped package installs
description: Installing dependencies into an individual pnpm workspace package in Replit.
---

The Replit language-package callback can attempt to add a package at the pnpm workspace root and fail with `ERR_PNPM_ADDING_TO_ROOT` when the dependency belongs to one artifact.

**Why:** The workspace root guard prevents an accidental unscoped dependency install.

**How to apply:** For an artifact-only dependency, install it with a package-filtered pnpm command, pin the version, and run the workspace audit.