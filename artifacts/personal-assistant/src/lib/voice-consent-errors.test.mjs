import test from 'node:test';
import assert from 'node:assert/strict';
import { getVoiceConsentErrorMessage } from './voice-consent-errors.ts';

test('classifies known authorization errors without exposing details', () => {
  assert.equal(
    getVoiceConsentErrorMessage({ code: 'MFA_REQUIRED', requestId: 'private-request-id' }),
    'Complete multi-factor authentication before saving voice consent.',
  );
  assert.equal(
    getVoiceConsentErrorMessage({ code: 'AUTHORIZATION_DENIED', data: { accountId: 'private-account-id' } }),
    'Your account is not currently authorized to use AI voice features.',
  );
  assert.equal(
    getVoiceConsentErrorMessage({ code: 'UNAUTHORIZED', session: 'private-session' }),
    'Your session has expired. Sign in again before saving voice consent.',
  );
});

test('uses generic copy for unknown and malformed errors', () => {
  const message = 'Consent could not be saved. Please try again.';
  assert.equal(getVoiceConsentErrorMessage(new Error('private response body')), message);
  assert.equal(getVoiceConsentErrorMessage({ code: 'INTERNAL_ERROR', detail: 'private detail' }), message);
  assert.equal(getVoiceConsentErrorMessage(null), message);
});