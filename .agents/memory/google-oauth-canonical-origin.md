---
name: Google OAuth canonical origin
description: Native Google OAuth callback URIs must be derived from the configured canonical origin in staged and production environments.
---

**Rule:** when `ASKOLO_CANONICAL_ORIGIN` is configured, use it for both the authorization redirect URI and the token exchange rather than reconstructing the origin from proxy headers.

**Why:** reverse proxies and artifact routers can expose a host different from the public OAuth origin; using the forwarded host causes Google redirect-URI mismatches and failed code exchanges.

**How to apply:** keep configured canonical hosts in the OAuth allowlist, preserve localhost/request-host derivation only for development, and verify the emitted callback URI with a staging client before promotion.