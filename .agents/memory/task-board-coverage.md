---
name: Task board coverage and gap map
description: Durable map of current production-readiness task coverage, duplicate task groups, and unowned work.
---

# Task Board Coverage and Gap Map

## Purpose

This is the unified reference for reconciling the production-readiness audit with the project task board. It records what existing work actually covers, what is only partial, which drafts overlap, and the missing work that must be assigned before implementation proceeds.

**Why:** The board contains several repeated or superseded drafts, and task titles alone do not prove that their acceptance criteria cover the full launch requirement.

**How to apply:** Before creating a task, search by capability and compare its full plan with this document. Prefer updating or consolidating an existing draft over creating another task with a similar title.

## Current board coverage

### Voice and realtime

- The merged AssemblyAI transcription foundation is only a provider/transcription base. It does not prove a Go-owned realtime session lifecycle.
- The draft unified Go AssemblyAI voice platform is the intended owner for server-managed voice sessions, short-lived/session-bound authorization, ordered transcript turns, credit reservation and settlement, reconnect behavior, maximum duration, privacy, deletion, abuse controls, and failure handling.
- The public Askolo `/ws` application protocol is not clearly covered by the voice plan. A route-forwarding smoke test only proves that the prefix reaches a service; it does not prove that assistant/app streaming, voice sessions, message schemas, reconnects, backpressure, origin checks, or graceful draining work.
- Do not create another standalone voice-agent task. Extend the unified voice platform scope if the public `/ws` contract belongs in the same lifecycle; split it only if the board owner deliberately keeps provider voice transport and public application streaming separate.

### Durable automation

- The durable automation execution drafts cover persistent definitions and runs, leases, idempotency, heartbeats, one transient retry, abandoned-run recovery, bounded execution, credit reconciliation, terminal states, cancellation, and execution history.
- They support manual, scheduled, and app-event runs.
- They explicitly exclude inbound email and webhook ingestion. They therefore cannot be used as proof that GitHub, Google, or AssemblyAI webhook receivers exist.
- The two durable-automation drafts are duplicate scopes and should be consolidated before execution.

### AI credits and policy

- The merged credit ledger provides the accounting foundation.
- The AI credit policy drafts cover configurable operation weights, overrun margin, grants, rollover and expiry policy, per-user adjustments, estimates, reservations, settlement, refunds, usage inspection, versioning, and auditability.
- Credit policy work does not replace provider-backed AI execution, route abuse controls, or webhook cost controls.
- The two AI-credit-policy drafts are overlapping scopes and should be consolidated.

### Authentication, MFA, and monitoring

- Authentication, authorization handover, MFA enrollment/challenge, recovery throttling, and privacy-safe operational events are already represented by merged work and follow-up monitoring drafts.
- MFA recovery abuse controls are not a general rate-limit framework for every public route.
- MFA alert and readiness checks do not prove provider webhook security, AI request quotas, voice maximum duration enforcement, or WebSocket connection controls.

### Recovery and database history

- The merged database-history retirement gate defines conditions for retiring historical Drizzle tooling.
- The production-recovery draft requires a disposable restore from a current production backup, Go service startup against that restore, and critical authentication/session/product-path checks.
- The immutable-archive draft requires the historical SQL, journals, snapshots, and schema modules to remain verifiable outside the retired package.
- These recovery drafts do not create an executable Go migration runner or prove zero committed-transaction loss and one-hour restoration. A backup restore rehearsal is evidence of recoverability, not a zero-RPO guarantee.

### Deployment environments and publication

- The deployment-environment draft describes separate development, staging, and production Repls.
- The Google staging publication draft and production-labelled-hostname guard are the relevant follow-ups for environment isolation and canonical callback correctness.
- The merged Google callback safety work improves source behavior but does not prove that the current published environment is correctly isolated.
- The published-route smoke draft checks `/api`, `/healthz`, `/readyz`, `/ws`, and `/webhooks` after forwarding. It is a routing contract check, not an implementation check for WebSockets or webhooks.
- The development-domain plan now owns the verified artifact inventory: the registered personal-assistant web artifact is authoritative, while its web workflow is a second preview entity for the same frontend rather than a second website. The retired TypeScript API directory and mockup placeholder are unrelated residue.
- Canonical plans for later product work must use the current Go backend and generated contracts; deleted `artifacts/api-server` and legacy database-package paths are not implementation sources.

## Duplicate reconciliation status

The duplicate pass is complete for the current non-archived board scan. Each exact-title group now has one canonical draft and its older entries are explicitly marked as superseded. No live exact-title or high-similarity duplicate pair remains, and no live task depends on a superseded entry.

Canonical groups now cover:

- Durable automation execution.
- AI credit policy controls.
- Unified Go AssemblyAI voice platform, including the guarded runtime aliases.
- TOTP enrollment and challenge.
- Keyboard accessibility baseline.
- Theme presets.
- Context menus and commands.
- MFA recovery and trusted devices.
- PWA and mobile direction.
- Multiple Google accounts.
- Unified responsibility model and responsibility migration.
- Workspace sharing/filtering and workspaces/labels.
- Conflict-aware planning.
- Temporary interactive tutorial.
- Agent approvals, memory, and audit.
- Private Mixpanel analytics.
- Account-aware Google sync.
- Secure persona survey.
- Arabic RTL foundation and Arabic UI/metadata rollout.
- Automation triggers and templates.
- Private Meilisearch evaluation.
- Development web/API domain separation and preview-artifact reconciliation.
- Landing SEO/GEO.

An older task describing production-complete AI and voice responses is archived. It is the closest match for the canned AI-response gap, but it is not current coverage unless its scope is deliberately restored or replaced. Superseded duplicate entries are board history, not additional executable scope.

## Missing work with implementation-ready scope

The following capabilities are not clearly owned by a current non-archived task.

### 1. Provider webhook ingestion and configurable actions

**What and why:** Implement one durable, provider-aware webhook ingestion boundary so external events can safely trigger user-configured actions without duplicate side effects or unauthenticated execution.

**Required scope:**

- Receive and verify GitHub push, installation, issue, and pull-request events.
- Receive and verify Google Gmail, Calendar, and OAuth lifecycle/revocation events using the provider’s documented notification and authorization model.
- Receive and verify AssemblyAI completion, failure, and callback events where the selected integration uses callbacks.
- Validate provider signatures, timestamps, event identifiers, content type, and payload size before persistence.
- Persist an inbox record before acknowledging an accepted event; enforce idempotency on provider delivery identifiers and a stable payload fallback.
- Acknowledge quickly, process asynchronously, retry transient failures with bounded backoff, and retain dead-lettered events with operator-visible reasons.
- Resolve the target user and configured action only through server-owned authorization and tenant boundaries. Never trust user identity or action scope from an unverified payload.
- Make actions configurable per user, auditable, cancellable where safe, and safe to replay without duplicating external side effects.
- Add tests for signature failures, replay, duplicate delivery, malformed payloads, provider retries, unknown installations/accounts, revoked access, authorization boundaries, and downstream failure.

**Done looks like:** Every required provider has a verified receiver, accepted events survive restart, duplicate deliveries do not duplicate actions, and a user can configure which safe actions an event may trigger.

**Critical constraint:** This is not covered by durable automation alone because that work explicitly excludes inbound webhooks. The webhook boundary may enqueue durable automation runs, but ingestion security and idempotency remain part of this scope.

### 2. Complete the public Askolo WebSocket protocol

**What and why:** Replace the placeholder public `/ws` behavior with the server-owned application protocol required for assistant/app streaming and voice sessions.

**Required scope:**

- Define authenticated connection establishment, session binding, message types, correlation identifiers, terminal events, and protocol versioning.
- Support assistant streaming and voice-session events without exposing provider credentials or allowing clients to choose another user’s session.
- Enforce origin policy, connection quotas, maximum session duration, message-size limits, idle timeouts, backpressure, and per-user cost controls.
- Handle reconnect and resume semantics explicitly; prevent duplicate transcript turns and duplicate tool/action execution.
- Close connections predictably during deploys and drain active sessions within the configured limit.
- Emit privacy-safe structured lifecycle logs and metrics without recording raw audio, credentials, or transcript content unnecessarily.
- Add protocol, authorization, abuse, restart, reconnect, timeout, and graceful-shutdown tests.

**Board treatment:** Prefer extending the unified voice platform draft with the public protocol requirements rather than creating a second voice task. Keep this as a separate task only if the board owner wants an independent public-streaming milestone.

### 3. Replace canned AI responses with provider-backed execution

**What and why:** Make every advertised assistant, coaching, planning, extraction, and other AI endpoint execute through the approved provider boundary instead of returning canned or placeholder output.

**Required scope:**

- Inventory every advertised AI endpoint and classify it as implemented, provider-backed, degraded, or placeholder.
- Route provider work through fresh atomic credit reservations and claims; never execute from a reused, active, expired, or ambiguous reservation.
- Enforce request validation, bounded input/output size, timeouts, cancellation, provider error mapping, and safe retry rules.
- Return truthful status and failure information; do not present canned output as generated work.
- Preserve privacy boundaries for user content, uploaded files, transcripts, and connected-service data.
- Add abuse controls and cost ceilings for each provider-backed operation.
- Add integration and failure tests using provider stubs, including timeout, malformed provider output, partial tool execution, duplicate retry, insufficient credits, and cancellation.

**Board treatment:** The archived production-complete AI/voice task is the closest prior owner. Restore or replace that scope deliberately; do not create a narrowly named duplicate that covers only one endpoint.

### 4. Establish executable Go migrations and schema-drift checks

**What and why:** Make schema changes reproducible and reviewable without depending on the retired Drizzle package.

**Required scope:**

- Define the versioned migration format, ordering, metadata, checksum policy, and transaction behavior.
- Provide an executable migration command for development, staging, and production release workflows.
- Make startup behavior explicit: migrations must not silently run destructive changes or hide failures.
- Add forward-application and safe failure tests against a clean database and a representative existing database.
- Add schema-drift validation so generated/declared schema and applied migration history cannot silently diverge.
- Document rollback/forward-fix expectations and how a restore is brought to the required schema.
- Preserve historical migration evidence in the protected archive required by the database-history retirement work.

**Critical constraint:** The recovery/archive drafts preserve evidence and test restores; they do not substitute for an executable migration owner.

### 5. Prove recovery objectives, including the stated zero-RPO requirement

**What and why:** Turn the stated recovery target—no loss of committed transactions and restoration within one hour—into a tested, operationally achievable contract.

**Required scope:**

- Define what “committed transaction” means for the application and database boundary.
- Choose and document the replication, WAL, backup, or managed recovery configuration required to meet the RPO target; do not claim zero RPO from scheduled backups alone.
- Measure backup freshness, replication lag, restore duration, schema compatibility, and application cutover time.
- Run a repeatable disposable restore rehearsal using production-like data handling without exposing credentials or personal data.
- Verify authentication, sessions, credits, connected-service state, automation state, webhook inboxes, and audit history after restore.
- Record evidence for the one-hour RTO and explicitly document any condition under which the objective is not met.
- Define operator steps for failover, restore, validation, traffic cutover, and post-restore reconciliation.

**Board treatment:** This should extend the recovery work rather than duplicate the database-history archive. Archive preservation, migration execution, and recovery-objective proof are related but distinct acceptance criteria.

### 6. Add global public-endpoint abuse and cost controls

**What and why:** Provide consistent, route-specific defenses across all public endpoints instead of limiting protection to MFA recovery.

**Required scope:**

- Define limits and quotas for authentication, password recovery, OAuth callbacks, AI requests, file uploads, voice sessions, WebSocket connections/messages, and webhooks.
- Use shared server-side state where multiple backend processes can otherwise bypass limits.
- Key controls on privacy-safe hashed client/user/account identifiers and avoid storing raw sensitive identifiers in rate-limit records or logs.
- Apply maximum request body sizes, concurrency limits, timeouts, provider spend ceilings, maximum voice duration, and queue bounds.
- Return stable retry information and safe error messages without leaking account existence or provider details.
- Add abuse telemetry, aggregate alerts, and operator controls for temporary blocks and recovery.
- Test distributed enforcement, boundary conditions, clock/expiry behavior, restart behavior, and provider failure.

**Board treatment:** MFA recovery throttling and AI credit policy are inputs to this work, not substitutes for a complete public-endpoint control plane.

### 7. Complete the public-route security review

**What and why:** Close the launch requirement for route-specific security beyond authentication and basic health checks.

**Required scope:**

- Review CSRF and same-origin behavior for state-changing browser requests.
- Review CORS and allowed-host behavior for development, staging, and production.
- Enforce WebSocket origin and session authorization checks.
- Threat-model provider webhook spoofing, replay, SSRF through callback content, oversized payloads, and malicious downstream actions.
- Review transcript/audio privacy, retention, deletion, logging, and connected-service scope boundaries.
- Validate error redaction, secret handling, audit events, and operator diagnostics.
- Add regression tests for every identified high-risk boundary and record accepted residual risk.

**Board treatment:** Combine this with global abuse controls only if the task remains readable and has separate acceptance criteria. Otherwise keep security review as a dependent launch gate.

## Not missing, but still not complete

- The deployment-isolation, staging-publication, and production-hostname drafts should be consolidated or explicitly ordered; they are related environment work, not duplicates of backend feature tasks.
- The published-route smoke draft should remain a release gate, but its success must not be interpreted as evidence that the `/ws` or `/webhooks` feature is implemented.
- Google callback source changes are merged, but environment-level validation is still required.

## Reconciliation rule

For the coming task cleanup:

1. Preserve one canonical task for each capability.
2. Merge duplicate drafts by updating the most complete plan and archiving only the superseded copy after the board owner confirms.
3. Extend the unified voice task with public `/ws` requirements if they share the same lifecycle.
4. Keep webhook ingestion separate from durable automation because webhook verification and replay safety are provider-boundary responsibilities.
5. Keep migration execution separate from recovery rehearsal, while making recovery depend on the migration contract.
6. Keep global abuse controls separate from the credit ledger; credits govern budget, while abuse controls govern request and resource consumption.
7. Use the archived AI-completion scope as the starting point for restoration or replacement instead of creating another endpoint-specific placeholder task.
