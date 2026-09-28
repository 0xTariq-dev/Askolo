---
name: Replit artifact and canvas separation
description: How artifact files, Library registrations, and design-canvas frames differ in this workspace.
---

Artifact manifests, registry entries, managed workflows, and canvas frames are distinct. Removing root `.replit` workflow blocks alone did not clear default frames. Removing an unused artifact directory did unregister that artifact and its managed workflow here, but its default canvas frame persisted.

`removeWorkflow` refuses artifact-managed workflows. For an explicitly unwanted artifact, remove its source directory and any duplicate root workflow entry; keep packages still imported by other apps. Artifact frames cannot be deleted through canvas actions.

**Why:** Removing duplicate personal-assistant workflow blocks left the default frames. Removing the Askolo-UI API-server and mockup-sandbox artifacts then cleared their registry entries and managed workflows automatically, while a default API-server canvas frame remained and `/healthz` stayed healthy.

**How to apply:** Check cross-artifact imports before deleting a package; retain shared design-system packages when consumers depend on them. After deletion, verify artifact registry, workflow list, service health, and canvas state separately. Treat remaining default frames as persistent until a supported cleanup path is confirmed; do not disable live routes just to hide them.