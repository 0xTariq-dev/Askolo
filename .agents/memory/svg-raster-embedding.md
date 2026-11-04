---
name: SVG raster embedding
description: Browser behavior for SVGs that include raster logo assets.
---

Use direct PNG assets for application UI logos. If an SVG must render as a standalone image, embed its raster source as a data URI rather than referencing a sibling file with a relative `href`.

**Why:** Browsers may suppress external image references when an SVG is loaded through an `<img>` or as a favicon, leaving the SVG document valid but visually blank.

**How to apply:** Verify the SVG itself in a browser image context, not only that its URL returns HTTP 200 or that the source contains an `<image>` element.
