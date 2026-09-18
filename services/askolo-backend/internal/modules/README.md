# Backend module boundaries

The Go service is designed to become the primary backend without moving
transport or domain code during the migration.

- `internal/transport/*` owns protocol concerns only: HTTP status and headers,
  WebSocket lifecycle, webhook verification/acknowledgement, payload limits,
  and conversion into application commands.
- `internal/app` is the composition root. It wires configuration, middleware,
  transports, application modules, and infrastructure adapters.
- `internal/platform/*` contains cross-cutting infrastructure such as request
  IDs, safe errors, logging, authentication, rate limits, and timeouts. It
  must not contain product-specific business rules.
- Future `internal/modules/<name>` packages own one product capability and
  expose application-facing interfaces. They must not import transport
  packages or call provider SDKs directly.
- Future `internal/adapters/<name>` packages own databases and external
  providers. Provider credentials and raw payloads stay inside adapters.
- The TypeScript backend may call `/internal/*` during the companion phase.
  Public `/api/*`, `/ws`, and `/webhooks/*` become first-class ingress paths
  only after authentication, ownership, and rollout are explicitly migrated.