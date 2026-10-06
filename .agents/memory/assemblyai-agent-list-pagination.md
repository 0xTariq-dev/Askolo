---
name: AssemblyAI agent list pagination
description: The live Voice Agent list response can differ from the documented array shape.
---

The live Voice Agent list endpoint returned a pagination envelope containing `agents`, `has_more`, and `response_metadata`, while the current API docs described a bare array. A provisioner must support both forms and must not create agents if the response indicates additional pages.

**Why:** An incomplete list could hide an existing same-name agent and cause duplicate provider resources.

**How to apply:** Parse the response structure without logging provider bodies or identifiers. Follow pagination when supported; otherwise fail closed when `has_more` is true.
