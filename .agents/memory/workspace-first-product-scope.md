---
name: Space-centric product scope
description: Askolo product UI uses Spaces for family and professional contexts, with Home and explicit Agent scope.
---

Askolo is not limited to families or households. A Space can represent a family or a professional team; Spaces and invitations are cornerstones of the product. Use “Space” in product-facing language. The current technical schema and API use workspace-scoped identifiers; do not silently rename that contract.

The main view is named Home and presents a structured, factual day brief for the selected Space: the next scheduled item, focus-action progress, and flexible work. It is not an AI-generated narrative by default. Provide a separate All Spaces view and label each item with its Space. The Agent follows the selected Space or All Spaces scope the user is using and never silently widens it.

**Why:** the user explicitly asked to document the product scope and then selected these Home, All Spaces, and Agent-scope behaviors while defining Today/Plan.

**How to apply:** use Space and Home in user-facing navigation and copy, support family and professional team contexts, label cross-Space items, and keep Agent actions bounded to the user's explicit scope. Preserve existing technical workspace identifiers unless a separate contract change is approved.
