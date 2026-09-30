---
name: Resend MCP output handling
description: Safely inspect Resend MCP results when tools return human-readable text instead of structured JSON.
---

Resend list/get integrations can return text-formatted content blocks rather than structured JSON, even though their tools are documented as listing fields. Filter within the tool session for only the explicitly requested recipient and message purpose; keep recipient addresses, message bodies, and verification codes in memory only, and print only safe status/timestamp summaries.

**Why:** Resend MCP output may include personal email addresses and full verification-message contents, while its response shape may not match a JSON parser.

**How to apply:** When tracing a transactional message, parse only the expected metadata from the formatted result, match the user-authorized recipient exactly, and avoid logging raw provider output.