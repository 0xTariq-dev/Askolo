---
name: Outbound HTTP cancellation tests
description: Reliable tests for cancellation propagation in provider HTTP clients.
---

For isolated outbound request-cancellation tests, use a controlled `http.RoundTripper` that waits on `request.Context().Done()` and returns `request.Context().Err()`. A blocking `httptest.Server` handler that waits for its request context can remain active and make server shutdown hang in this environment.

**Why:** A provider-upload cancellation test using a blocking test-server handler did not cleanly stop its active connection after cancellation. The controlled transport tested the client behavior without leaving a server goroutine behind.

**How to apply:** Use a custom transport to test context propagation. Use `httptest.Server` when the test needs a real network handshake, redirect behavior, or protocol framing.