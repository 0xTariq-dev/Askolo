---
name: Fresh Agent conversations
description: How a new Agent visit starts a separate conversation without erasing prior history.
---

When Agent opens as a fresh chat, do not load the saved current conversation or pass its ID on the first request. The backend creates a new conversation when the ID is omitted; retain the returned ID for follow-up messages in that visit.

**Why:** The user requested a fresh chat, but did not approve deleting conversation history.

**How to apply:** Keep fresh-chat initialization scoped to the Agent route. Do not change the current-conversation behavior for the compact assistant or other conversation entry points.
