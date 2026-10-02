---
name: Replit artifact and canvas separation
description: How artifact files, Library registrations, and design-canvas frames differ in this workspace.
---

Artifact manifests, registry entries, managed workflows, and canvas frames are distinct. Removing root `.replit` workflow blocks alone did not clear default frames. Removing an unused artifact directory did unregister that artifact and its managed workflow here, but its default canvas frame persisted.

`removeWorkflow` refuses artifact-managed workflows. For an unwanted artifact whose package is disposable, remove its source directory. If the package must remain, remove only `.replit-artifact/artifact.toml`; this unregisters the artifact and clears its managed workflow while retaining package files. Keep packages still imported by other apps. Artifact frames cannot be deleted through canvas actions.

**Why:** Removing duplicate personal-assistant workflow blocks left the default frames. Removing artifact directories or only their manifests cleared registry entries and managed workflows automatically; manifest-only removal preserved the design-system package byte-for-byte. A default API-server canvas frame remained and `/healthz` stayed healthy.

**How to apply:** Check cross-artifact imports before deleting a package. After manifest-only removal, verify the package file tree is unchanged. After either cleanup path, verify artifact registry, workflow list, and service health separately. Treat remaining default frames as persistent until a supported cleanup path is confirmed; do not disable live routes just to hide them.