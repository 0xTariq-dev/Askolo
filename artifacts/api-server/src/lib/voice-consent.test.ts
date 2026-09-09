import assert from "node:assert/strict";
import test from "node:test";

import {
  VOICE_CONSENT_VERSION,
  isCurrentVoiceConsent,
} from "./voice-consent";

test("voice consent requires the current version and a timestamp", () => {
  assert.equal(
    isCurrentVoiceConsent({
      consentAt: new Date("2026-09-09T00:00:00.000Z"),
      consentVersion: VOICE_CONSENT_VERSION,
    }),
    true,
  );
  assert.equal(
    isCurrentVoiceConsent({
      consentAt: new Date("2026-09-09T00:00:00.000Z"),
      consentVersion: "voice-consent-older",
    }),
    false,
  );
  assert.equal(
    isCurrentVoiceConsent({
      consentAt: null,
      consentVersion: VOICE_CONSENT_VERSION,
    }),
    false,
  );
  assert.equal(isCurrentVoiceConsent(undefined), false);
});