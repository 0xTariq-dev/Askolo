import assert from "node:assert/strict";
import test from "node:test";
import {
  formatCredits,
  formatCreditsInput,
  formatUsdMicrosInput,
  parseUsdMicros,
  tryParseCreditsToUsdMicros,
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

test("credit display and editable inputs use exact credit-scale precision", () => {
  assert.equal(formatCredits(1_000), "1 credit");
  assert.equal(formatCredits(1_000_000), "1,000 credits");
  assert.equal(formatCredits(1_234_567), "1,234.567 credits");
  assert.equal(formatCredits(-2_500), "-2.5 credits");
  assert.equal(formatCreditsInput(1_234_567), "1234.567");
  assert.equal(formatCreditsInput(-2_500), "-2.5");
});

test("credit drafts convert strictly to internal USD microunits", () => {
  assert.equal(tryParseCreditsToUsdMicros("1,234.567"), 1_234_567);
  assert.equal(tryParseCreditsToUsdMicros("1,000"), 1_000_000);
  assert.equal(tryParseCreditsToUsdMicros("-0.001"), -1);
  assert.equal(tryParseCreditsToUsdMicros("1."), null);
  assert.equal(tryParseCreditsToUsdMicros("0.0001"), null);
  assert.equal(tryParseCreditsToUsdMicros("1,23"), null);
  assert.equal(tryParseCreditsToUsdMicros(""), null);
});