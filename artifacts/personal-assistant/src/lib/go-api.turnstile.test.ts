import assert from 'node:assert/strict';
import test from 'node:test';
import { turnstileActionForMode } from './auth-turnstile';
import { goApi } from './go-api';

test('each protected auth API request submits its current Turnstile token', async () => {
  const requests: Array<{ path: string; body: Record<string, unknown> }> = [];
  const originalFetch = globalThis.fetch;

  globalThis.fetch = async (input, init) => {
    const rawURL =
      typeof input === 'string'
        ? input
        : input instanceof URL
          ? input.toString()
          : input.url;
    const url = new URL(rawURL, 'http://askolo.test');
    requests.push({
      path: url.pathname,
      body: JSON.parse(String(init?.body)) as Record<string, unknown>,
    });
    return new Response(JSON.stringify({ status: 'accepted' }), {
      status: 202,
      headers: { 'Content-Type': 'application/json' },
    });
  };

  try {
    await goApi.passwordSignup({
      email: 'signup@example.com',
      password: 'safe-password-123',
      turnstileToken: 'signup-token',
    });
    await goApi.passwordLogin({
      email: 'login@example.com',
      password: 'safe-password-123',
      turnstileToken: 'login-token',
    });
    await goApi.requestPasswordRecovery({
      email: 'recovery@example.com',
      method: 'primary_email',
      turnstileToken: 'password-recovery-first-token',
    });
    await goApi.requestPasswordRecovery({
      email: 'recovery@example.com',
      method: 'primary_email',
      turnstileToken: 'password-recovery-resend-token',
    });
    await goApi.resendEmailVerification({
      email: 'verification@example.com',
      turnstileToken: 'email-resend-token',
    });
    await goApi.requestMFARecovery({
      email: 'mfa@example.com',
      currentPassword: 'safe-password-123',
      turnstileToken: 'mfa-recovery-first-token',
    });
    await goApi.requestMFARecovery({
      email: 'mfa@example.com',
      currentPassword: 'safe-password-123',
      turnstileToken: 'mfa-recovery-resend-token',
    });
  } finally {
    globalThis.fetch = originalFetch;
  }

  assert.deepEqual(
    requests.map(({ path, body }) => [path, body.turnstileToken]),
    [
      ['/api/auth/password/signup', 'signup-token'],
      ['/api/auth/password/login', 'login-token'],
      ['/api/auth/password/recovery/request', 'password-recovery-first-token'],
      ['/api/auth/password/recovery/request', 'password-recovery-resend-token'],
      ['/api/auth/email/resend', 'email-resend-token'],
      ['/api/auth/mfa/recovery/request', 'mfa-recovery-first-token'],
      ['/api/auth/mfa/recovery/request', 'mfa-recovery-resend-token'],
    ],
  );
});

test('auth screens use only the actions required for protected requests', () => {
  assert.equal(turnstileActionForMode('signup'), 'signup');
  assert.equal(turnstileActionForMode('signin'), 'login');
  assert.equal(turnstileActionForMode('verify'), 'email_resend');
  assert.equal(turnstileActionForMode('recovery-request'), 'password_recovery');
  assert.equal(turnstileActionForMode('recovery-method'), 'password_recovery');
  assert.equal(turnstileActionForMode('recovery-verify'), 'password_recovery');
  assert.equal(turnstileActionForMode('mfa-recovery-request'), 'mfa_recovery');
  assert.equal(turnstileActionForMode('mfa-recovery-verify'), 'mfa_recovery');
  assert.equal(turnstileActionForMode('recovery-reset'), null);
  assert.equal(turnstileActionForMode('mfa'), null);
  assert.equal(turnstileActionForMode('mfa-recovery-complete'), null);
});
