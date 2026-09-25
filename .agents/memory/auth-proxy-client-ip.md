---
name: Authentication proxy client IP
description: Runtime observation of forwarded client-IP behavior through the Replit development artifact proxy.
---

The Replit development artifact proxy removed a caller-supplied `X-Forwarded-For` marker before the Go API received the request. The backend peer matched the current private-proxy trust rule, and the forwarded header contained three hops. The probe recorded only booleans and a hop count; it did not expose or retain addresses. This observation does not establish production Autoscale's exact peer or header behavior.

**Why:** Forwarded-IP trust is a security boundary; assuming development and production ingress are identical could create bypasses or apply limits to a shared proxy.

**How to apply:** Treat this as evidence only for the development artifact route. Before changing trust ranges or claiming production Autoscale behavior, verify the production ingress contract without exposing or retaining raw client IPs.