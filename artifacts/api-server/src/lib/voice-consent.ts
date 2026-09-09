export const VOICE_CONSENT_VERSION = "voice-consent-2026-09-07-v1";
export const VOICE_CONSENT_REQUIRED_MESSAGE = "Voice transcription consent is required.";

export type VoiceConsentRecord = {
  consentAt?: Date | null;
  consentVersion?: string | null;
};

export function isCurrentVoiceConsent(
  preference: VoiceConsentRecord | null | undefined,
): boolean {
  return Boolean(
    preference?.consentAt &&
      preference.consentVersion === VOICE_CONSENT_VERSION,
  );
}