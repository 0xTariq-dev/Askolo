---
name: Theme prepaint bootstrap
description: Vite's production module merging can invalidate render-blocking theme bootstrap assumptions.
---

Use a synchronous classic script in the document head to apply the saved or query-selected light/dark class and `color-scheme` before styles render. Keep full palette application in the regular bootstrap using shared design-system helpers before React mounts. Do not rely on a module script's `blocking="render"` attribute as the production guarantee.

**Why:** Vite's chunk merging moved the module bootstrap into the app bundle and removed its render-blocking attribute from built HTML. Applying the mode early prevents the largest light/dark surface mismatch while full token preferences load.

**How to apply:** After changing theme startup, inspect the built HTML and confirm the classic head script stays ahead of styles and app scripts. Verify the shared-token bootstrap applies the selected palette before React mounts.