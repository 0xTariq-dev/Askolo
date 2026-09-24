---
name: Go security upgrades
description: Toolchain verification and module-only vulnerability findings during Go security updates.
---

When upgrading Go beyond the workspace's installed module, ensure the build and validation commands enable the Go checksum database if the environment defaults it to `off`. The automatic toolchain download otherwise fails verification even when the toolchain is available.

**Why:** The managed Go module can lag security patch releases, while the workspace's checksum setting blocks the automatic download until it is enabled for the Go commands.

**How to apply:** Verify a clean build uses the patched toolchain (inspect binary build info), not just the `go` directive.

An x/crypto advisory for the abandoned `openpgp` subpackage has no fixed version. Do not replace password hashing solely to eliminate a module-level finding when the application imports only Argon2 and the call-path scan confirms no affected code.

**Why:** A version bump cannot fix an unfixed subpackage advisory, and replacing the password hashing implementation changes security-sensitive behavior without removing a reachable vulnerability.

**How to apply:** Report module-only findings separately from vulnerabilities in imported packages and called symbols; rerun a source-aware scan when the dependency usage changes.