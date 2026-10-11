---
name: Askolo app UI migration
description: Review guidance for consuming the shared Askolo design system in the personal assistant.
---

Treat shared component adoption as both a visual and behavior migration. Follow the current design-system package API when component props change so focus behavior is retained. Keep the signed-in shell viewport-bounded when it relies on an inner scroll pane; a minimum height alone does not constrain that flex layout.

**Why:** A design-system migration can pass type and build checks while route-level styling, focus semantics, reduced-motion support, or full-height scrolling still regresses.

**How to apply:** For UI-wide changes, audit every route for remaining legacy palette styles and demo content, preserve behavior-sensitive props, check the design-system API, and verify keyboard/accessibility and responsive behavior.

Sidebar tooltip triggers use Radix primitives and must remain beneath a `TooltipProvider`; keep the provider around the shell-level sidebar and choose outward tooltip placement for the active text direction.

**Why:** Registry replacements can preserve the tooltip trigger components while dropping their provider, causing a runtime error on pages that render the sidebar.

**How to apply:** After replacing sidebar primitives, check provider ancestry for every `Tooltip` and verify left/right placement in LTR and RTL layouts.

Theme preset IDs are persisted preferences, including per-space overrides. When renaming one, normalize legacy IDs in both the prepaint bootstrap and the preference loader.

**Why:** Updating the visible preset list alone leaves saved IDs invalid during first paint or after app startup.

**How to apply:** Keep the compatibility mapping until legacy stored values have aged out, and test both account-level and space-level saved selections.

Keep lazy signed-in route Suspense boundaries inside the persistent app shell, so first-time page chunk loads replace only the content pane rather than the sidebar and whole screen.

**Why:** A full-screen fallback on the first visit to an uncached lazy route can look like a browser reload even when Wouter performs client-side navigation.

**How to apply:** Retain the outer fallback for initial shell startup, but place a smaller loading state around the authenticated page switch inside `SidebarAppLayout`.

The signed-in primary navigation should list Home, Agent, Agenda, Calendar, and Email in that order. Goals and Notes belong in More; fold task-like views such as habits, chores, and action items into Agenda.

**Why:** The user considers Email a distinct primary view, not an Agenda section, and wants task-like views consolidated rather than restored as separate primary links.

**How to apply:** Update every signed-in shell's nav and command menu consistently, keep Goals and Notes discoverable in More, and send assistant drafts to the full Agent page rather than a floating panel.

For ReUI example installs, dry-run the registry item and back up any app-owned files it will overwrite. If the `@reui` alias redirects to a canonical public registry JSON URL that the shadcn CLI rejects, install that exact JSON from a temporary local file. Preserve the user-selected variant instead of switching variants only to match the app's current primitives.

**Why:** Registry redirects can fail in the CLI even when the free example is publicly downloadable, and a variant substitution changes the requested example.

**How to apply:** Verify the resolved item name and variant before installation, inspect every overwrite, retain a recoverable temporary backup, and keep a reference-only example unconnected until its app-specific use is requested.

ReUI MCP search is for component examples and icons, not opaque shadcn preset payloads. Resolve a preset code with the shadcn CLI; if an apply fails because an app-owned component is absent from the registry, preserve that component and retry without overwriting it.

**Why:** ReUI component search does not decode preset codes, and a registry apply can fail on a custom component even when the preset itself is valid.

**How to apply:** Keep the preset code and decoded appearance settings in the project's token source, back up any app-owned registry targets before CLI apply, and restore them immediately afterward.
