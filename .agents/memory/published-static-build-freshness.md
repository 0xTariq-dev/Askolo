---
name: Published static build freshness
description: How to distinguish stale published assets from current source behavior during audits
---

Before diagnosing a production SEO or performance report, compare the live HTML's hashed asset names and modification date with the current build output. A published static deployment can lag the branch and return the SPA shell for asset paths that belong only to a newer local build.

**Why:** A stale deployment can make compression, CSS size, redirects, and page-structure reports describe an older release rather than the code being reviewed.

**How to apply:** Treat source fixes and republishing as separate steps; verify the live asset manifest after publishing before interpreting a follow-up crawl.