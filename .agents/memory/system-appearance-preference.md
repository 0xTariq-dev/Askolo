---
name: System appearance preference
description: How the app represents the selected Light, Dark, or System choice while theme tokens require a concrete mode.
---

Keep the selected appearance preference (`light`, `dark`, or `system`) separate from the resolved `mode` (`light` or `dark`). Persist both so System can follow OS changes without weakening the concrete-mode contract used by token generation. The classic prepaint script must resolve System from `prefers-color-scheme` before styles render, and the React provider must listen for later OS changes.

**Why:** Theme palettes and surface styles are keyed by concrete light/dark modes, while the menu needs to preserve whether the user explicitly chose System.

**How to apply:** When adding or changing appearance choices, update local preference loading, prepaint behavior, and the provider's live system listener together.
