# Backend module boundaries

The Go service is Askolo's current backend and sole server-side owner. Keep
transport, product rules, and infrastructure adapters separate as capabilities
evolve.

- `internal/transport/*` owns protocol concerns only: HTTP status and headers,
  WebSocket lifecycle, webhook verification/acknowledgement, payload limits,
  and conversion into application commands.
- `internal/app` is the composition root. It wires configuration, middleware,
  transports, application modules, and infrastructure adapters.
- `internal/platform/*` contains cross-cutting infrastructure such as request
  IDs, safe errors, logging, authentication, rate limits, and timeouts. It
  must not contain product-specific business rules.
- `internal/modules/<name>` packages own one product capability and
  expose application-facing interfaces. They must not import transport
  packages or call provider SDKs directly.
- `internal/adapters/<name>` packages own databases and external
  providers. Provider credentials and raw payloads stay inside adapters.
- Public ingress is handled by Go transport packages. Keep `/internal/*`
  restricted to authenticated service-to-service calls; browsers must not call
  internal routes. Enable webhook paths only after signature verification,
  authorization, and application handling are complete.