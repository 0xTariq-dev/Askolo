import assert from "node:assert/strict";
import test from "node:test";

import {
  ApiError,
  customFetch,
  getQueryRetryDelay,
  shouldRetryQuery,
} from "../src/custom-fetch.ts";

function jsonResponse(status, payload, headers = {}) {
  return new Response(JSON.stringify(payload), {
    status,
    statusText: status === 403 ? "Forbidden" : "Service Unavailable",
    headers: {
      "content-type": "application/json",
      ...headers,
    },
  });
}

function apiError(status, payload = {}, headers = {}) {
  const response = jsonResponse(status, payload, headers);
  return new ApiError(response, payload, {
    method: "GET",
    url: "/api/test",
  });
}

test("ApiError exposes the Go error code and request ID", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () =>
    jsonResponse(403, {
      error: "Not allowed",
      code: "FORBIDDEN",
      requestId: "request-123",
    });

  try {
    await assert.rejects(
      customFetch("/api/private"),
      (error) => {
        assert.ok(error instanceof ApiError);
        assert.equal(error.status, 403);
        assert.equal(error.code, "FORBIDDEN");
        assert.equal(error.requestId, "request-123");
        assert.match(error.message, /Not allowed/);
        return true;
      },
    );
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("customFetch forwards cancellation and request credentials", async () => {
  const originalFetch = globalThis.fetch;
  const controller = new AbortController();
  let requestInit;

  globalThis.fetch = async (_input, init) => {
    requestInit = init;
    return new Response("{}", {
      status: 200,
      headers: { "content-type": "application/json" },
    });
  };

  try {
    await customFetch("/api/test", {
      signal: controller.signal,
      credentials: "include",
      responseType: "json",
    });

    assert.equal(requestInit.signal, controller.signal);
    assert.equal(requestInit.credentials, "include");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("query retry policy retries only transient, non-aborted failures once", () => {
  assert.equal(shouldRetryQuery(0, new TypeError("network failed")), true);
  assert.equal(shouldRetryQuery(0, apiError(408)), true);
  assert.equal(shouldRetryQuery(0, apiError(429)), true);
  assert.equal(shouldRetryQuery(0, apiError(503)), true);

  assert.equal(shouldRetryQuery(1, apiError(503)), false);
  assert.equal(shouldRetryQuery(0, apiError(401)), false);
  assert.equal(shouldRetryQuery(0, apiError(403)), false);
  assert.equal(shouldRetryQuery(0, new DOMException("Aborted", "AbortError")), false);
});

test("query retries honor Retry-After and skip excessively long waits", () => {
  const rateLimited = apiError(429, { error: "Rate limited" }, { "retry-after": "12" });
  const longWait = apiError(503, { error: "Unavailable" }, { "retry-after": "120" });

  assert.equal(shouldRetryQuery(0, rateLimited), true);
  assert.equal(getQueryRetryDelay(0, rateLimited), 12_000);
  assert.equal(shouldRetryQuery(0, longWait), false);
});