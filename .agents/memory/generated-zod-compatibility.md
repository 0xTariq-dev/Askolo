---
name: Generated Zod compatibility
description: Runtime compatibility rule for generated API schemas and the installed Zod version.
---

Generated API schemas currently emit Zod 4-only helpers while the application runtime uses Zod 3; a compatibility shim bridges that mismatch.

**Why:** The generated schema is loaded at API startup, so an incompatible generated method causes the entire API workflow to fail before any route can serve requests.

**How to apply:** Keep the compatibility shim effective after regeneration, or upgrade the generator and runtime together. Verify generated-schema startup and validation after either change; do not upgrade Zod casually because generated schema behavior affects the whole API surface.