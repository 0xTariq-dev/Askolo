---
name: Replit artifact and canvas separation
description: How artifact files, Library registrations, and design-canvas frames differ in this workspace.
---

Artifact files, Library registrations, and design-canvas frames are separate things. The original Askolo-UI submodule has `artifact.toml` files for its API server and mockup sandbox, but those files do not make corresponding frames appear on the design canvas. Do not infer a frame's origin from the existence of an artifact manifest.

Replit docs currently say an individual artifact cannot be removed from the Library and removing service entries from `artifact.toml` is not a supported way to hide preview frames. Canvas guidance also says artifact frames cannot be deleted through canvas actions. Removing active service configuration may break routing without solving a separate canvas-frame issue.

**Why:** The Askolo-UI submodule provides a concrete counterexample to treating manifests, Library entries, and canvas frames as one registry. The stale-frame issue needs investigation through canvas state/provenance rather than assumptions about service definitions.

**How to apply:** For stale canvas frames, inspect and identify the specific frame first. Do not edit active `artifact.toml` service definitions unless the user wants a routing change. Treat Library cleanup as a separate platform limitation; a clean Library requires a new project and migration.