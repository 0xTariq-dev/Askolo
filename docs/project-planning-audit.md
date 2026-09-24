# Project Planning Audit: Replit Task Board and Askolo Linear

**Reviewed:** 2026-09-24  
**Scope:** Review of the Replit task board, local planning files, and the Askolo-dev Linear project; approved Linear updates and reconciliation decisions are recorded below.

## Executive summary

The Linear dependency graph represents most of the established feature chains. The main planning friction is concentrated in:

1. A duplicate public WebSocket issue (TAR-37 and TAR-44).
2. Environment and publication gates whose blocker relationships may not reflect the intended release policy.
3. Divergence between historical Replit task dependencies and the current Linear dependency graph.
4. Scope boundaries among voice, WebSocket, AI, webhooks, automation, credits, abuse controls, security review, and approvals that should be made more explicit.
5. Superseded local proposals that still appear executable.
6. A few title/description and labeling inconsistencies.

Linear remains the active planning source of truth. The Replit board and local plans are useful for provenance and acceptance-criteria recovery, but should not independently define a second active dependency graph.

## Confirmed duplicate

### TAR-37 and TAR-44: public WebSocket protocol

Both issues represent the same public WebSocket protocol scope and cite the same migrated Replit task (#237). TAR-37 was created first; TAR-44 is the mapping already recorded in `linear-migration-tracking.md`, and current dependencies and related links are attached to TAR-44.

**Resolution selected:** Keep TAR-44 canonical and mark TAR-37 as a duplicate of TAR-44. Preserve TAR-44's existing relationship to TAR-5, TAR-40, and TAR-41.

**Resolution status:** Complete. Linear marks TAR-37 as `Duplicate`, and its `duplicateOf` relation points to TAR-44. Read-back verification confirmed the relation. TAR-44 remains canonical with its existing relationships intact.

## Environment and publication relationships

These issues have separate responsibilities; the public landing artifact issue is now archived and is historical only:

- **P28 Separate the public landing artifact:** archived; its old public/app/API product-surface split is not the current stage/project plan.
- TAR-33: development web/API domain separation.
- TAR-34: prevent production-labelled deployments from using staging hostnames.
- TAR-42: environment promotion safeguards.
- TAR-43: published API route checks after forwarding.
- TAR-41: public-route security review.

TAR-33, TAR-34, TAR-41, and TAR-43 currently block TAR-42. Linear read-back confirmed all four blockers. The user chose to keep **Prevent production-labelled deployments from using staging hostnames** active and archive the older public landing artifact issue.

The active title for TAR-33 is now **“Separate development web and API domains.”** The archived landing issue retains its P28 title as historical record; it no longer has active relationships.

**Stage/project architecture correction:** The user clarified that each deployment stage is planned around a separate environment and project, and that the public landing separation plan was rethought and was not intended for Linear. The archived issue described the older askolo.app / web.askolo.app / api.askolo.app product-surface split. Linear archived it without moving it to trash and cleared all its relationships, including the former related link to **Prevent production-labelled deployments from using staging hostnames**. That host-guard issue remains active and blocks environment promotion. **P29 Implement landing SEO and GEO** has since been revised to cover stage-specific projects without assuming the old domain split; approved host values remain configuration inputs rather than invented values.

## Local-to-Linear dependency drift

The two planning records do not always express the same sequencing:

- Local tasks #80 and #81 have no declared dependency, while TAR-17 and TAR-18 are blocked by TAR-16.
- Local task #219 depends on merged task #181, while TAR-43 is only related to TAR-42 and has no matching blocker in Linear.
- The local dependency from **Prevent production-labelled deployments from using staging hostnames** to the older public landing separation task remains historical provenance. Its Linear issue is now independent, active, and a blocker of environment promotion; the archived landing issue has no active links.

These differences may be intentional, but maintaining two authoritative dependency graphs would make readiness hard to interpret. Recommended policy: Linear remains authoritative for active planning; local task references remain provenance unless deliberately reconciled into Linear.

## Scope boundaries to make explicit

### Voice, public WebSocket, AI, abuse, security, and governance

Use concise ownership statements to avoid overlapping implementation:

- **TAR-5:** Go-owned AssemblyAI transport, voice-session authority and lifecycle, provider credentials, voice duration, and voice-specific settlement.
- **TAR-44:** browser-facing WebSocket protocol, authentication/session binding at that boundary, message contract, reconnect/resume, and public connection lifecycle.
- **TAR-38:** provider-backed non-voice AI execution.
- **TAR-40:** distributed public-endpoint quotas, concurrency/resource limits, abuse controls, and provider-spend protection.
- **TAR-41:** security review, launch evidence, residual-risk recording, and security gate; it does not replace runtime implementation.
- **TAR-25:** approvals, audit, memory, and user-facing governance.

The agent approvals, memory, and audit scope originally had no relationships. The user chose to relate it to automation triggers and templates, provider webhook ingestion and safe actions, and provider-backed AI execution. These links clarify the approval/audit contract without blocking implementation. Durable automation execution remains unlinked by choice.

### Webhooks and durable automation

The ownership split is sound:

- **TAR-36:** provider verification, durable webhook inbox, deduplication/replay safety, and safe action selection.
- **TAR-26:** durable execution, leases, retries, recovery, and run history.
- **TAR-27:** user-facing trigger, template, notification, and automation configuration.

The user chose provider-specific child tasks under a shared webhook contract. Build provider webhook ingestion and safe actions now owns shared routing, normalized events, inbox persistence, replay protection, dispatch, authorization, and safe action selection. Handle AssemblyAI webhook callbacks safely, Handle Google webhook events safely, and Handle GitHub webhook events safely own their provider adapters and tests. The AssemblyAI child is related to Unify Go AssemblyAI voice platform; the Google child is related to Model multiple Google accounts and Build account-aware Google sync. These links are informational, not blockers. The planned rollout order remains documented as AssemblyAI, Google, then GitHub, with no inter-provider blocker chain.

### Credits, AI, voice, and abuse controls

Keep the contract explicit:

- **TAR-10:** credit policy, reservations, settlement, refunds, grants, and administration.
- **TAR-38:** provider invocation and truthful AI responses.
- **TAR-40:** request/resource abuse limits and provider-spend protections.
- **TAR-5:** voice-duration accounting and voice-specific settlement.

Credit controls do not replace rate limits, and global abuse controls should not reimplement the credit ledger.

## Relationships that may be over-constraining

Review whether these are genuine implementation prerequisites or only shared context:

- The earlier blocker from private analytics evaluation to the public landing artifact scope was cleared when the user later clarified that the outdated landing issue was not intended for Linear and chose to archive it.
- Build the secure persona survey blocks the temporary interactive tutorial, but the user chose to make its link to the Arabic RTL foundation related rather than blocking.
- The keyboard accessibility baseline now blocks Add context menus and commands, where browser-menu overrides and key handling require it. Its links to Add theme presets and Create Arabic RTL foundation are related rather than blocking.

Use `blocks` for required sequencing, `related` for shared context/integration points, and an explicit release-gate relationship only when promotion is actually prohibited without the evidence.

## Replit task-board hygiene

The board contains 242 task records, including historical merged/cancelled items and proposed entries whose titles say they are superseded (for example, entries pointing to #43, #69, #83, #85, or #90). These can look like additional executable work despite having a successor.

Recommended treatment:

- Preserve the successor reference and provenance.
- Mark superseded proposals inactive/cancelled after confirming the canonical successor.
- Do not migrate old test-gap or tech-debt tasks into Linear as standalone issues unless they represent an independent release obligation.
- Attach useful checks to the canonical Linear issue or release-evidence view instead of creating one Linear issue per test gap.

Newer local checks that may improve canonical acceptance criteria include:

- **Fail hung auth release checks before they block publishing:** not added to a canonical scope by the user's choice; leave it outside current Linear acceptance criteria pending local-board cleanup.
- **Verify every schema-isolated integration check works without superuser access:** added to Establish executable Go migrations as a least-privilege integration-test requirement.
- **Verify MFA recovery limits across separate backend processes:** added to P09 Add MFA recovery and trusted devices as a shared-state, cross-process verification requirement.
- **Confirm every published API entry point responds after forwarding:** already represented by the published-route issue and now blocks environment promotion.
- **Make the one-command workspace build work in a fresh shell:** kept separate from the current canonical scopes by the user's choice.
- **Catch MFA alert-policy drift before it reaches production:** added to P09 Add MFA recovery and trusted devices as a pre-production policy-drift check with privacy-safe failure evidence.

These checks enrich existing Linear scopes where approved; none creates a duplicate standalone Linear issue. Local-board cleanup remains deferred until the end of the reconciliation.

## Title and description consistency

Two description headings that did not match their Linear issue titles have been aligned. Only the headings changed; the remaining descriptions and migration provenance were preserved and verified by read-back.

- **Prove production recovery without the archived Drizzle tooling:** the previous heading “Prove production recovery objectives” now matches the issue title.
- **Build provider webhook ingestion and safe actions:** the previous heading “Process provider updates safely” now matches the issue title.

## Recommended decision sequence

1. **Complete:** TAR-37 is a duplicate of **Complete the public WebSocket protocol**; the latter remains canonical.
2. **Complete:** **Complete public-route security review** and **Confirm every published API entry point responds after forwarding** block **Complete environment promotion safeguards**, alongside the development-domain and production-host guards.
3. **Complete:** The active development-domain issue is titled **Separate development web and API domains**; the obsolete public-artifact issue is archived.
4. **Complete:** Linear is authoritative for active sequencing; local dependencies remain provenance unless deliberately reconciled.
5. **Complete:** Read-back confirmed the voice, governance, webhook, AI, abuse, and security ownership links. The voice platform blocks the browser WebSocket protocol; governance remains related to trigger configuration, webhook ingestion, and provider-backed AI, but unlinked to durable automation by choice.
6. **Complete:** **Build provider webhook ingestion and safe actions** has three provider-specific children; the rollout order is documented without inter-provider blocker links.
7. **Complete:** Relationship review confirmed the intended survey/tutorial and keyboard/context-menu blockers; the RTL/theme links remain related, and the outdated public landing issue is archived.
8. **Complete:** Approved newer release checks were added to canonical scopes; checks the user chose to keep separate remain separate.
9. **Pending last:** Clean up superseded local proposals after canonical successor mappings are confirmed.

All approved Linear changes recorded in this audit have been applied and read back. Local-board cleanup remains deferred until the final reconciliation step; unapproved suggestions remain proposals.

## Decision log

- **TAR-37/TAR-44:** TAR-37 was marked `Duplicate` of TAR-44 in Linear; the relation was read back and verified. TAR-44's existing relationships remain intact.
- **TAR-42 promotion gate:** The user chose to make both TAR-41 (security review) and TAR-43 (published-route validation) blockers. These were moved from related links to blockers; the existing TAR-33 and TAR-34 blockers were preserved. Linear read-back verified all four blockers.
- **P28 title collision:** The user chose to remove the P28 prefix from the active development-domain issue; its title is now “Separate development web and API domains.” The old P28 landing issue is archived; local task #93's original wording is retained as provenance pending local-board cleanup.
- **Dependency authority:** Per the existing project convention, Linear is authoritative for active sequencing. Local task dependencies remain historical provenance unless a specific discrepancy warrants a deliberate Linear change.
- **Public landing issue:** The user later clarified that the old product-surface split was not intended for Linear under the revised stage/project plan and chose to archive **P28 Separate the public landing artifact** while keeping the production-host guard active. Linear read-back confirmed the archived issue is not in trash, has no remaining relations, and the host guard remains active. Archiving also cleared the old links from private analytics evaluation and Arabic RTL foundation, and unblocked **P29 Implement landing SEO and GEO**.
- **Persona survey dependencies:** Build the secure persona survey remains a blocker of the temporary interactive tutorial. Its link to the Arabic RTL foundation is now related, so that infrastructure work can proceed independently. The Arabic RTL foundation's other dependencies were preserved.
- **Keyboard accessibility dependencies:** The keyboard accessibility baseline remains a blocker of Add context menus and commands because browser-menu overrides and key handling require it. Its links to Add theme presets and Create Arabic RTL foundation are related.
- **Governance and action scopes:** Agent approvals, memory, and audit is now related to Add automation triggers and templates, Build provider webhook ingestion and safe actions, and Make AI endpoints provider-backed. Build durable automation execution remains unlinked; these links are not blockers.
- **Webhook task structure:** Three provider-specific child tasks now sit under Build provider webhook ingestion and safe actions. Its description defines the shared contract and child ownership; provider-specific test and verification work is assigned to each child. The Google account/sync links now sit on Handle Google webhook events safely only, and the AssemblyAI child is related to Unify Go AssemblyAI voice platform; all three links remain informational. The rollout order is documented without inter-provider blocker links.
- **Newer release checks:** The schema-isolated/no-superuser integration check was added to Establish executable Go migrations; cross-process recovery-throttle verification and pre-production MFA alert-policy drift checking were added to P09 Add MFA recovery and trusted devices. The user kept the fresh-shell build separate and did not select the hung-auth-check timeout for a canonical scope. The published-route smoke check already lives in its own issue and blocks environment promotion.
- **Description headings:** Aligned the headings for Prove production recovery without the archived Drizzle tooling and Build provider webhook ingestion and safe actions with their Linear issue titles. Read-back confirmed the migration comments and remaining description content were preserved.
- **Stage/project architecture correction:** The user clarified that deployment stages use separate environments/projects and the public landing separation plan was rethought and was not intended for Linear. **P28 Separate the public landing artifact** is archived; Linear cleared its relations. **Prevent production-labelled deployments from using staging hostnames** remains active and blocks environment promotion.
- **Landing SEO scope:** **P29 Implement landing SEO and GEO** remains active and blocked by **P22 Roll out Arabic UI and metadata**. Its description now covers public landing SEO/GEO across separate stage projects: production canonical metadata and sitemap use the approved production origin, development and staging are non-indexable, and host-identity validation remains owned by **Prevent production-labelled deployments from using staging hostnames**. No hostnames were invented and no new relationship was added.
- **Cross-owner link verification:** Read-back confirmed the current voice, governance, webhook, provider-backed AI, abuse, security, and browser WebSocket relationships match their scope boundaries. The keyboard-accessibility relationship check is also complete.
- Remaining work is the deferred local-board cleanup; it follows the confirmed successor mappings and approved Linear changes.