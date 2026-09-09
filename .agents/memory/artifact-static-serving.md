---
name: Managed static artifact serving
description: The web artifact schema currently manages production static serving and rejects replacing it with a custom serve command.
---

The web artifact should keep its validated managed `static` production server unless Replit exposes a supported cache-header or compression configuration. Attempting to replace `serve = "static"` with a custom Node command fails artifact schema validation.

**Why:** Asset-level cache headers and content encoding are important for production performance, but an unvalidated custom server would risk breaking artifact routing and the separate `/api` service.

**How to apply:** Keep client-side route/query caching and chunk optimization in the app. Revisit immutable asset headers and Brotli/gzip only through a supported artifact/deployment configuration.