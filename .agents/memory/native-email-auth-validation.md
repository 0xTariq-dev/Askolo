---
name: Native email auth validation harness timing
description: The synthetic cleanup regression can race while observing the fake PostgreSQL pid handoff.
---

The native email auth release validation harness may intermittently report that the early-exit case did not start disposable PostgreSQL even though an immediate rerun passes.

**Why:** The harness waits for the fake Go marker but does not independently wait for the fake PostgreSQL pid marker, so process scheduling can expose the handoff race.

**How to apply:** Treat an isolated failure from `test-native-email-auth-validation.sh` as a harness-timing issue when the backend tests and a direct rerun pass; do not change application code to address it.