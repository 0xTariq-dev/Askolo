---
name: Askolo app UI migration
description: Review guidance for consuming the shared Askolo design system in the personal assistant.
---

Treat shared component adoption as both a visual and behavior migration. Follow the current design-system package API when component props change so focus behavior is retained. Keep the signed-in shell viewport-bounded when it relies on an inner scroll pane; a minimum height alone does not constrain that flex layout.

**Why:** A design-system migration can pass type and build checks while route-level styling, focus semantics, reduced-motion support, or full-height scrolling still regresses.

**How to apply:** For UI-wide changes, audit every route for remaining legacy palette styles and demo content, preserve behavior-sensitive props, check the design-system API, and verify keyboard/accessibility and responsive behavior.

<<<<<<< HEAD
Keep lazy signed-in route Suspense boundaries inside the persistent app shell, so first-time page chunk loads replace only the content pane rather than the sidebar and whole screen.

**Why:** A full-screen fallback on the first visit to an uncached lazy route can look like a browser reload even when Wouter performs client-side navigation.

**How to apply:** Retain the outer fallback for initial shell startup, but place a smaller loading state around the authenticated page switch inside `SidebarAppLayout`.

=======
>>>>>>> c7ea45a5fdbf4bb422d76b6a13e0d4dca631b40a
For ReUI example installs, dry-run the registry item and back up any app-owned files it will overwrite. If the `@reui` alias redirects to a canonical public registry JSON URL that the shadcn CLI rejects, install that exact JSON from a temporary local file. Preserve the user-selected variant instead of switching variants only to match the app's current primitives.

**Why:** Registry redirects can fail in the CLI even when the free example is publicly downloadable, and a variant substitution changes the requested example.

**How to apply:** Verify the resolved item name and variant before installation, inspect every overwrite, retain a recoverable temporary backup, and keep a reference-only example unconnected until its app-specific use is requested.
