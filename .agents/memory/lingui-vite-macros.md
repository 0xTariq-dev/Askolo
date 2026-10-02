---
name: Lingui localization in Vite
description: Keep Lingui macro transforms and Vite-generated catalog loaders working in the browser.
---

The Lingui Vite plugin compiles imported `.po` catalogs, but it does not transform `@lingui/core/macro` imports. Vite's React Babel setup must also run `@lingui/babel-plugin-lingui-macro`.

Vite statically replaces `import.meta.glob(...)` with a literal loader map; it does not make `import.meta.glob` a runtime browser function. A `typeof import.meta.glob` feature check can therefore disable the generated map in the browser even though the transform registered the catalog paths.

**Why:** A successful build or Vite SSR check can hide browser-only failures: the macro may remain untransformed, or a runtime guard may discard the catalog map.

**How to apply:** For Vite + React, configure the Lingui Babel plugin alongside `@vitejs/plugin-react`; avoid runtime feature checks on `import.meta.glob` when guarding non-Vite tests, and verify catalog loading in an actual browser as well as the production build.