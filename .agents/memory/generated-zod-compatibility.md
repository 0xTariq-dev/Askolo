---
name: Generated Zod compatibility
description: Runtime compatibility rule for generated API schemas and the installed Zod version.
---

Generated API schemas can emit Zod 4-only helpers while the application runtime uses Zod 3; code generation must normalize those helpers and the duplicate-export barrel before type-checking.

**Why:** The generated schema is loaded at API startup, so an incompatible generated method can fail the API before any route serves requests. Orval also exports colliding schema and type names from its barrel.

**How to apply:** Keep the post-generation normalization in the `api-spec` codegen path and run the library type-check after regeneration. Do not upgrade Zod casually because generated schema behavior affects the whole API surface.