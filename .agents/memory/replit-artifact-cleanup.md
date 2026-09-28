---
name: Replit artifact cleanup limits
description: Supported cleanup options for registered artifacts and generated service frames in Replit workspaces.
---

Replit does not support removing one registered artifact from the Library, nor removing service definitions from `artifact.toml` as a way to hide generated preview frames. Artifact frames also cannot be deleted through canvas actions; moving them away from the main work area is a safe visual workaround. Removing active services can break the app without removing the Library entry.

**Why:** The Library registration, canvas frames, and active service routing are separate platform concerns. Removing service configuration does not serve as an artifact cleanup operation.

**How to apply:** Before changing an artifact manifest for cleanup, distinguish registered Library artifacts from generated service frames. Keep active web/API services intact; use a separate project if an artifact must no longer appear in that project's Library.