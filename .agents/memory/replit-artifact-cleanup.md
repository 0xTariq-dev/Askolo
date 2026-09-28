---
name: Replit artifact and canvas separation
description: How artifact files, Library registrations, and design-canvas frames differ in this workspace.
---

Artifact manifests, Library registrations, workflows, and design-canvas frames are distinct. Replit can derive managed workflows from artifact manifests; removing root `.replit` workflow blocks alone did not remove the corresponding `default-*` frames in this workspace.

Do not remove active service definitions from `artifact.toml` to clean canvas frames; that can break routing. After workflow-config changes, verify the managed workflows and routes, then check canvas state. If frames persist, do not disable a live service just to remove them without confirming the impact. Artifact frames cannot be deleted through canvas actions.

**Why:** The board showed a canonical app frame plus two `default-*` web/API frames. Removing the explicit root workflow blocks and restarting left all three frames in place, while artifact-owned workflows remained active and `/healthz` stayed healthy.

**How to apply:** Match selected frame IDs to workflow names, compare `.replit` entries with artifact manifests, and remove only proven duplicate configuration. If the board still shows the frames, treat them as managed/persistent previews and preserve live routes until a supported cleanup path is confirmed.