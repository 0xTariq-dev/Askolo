# Project Planning Audit: Replit Task Board and Askolo Linear

**Reviewed:** 2026-09-24  
**Scope:** Read-only review of the Replit task board, local planning files, and the Askolo-dev Linear project, followed by the explicitly approved TAR-37/TAR-44 duplicate correction.

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

The following issues are related but have separate responsibilities:

- TAR-29: public landing artifact and public/app/API domain separation.
- TAR-33: development web/API domain separation.
- TAR-34: prevent production-labelled deployments from using staging hostnames.
- TAR-42: environment promotion safeguards.
- TAR-43: published API route checks after forwarding.
- TAR-41: public-route security review.

TAR-33 and TAR-34 currently block TAR-42. TAR-43 and TAR-41 are related to TAR-42, not blockers. If route validation and security review are required before promotion, their relations should be upgraded to blockers; otherwise the current relationships should remain informational.

TAR-29 and TAR-33 both carry the “P28” prefix despite describing different work. Suggested normalization: remove the P28 prefix from TAR-33 and title it **“Separate development web and API domains.”**

## Local-to-Linear dependency drift

The two planning records do not always express the same sequencing:

- Local tasks #80 and #81 have no declared dependency, while TAR-17 and TAR-18 are blocked by TAR-16.
- Local task #219 depends on merged task #181, while TAR-43 is only related to TAR-42 and has no matching blocker in Linear.
- Local task #233 depends on #189, while TAR-34 is related to TAR-29 rather than carrying that prerequisite as a blocker.

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

TAR-25 has no relationships despite relevant boundaries with TAR-27 automation actions, TAR-36 webhook-triggered actions, and TAR-38 AI tool/action execution. Suggested next step: add related links and clarify the approval/audit contract, without making TAR-25 a universal blocker for all execution.

### Webhooks and durable automation

The ownership split is sound:

- **TAR-36:** provider verification, durable webhook inbox, deduplication/replay safety, and safe action selection.
- **TAR-26:** durable execution, leases, retries, recovery, and run history.
- **TAR-27:** user-facing trigger, template, notification, and automation configuration.

TAR-36 combines AssemblyAI, Google, and GitHub event families with different prerequisites. Keep one issue with provider-phase milestones and selective dependencies, or split provider-specific work into children under a shared inbox/action contract. Do not make the entire webhook scope wait on one provider's prerequisite if other provider phases can proceed independently. In particular, AssemblyAI and GitHub work need not wait for the Google-specific TAR-14/TAR-15 phase.

### Credits, AI, voice, and abuse controls

Keep the contract explicit:

- **TAR-10:** credit policy, reservations, settlement, refunds, grants, and administration.
- **TAR-38:** provider invocation and truthful AI responses.
- **TAR-40:** request/resource abuse limits and provider-spend protections.
- **TAR-5:** voice-duration accounting and voice-specific settlement.

Credit controls do not replace rate limits, and global abuse controls should not reimplement the credit ledger.

## Relationships that may be over-constraining

Review whether these are genuine implementation prerequisites or only shared context:

- TAR-12 blocks TAR-29. Analytics privacy evaluation may not need to block public landing-artifact work unless analytics is a launch requirement.
- TAR-21 blocks TAR-22 and TAR-23. Persona-survey work may inform tutorial personalization, but may not need to block Arabic RTL foundations.
- TAR-6 blocks TAR-23, TAR-9, and TAR-7. Keep these blockers only if the accessibility work is truly a prerequisite rather than a parallel quality requirement.

Use `blocks` for required sequencing, `related` for shared context/integration points, and an explicit release-gate relationship only when promotion is actually prohibited without the evidence.

## Replit task-board hygiene

The board contains 242 task records, including historical merged/cancelled items and proposed entries whose titles say they are superseded (for example, entries pointing to #43, #69, #83, #85, or #90). These can look like additional executable work despite having a successor.

Recommended treatment:

- Preserve the successor reference and provenance.
- Mark superseded proposals inactive/cancelled after confirming the canonical successor.
- Do not migrate old test-gap or tech-debt tasks into Linear as standalone issues unless they represent an independent release obligation.
- Attach useful checks to the canonical Linear issue or release-evidence view instead of creating one Linear issue per test gap.

Newer local checks that may improve canonical acceptance criteria include:

- #201: fail hung auth release checks before publishing is blocked.
- #203: run schema-isolated integration checks without superuser privileges.
- #210: make the one-command workspace build work in a fresh shell.
- #217: verify MFA recovery limits across backend processes.
- #219: check every published API entry point after forwarding.
- #231: catch MFA alert-policy drift before production.

These should be evaluated against existing scopes before migration; the local tasks are evidence sources, not automatic new Linear work.

## Title and description consistency

Two migrated issues have differing titles and description headings:

- **TAR-35:** issue title “Prove production recovery without the archived Drizzle tooling”; description heading “Prove production recovery objectives.”
- **TAR-36:** issue title “Build provider webhook ingestion and safe actions”; description heading “Process provider updates safely.”

Align the description headings with the Linear issue titles so search results, exports, and copied plans remain consistent.

## Recommended decision sequence

1. Resolve TAR-37/TAR-44 (TAR-44 remains canonical).
2. Decide whether TAR-41 and TAR-43 are mandatory blockers for TAR-42.
3. Normalize the TAR-33 title and resolve the repeated P28 prefix.
4. Decide how strictly to reconcile local dependency history with Linear's authoritative graph.
5. Clarify ownership links across TAR-5, TAR-25, TAR-36, TAR-38, TAR-40, TAR-41, and TAR-44.
6. Choose whether TAR-36 stays one issue with phase milestones or gains provider-specific child issues.
7. Review the potentially over-constraining TAR-12/TAR-29, TAR-21/TAR-22/TAR-23, and TAR-6 relationships.
8. Enhance canonical scopes with applicable newer local release checks.
9. Clean up superseded local proposals only after their canonical successor mappings are confirmed.

Other than the TAR-37/TAR-44 duplicate, TAR-42 promotion gates, and TAR-33 title normalization, the remaining recommendations are findings and proposals, not approved edits.

## Decision log

- **TAR-37/TAR-44:** TAR-37 was marked `Duplicate` of TAR-44 in Linear; the relation was read back and verified. TAR-44's existing relationships remain intact.
- **TAR-42 promotion gate:** The user chose to make both TAR-41 (security review) and TAR-43 (published-route validation) blockers. These were moved from related links to blockers; the existing TAR-33 and TAR-34 blockers were preserved. Linear read-back verified all four blockers.
- **P28 title collision:** The user chose to keep P28 on TAR-29 and remove it from TAR-33. TAR-33 is now titled “Separate development web and API domains”; local task #93's original wording is retained as provenance pending local-board cleanup.
- **Dependency authority:** Per the existing project convention, Linear is authoritative for active sequencing. Local task dependencies remain historical provenance unless a specific discrepancy warrants a deliberate Linear change.
- Remaining recommendations are awaiting the user's decisions.