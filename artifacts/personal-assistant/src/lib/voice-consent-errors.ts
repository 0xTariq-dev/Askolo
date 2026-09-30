import { ApiError } from '@workspace/api-client-react';

const GENERIC_VOICE_CONSENT_ERROR =
  'Consent could not be saved. Please try again.';

/**
 * Convert API authorization failures into safe, actionable copy for the
 * consent dialog. This deliberately exposes neither response bodies nor
 * request/session/account identifiers.
 */
export function getVoiceConsentErrorMessage(error: unknown): string {
  const code = error instanceof ApiError
    ? error.code
    : typeof error === 'object' && error !== null && 'code' in error &&
        typeof error.code === 'string'
      ? error.code
      : null;

  switch (code) {
    case 'MFA_REQUIRED':
      return 'Complete multi-factor authentication before saving voice consent.';
    case 'AUTHORIZATION_DENIED':
      return 'Your account is not currently authorized to use AI voice features.';
    case 'UNAUTHORIZED':
      return 'Your session has expired. Sign in again before saving voice consent.';
    default:
      return GENERIC_VOICE_CONSENT_ERROR;
  }
}