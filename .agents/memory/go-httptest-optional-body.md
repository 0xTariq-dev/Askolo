---
name: Optional httptest bodies
description: Avoid typed-nil io.Reader panics when building bodyless HTTP test requests.
---

For optional request bodies passed to `httptest.NewRequest`, declare the variable as the `io.Reader` interface and leave that interface nil when there is no body. A typed nil pointer such as `(*strings.Reader)(nil)` becomes a non-nil interface and can panic inside `httptest.NewRequest`.

**Why:** A bodyless request helper once passed a typed nil reader, causing a test panic before the handler ran and obscuring the actual integration-test result.

**How to apply:** In shared HTTP test helpers, use an `io.Reader` local, assign a concrete reader only when a body exists, and include a bodyless GET in helper tests.
