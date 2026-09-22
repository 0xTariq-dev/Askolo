---
name: PostgreSQL process control in Go tests
description: Prevent pg_ctl child processes from keeping exec output pipes open during database outage tests.
---

When a Go integration test starts PostgreSQL with `exec.Command(...).CombinedOutput()`, pass `pg_ctl -l /dev/null` or a dedicated log file for the restarted server.

**Why:** PostgreSQL can inherit the command's stderr pipe. The server may become ready while `CombinedOutput()` still waits forever for EOF, causing the test to hang during cleanup.

**How to apply:** Use explicit pg_ctl logging whenever a test starts a disposable PostgreSQL instance from Go and waits for the command to return.