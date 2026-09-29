---
name: Askolo app UI migration
description: Review guidance for consuming the shared Askolo design system in the personal assistant.
---

Treat shared component adoption as both a visual and behavior migration. Follow the current design-system package API when component props change so focus behavior is retained. Keep the signed-in shell viewport-bounded when it relies on an inner scroll pane; a minimum height alone does not constrain that flex layout.

**Why:** A design-system migration can pass type and build checks while route-level styling, focus semantics, reduced-motion support, or full-height scrolling still regresses.

**How to apply:** For UI-wide changes, audit every route for remaining legacy palette styles and demo content, preserve behavior-sensitive props, check the design-system API, and verify keyboard/accessibility and responsive behavior.