---
name: Generated Zod compatibility
description: Runtime compatibility rule for generated API schemas and the installed Zod version.
---

Generated API schemas can emit Zod 4-only helpers such as `zod.int()` and `zod.url()` even though the application runtime uses Zod 3, where these are expressed as `zod.number().int()` and `zod.string().url()`.

**Why:** The generated schema is loaded at API startup, so an incompatible generated method causes the entire API workflow to fail before any route can serve requests.

**How to apply:** Preserve the compatibility shim or update the generator/runtime pair together; do not upgrade Zod casually because generated schema and validation behavior affect the whole API surface.