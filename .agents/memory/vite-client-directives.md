---
name: Vite client directive warnings
description: Avoid Next.js client-component markers in this Vite-only frontend.
---

This frontend is a client-only Vite app. Do not add top-level `"use client"` directives to its UI modules; they have no client-boundary meaning here and Rollup can emit source-map location warnings for those modules.

**Why:** Removing the directives from the affected UI components eliminated the build warnings without changing runtime behavior; tests, typecheck, and production build remained successful.

**How to apply:** Omit `"use client"` in new or generated UI files. If copied client-only components include it, remove the directive in source rather than disabling source maps, then rerun the production Vite build.
