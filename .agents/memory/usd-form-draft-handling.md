---
name: USD form draft handling
description: Preserve editable money drafts while keeping the API’s integer-microunit contract.
---

Keep money inputs as editable decimal text while users type. Validate the complete value, including grouping and supported precision, then convert it to safe integer USD microunits for submission. Never coerce an incomplete or invalid draft to zero for a write.

**Why:** the API needs exact fixed-point amounts, while reformatting on each keystroke can lose partial values and make fields difficult to edit.

**How to apply:** credit policy, rate, and adjustment forms, and future admin money inputs.