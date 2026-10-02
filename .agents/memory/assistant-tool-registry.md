---
name: Assistant tool registry contract
description: Compatibility boundary between model-facing tool arguments and persisted assistant confirmations.
---

Keep the model-facing planner envelope (`intent` plus `arguments`) separate from the persisted confirmation intent. Validate the selected tool and its strict argument object through the registry, then transform it into the established stored intent before requesting confirmation.

**Why:** The existing storage and client confirmation flow depend on the established intent shape. Reusing that format avoids a schema or UI migration while making the planner extensible without trusting model output as executable data.

**How to apply:** When explicitly adding a tool, register its schema, validation/preparation, authorization metadata, and confirmation handler. Keep tool selection allowlisted and preserve explicit confirmation for writes; do not expand the active tool set without user scope.