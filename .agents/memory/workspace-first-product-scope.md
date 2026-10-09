---
name: Space-centric product scope
description: Askolo product UI uses Spaces for family and professional contexts, with Home and explicit Agent scope.
---

Askolo is not limited to families or households. A Space can represent a family or a professional team; Spaces and invitations are cornerstones of the product. Use “Space” in product-facing language. The current technical schema and API use workspace-scoped identifiers; do not silently rename that contract.

The main view is named Home and presents a concise, factual roll-up for the selected Space rather than plotting every task, event, email, and Plan entry. Illustrative summaries include urgent-task counts, email counts with the number needing action, and meeting counts. Focus actions come from items already in the day's Plan. It is not an AI-generated narrative by default. Provide a separate All Spaces view and label each item with its Space. The Agent follows the selected Space or All Spaces scope the user is using and never silently widens it. Home layout references are examples, not fixed templates: each user can customize their layout independently for each Space. Reuse existing design-system support for card resizing and drag-and-drop ordering; predefined widgets are future scope.

**Why:** the user explicitly defined the Home, All Spaces, Agent-scope, and per-user/per-Space layout behaviors. They supplied a sample layout but said it should not be followed strictly, and deferred predefined widgets.

**How to apply:** use Space and Home in user-facing navigation and copy, support family and professional team contexts, label cross-Space items, keep Agent actions bounded to the user's explicit scope, and scope layout preferences to the user-Space pair. Preserve existing technical workspace identifiers unless a separate contract change is approved.
