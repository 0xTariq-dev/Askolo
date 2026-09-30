---
name: Workspace-scoped authorization
description: Keeping collection-level actions distinct from authorization of a specific resource.
---

Authorization scope has three states: both resource type and ID empty means workspace/action scope; both present means a concrete resource scope; exactly one present remains an incomplete resource scope and must be denied. Collection operations without a specific ID should normalize to workspace/action scope at the handler boundary. Do not relax the evaluator to accept a type-only resource scope.

**Why:** Generic actions such as AI execution need workspace capability checks but do not target a particular object. Treating a type-only selector as a valid object scope can bypass future ownership checks.

**How to apply:** For collection/list/create operations with no concrete resource ID, authorize the workspace action with both resource fields empty. Keep both fields populated for resource-specific operations so ownership and workspace checks still run.