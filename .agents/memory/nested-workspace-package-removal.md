---
name: Nested workspace package removal
description: Verify package-removal callbacks against package manifests and the shared pnpm lockfile in nested artifacts.
---

When removing a dependency owned by a nested pnpm workspace package, do not treat a successful package-management callback as proof that the nested manifest and shared lockfile changed. Verify the target package manifest and lockfile importer, then reconcile both if the callback only handled the workspace root.

**Why:** The removal callback reported success, but the artifact manifest still declared the package and the shared lockfile retained its importer and transitive entries.

**How to apply:** After a package removal, check the owning artifact's `package.json` and `pnpm-lock.yaml`; only consider the dependency fully removed after a frozen-lockfile check succeeds.