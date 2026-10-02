import assert from "node:assert/strict";
import test from "node:test";
import {
  formatUsdMicrosInput,
  parseUsdMicros,
  tryParseUsdMicros,
} from "./credit-api.ts";

test("USD input formatting keeps editable decimal text separate from currency adornments", () => {
  assert.equal(formatUsdMicrosInput(1_234_500_000), "1234.50");
  assert.equal(formatUsdMicrosInput(-250_001), "-0.250001");
  assert.equal(formatUsdMicrosInput(0), "0.00");
});

test("strict USD parsing preserves valid values and identifies incomplete or unsafe drafts", () => {
  assert.equal(tryParseUsdMicros("$1,234.50"), 1_234_500_000);
  assert.equal(tryParseUsdMicros("-$0.000001"), -1);
  assert.equal(tryParseUsdMicros("1."), null);
  assert.equal(tryParseUsdMicros(""), null);
  assert.equal(tryParseUsdMicros("0.1234567"), null);
  assert.equal(tryParseUsdMicros("1,23.00"), null);
  assert.equal(tryParseUsdMicros("1,,234.00"), null);
  assert.equal(tryParseUsdMicros("9007199254740.992"), null);
  assert.equal(parseUsdMicros("not a number"), 0);
});