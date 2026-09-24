---
name: Planning scope boundaries
description: Durable ownership distinctions for roadmap planning; current issue status and dependencies live in Linear.
---

# Planning scope boundaries

Linear is the source of truth for current issue status, ownership, and dependencies. This note records stable scope distinctions, not a current task inventory.

**Why:** Past planning drafts conflated routing checks with feature implementation and combined capabilities with different security and execution boundaries.

**How to apply:** Before creating or updating work, search Linear for the canonical issue and read its current scope. Use these distinctions to avoid merging separate responsibilities or treating shared context as a blocker.

## Durable boundaries

- Forwarded-route smoke checks verify that paths reach a service; they do not prove the public WebSocket protocol or provider webhook ingestion is implemented.
- Go-owned voice provider transport and session lifecycle are separate from the browser-facing WebSocket contract for connection authorization, messages, reconnects, and draining. Keep the scopes distinct even when one depends on the other.
- Provider webhook verification, durable inboxing, replay protection, and safe event dispatch are separate from durable automation execution, leases, retries, and run history.
- Provider-backed AI execution, credit accounting, and global abuse controls are related but distinct. Credits govern budgets; abuse controls govern request and resource consumption.
- Executable schema migrations, preservation of historical migration evidence, and recovery objectives or restore rehearsals are separate capabilities.
- Security reviews and promotion safeguards verify evidence and record residual risk; they do not replace runtime authorization, abuse controls, or provider protections.
- The old public landing/app/API domain split is historical and archived. Environment separation means isolated stage projects, not that former product-surface split.