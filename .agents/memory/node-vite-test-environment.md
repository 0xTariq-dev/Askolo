---
name: Node tests and Vite env
description: Vite-specific module environment when frontend code is imported directly by Node's test runner.
---

Node's `tsx --test` runner does not populate Vite's `import.meta.env`. Frontend modules imported directly by tests must guard build-time environment reads or be exercised through a Vite-aware test runner.

**Why:** An unguarded `import.meta.env.BASE_URL` can throw during module loading, before any test setup or assertions run.

**How to apply:** Preserve Vite's configured value in the browser build, but provide an explicit safe default for direct Node imports; avoid relying on test-file globals to supply Vite's module environment.