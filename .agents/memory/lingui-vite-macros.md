---
name: Lingui macros in Vite
description: Keep Lingui compile-time macro transformation separate from Vite catalog loading.
---

The Lingui Vite plugin compiles imported `.po` catalogs, but it does not transform `@lingui/core/macro` imports. Vite's React Babel setup must also run `@lingui/babel-plugin-lingui-macro`.

**Why:** A production build can succeed while a development browser still executes the macro's runtime guard and renders a blank page.

**How to apply:** For Vite + React, configure the Lingui Babel plugin alongside `@vitejs/plugin-react`, then verify both the dev browser and production build after adding or changing Lingui macros.