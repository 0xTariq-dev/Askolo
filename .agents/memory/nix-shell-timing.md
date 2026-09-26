---
name: Shell timing without GNU time
description: Measuring command duration in the workspace's Nix shell.
---

The workspace shell does not provide `/usr/bin/time`. For simple wall-clock measurements, capture `date +%s%N` before and after the command and calculate elapsed seconds; do not rely on the absolute GNU `time` path.

**Why:** The absolute path fails with command-not-found in this environment, while nanosecond timestamps from `date` work.

**How to apply:** When timing migration or build commands, use a shell wrapper around `date +%s%N`; keep the measurement code out of the application.