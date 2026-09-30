---
name: API client declaration freshness
description: Diagnosing missing generated API exports in consumers of TypeScript composite projects.
---

When an app reports that an API value or type is missing, first compare the canonical generated source with the declaration output selected through TypeScript project references. A stale declaration build can disagree with current source even when package exports and runtime bundling point at the right files. If the canonical source is current, rebuild the referenced client project before running OpenAPI codegen or editing imports.

**Why:** Regenerating a correct contract does not refresh stale composite-project declarations; hand-editing callers can then hide a build-state problem and create API drift.

**How to apply:** For missing-export diagnostics, inspect source and the consumer's resolved declaration target. Refresh the client project's declarations when only build output is stale; regenerate from the OpenAPI source only when that source is actually outdated.