---
name: Structured data without reviews
description: How to handle marketing structured data when a product has no verified review dataset.
---

For a public product site without authentic review data, prefer `WebSite` and `Organization` structured data over a `SoftwareApplication` block that invites an `aggregateRating` warning.

**Why:** Optional Rich Results warnings are preferable to fabricated social proof, and adding a rating without a real review source creates a trust and policy risk.

**How to apply:** Add `SoftwareApplication` or `aggregateRating` only when the product has a genuine, verifiable review source that can support the markup. Validate the final published URL rather than relying only on local HTML.