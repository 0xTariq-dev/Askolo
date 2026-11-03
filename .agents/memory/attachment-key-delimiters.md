---
name: Attachment key delimiters
description: Preserves reliable parsing of private attachment object keys that contain base64url-encoded fields.
---

The `--` separator used in private attachment keys overlaps with the base64url alphabet and may appear inside generated IDs or encoded fields. Parse and validate candidate separator positions instead of splitting at the first two matches.

**Why:** random attachment IDs can contain consecutive hyphens, making first-match splitting intermittently return malformed metadata or hide a successfully uploaded file.

**How to apply:** when changing attachment-key encoding or parsing, preserve compatibility with existing keys and add deterministic tests for delimiter sequences within IDs and encoded values.
