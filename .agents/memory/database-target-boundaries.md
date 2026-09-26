---
name: Current and future database targets
description: Records the selected production database and the separate future external migration target.
---

Current production uses Replit-managed PostgreSQL. A separate external PostgreSQL instance is a future environment; no provider or target has been selected or connected.

**Why:** Production recovery and schema changes depend on which environment actually owns the live database. Treating the future external target as current could lead to unsafe migration or recovery instructions.

**How to apply:** Use Replit's Publish-managed schema flow for current production and keep Go migration execution away from that managed production database. Use the Go runner for disposable/development databases and only for a future external target after it is identified and explicitly approved. Evaluate current production recovery through Replit's documented restore behavior; do not claim RPO/RTO without measured evidence.