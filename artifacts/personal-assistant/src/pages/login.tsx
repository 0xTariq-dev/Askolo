import { Button } from '@workspace/askolo-design-system/components/ui/button';
import { Input } from '@workspace/askolo-design-system/components/ui/input';
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldSeparator,
} from '@workspace/askolo-design-system/components/ui/field';
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@workspace/askolo-design-system/components/ui/input-group';
import { motion, useReducedMotion } from 'framer-motion';
import { Eye, EyeOff, UserPlus } from 'lucide-react';
import { useEffect, useState } from 'react';
import { ApiError } from '@workspace/api-client-react';
import { useAppAuth } from '@/contexts/auth-context';
import { getApiErrorMessage, getRetryAfterSeconds, goApi } from '@/lib/go-api';
import { CCard12AuthCard } from '@/components/examples/c-card-12';
import { CButton60SocialAuthButtons } from '@/components/examples/c-button-60';

const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');
import logoUrl from '/logo.png';

type PasswordMode =
  | 'signin'
  | 'signup'
  | 'verify'
  | 'recovery-request'
  | 'recovery-method'
  | 'recovery-verify'
  | 'recovery-reset'
  | 'mfa'
  | 'mfa-support-request'
  | 'mfa-support-verify'
  | 'mfa-support-complete';

type RecoveryMethod = 'primary_email' | 'recovery_email';

const resendCooldownSeconds = 2 * 60;

class AuthRequestError extends Error {
  retryAfterSeconds: number;
  code?: string | null;
  requestId?: string | null;

  constructor(
    message: string,
    retryAfterSeconds = 0,
    details?: { code?: string | null; requestId?: string | null },
  ) {
    super(message);
    this.name = 'AuthRequestError';
    this.retryAfterSeconds = retryAfterSeconds;
    this.code = details?.code;
    this.requestId = details?.requestId;
  }
}

async function postAuth<T>(request: () => Promise<T>): Promise<T> {
  try {
    return await request();
  } catch (error) {
    const apiError = error instanceof ApiError ? error : undefined;
    throw new AuthRequestError(
      getApiErrorMessage(error, 'Unable to complete that request right now.'),
      getRetryAfterSeconds(error),
      apiError
        ? { code: apiError.code, requestId: apiError.requestId }
        : undefined,
    );
  }
}

export function LoginPage() {
  const prefersReducedMotion = useReducedMotion();
  const { mfaRequired } = useAppAuth();
  const initialMode: PasswordMode = mfaRequired
    ? 'mfa'
    : window.location.pathname.endsWith('/sign-up')
      ? 'signup'
      : 'signin';
  const beginProviderLogin = (provider: 'google' | 'github', intent: 'signin' | 'signup' = 'signin') => {
    const returnTo = `${window.location.pathname}${window.location.search}`;
    window.location.assign(
      `${basePath}/api/auth/${provider}?intent=${intent}&returnTo=${encodeURIComponent(returnTo || '/dashboard')}`,
    );
  };
  const [mode, setMode] = useState<PasswordMode>(initialMode);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [passwordVisible, setPasswordVisible] = useState(false);
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [code, setCode] = useState('');
  const [recoveryMethod, setRecoveryMethod] = useState<RecoveryMethod>('primary_email');
  const [moreWaysOpen, setMoreWaysOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [resendAvailableAt, setResendAvailableAt] = useState<number | null>(null);
  const [resendSeconds, setResendSeconds] = useState(0);
  const providerIntent = mode === 'signup' ? 'signup' : 'signin';

  const startResendCooldown = (seconds = resendCooldownSeconds) => {
    setResendAvailableAt(Date.now() + seconds * 1000);
    setResendSeconds(seconds);
  };

  const applyRetryAfter = (err: unknown) => {
    if (err instanceof AuthRequestError && err.retryAfterSeconds > 0) {
      startResendCooldown(err.retryAfterSeconds);
    }
  };

  useEffect(() => {
    if (!resendAvailableAt) {
      setResendSeconds(0);
      return;
    }
    const timer = window.setInterval(() => {
      const remaining = Math.max(0, Math.ceil((resendAvailableAt - Date.now()) / 1000));
      setResendSeconds(remaining);
      if (remaining === 0) {
        setResendAvailableAt(null);
      }
    }, 1000);
    return () => window.clearInterval(timer);
  }, [resendAvailableAt]);

  const changeMode = (nextMode: PasswordMode) => {
    setMode(nextMode);
    setPasswordVisible(false);
    setError(null);
    setNotice(null);
    if (nextMode === 'signin' || nextMode === 'signup' || nextMode === 'recovery-request') {
      setCode('');
      setNewPassword('');
      setConfirmPassword('');
      setRecoveryMethod('primary_email');
      setMoreWaysOpen(false);
      setResendAvailableAt(null);
      setResendSeconds(0);
    }
  };

  const submitPasswordFlow = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (submitting) return;
    setError(null);
    setNotice(null);
    setSubmitting(true);
    try {
      if (mode === 'signin') {
        const payload = await postAuth(() => goApi.passwordLogin({ email, password }));
        if (payload.status === 'mfa_required') {
          setMode('mfa');
          setNotice('Enter an authenticator code or one of your recovery codes to continue.');
        } else {
          window.location.assign(`${basePath}/dashboard`);
        }
      } else if (mode === 'signup') {
        await postAuth(() => goApi.passwordSignup({ email, password }));
        setMode('verify');
        startResendCooldown();
        setNotice('If an account can be created for this address, a verification code is on its way.');
      } else if (mode === 'verify') {
        await postAuth(() => goApi.verifyEmail({ email, code }));
        setMode('signin');
        setPassword('');
        setCode('');
        setResendAvailableAt(null);
        setNotice('Your email is verified. You can sign in now.');
      } else if (mode === 'recovery-request') {
        setMode('recovery-method');
        setMoreWaysOpen(false);
        setRecoveryMethod('primary_email');
      } else if (mode === 'recovery-method') {
        await postAuth(() => goApi.requestPasswordRecovery({ email, method: recoveryMethod }));
        setMode('recovery-verify');
        startResendCooldown();
        setNotice('If an eligible recovery method matches, a reset code is on its way.');
      } else if (mode === 'recovery-verify') {
        await postAuth(() =>
          goApi.verifyPasswordRecovery({ email, method: recoveryMethod, code }),
        );
        setMode('recovery-reset');
        setResendAvailableAt(null);
        setNotice('Code verified. Choose a new password for your Askolo account.');
      } else if (mode === 'mfa-support-request') {
        await postAuth(() => goApi.requestMFARecoverySupport({ email }));
        setMode('mfa-support-verify');
        startResendCooldown();
        setNotice('If this account is eligible, a verification code is on its way to its verified primary email.');
      } else if (mode === 'mfa-support-verify') {
        const payload = await postAuth(() =>
          goApi.verifyMFARecoverySupport({ email, code }),
        );
        if (payload.status !== 'mfa_recovery_support_review_required') {
          throw new Error('Your recovery request could not be completed.');
        }
        setMode('mfa-support-complete');
        setResendAvailableAt(null);
        setNotice(null);
      } else if (mode === 'mfa') {
        await postAuth(() => goApi.verifyMFA({ code }));
        window.location.assign(`${basePath}/dashboard`);
      } else {
        if (newPassword !== confirmPassword) {
          throw new Error('The passwords do not match.');
        }
        await postAuth(() => goApi.resetPassword({
          email,
          method: recoveryMethod,
          code,
          password: newPassword,
        }));
        setMode('signin');
        setPassword('');
        setNewPassword('');
        setConfirmPassword('');
        setCode('');
        setNotice('Your password was reset. Sign in with the new password.');
      }
    } catch (err) {
      applyRetryAfter(err);
      setError(err instanceof Error ? err.message : 'Unable to complete that request right now.');
    } finally {
      setSubmitting(false);
    }
  };

  const resendVerification = async () => {
    if (submitting || resendSeconds > 0) return;
    setError(null);
    setNotice(null);
    setSubmitting(true);
    try {
      await postAuth(() => goApi.resendEmailVerification({ email }));
      startResendCooldown();
      setNotice('If this account is waiting for verification, a new code is on its way.');
    } catch (err) {
      applyRetryAfter(err);
      setError(err instanceof Error ? err.message : 'Unable to resend the verification code.');
    } finally {
      setSubmitting(false);
    }
  };

  const resendRecovery = async () => {
    if (submitting || resendSeconds > 0) return;
    setError(null);
    setNotice(null);
    setSubmitting(true);
    try {
      await postAuth(() => goApi.requestPasswordRecovery({ email, method: recoveryMethod }));
      startResendCooldown();
      setNotice('If an eligible recovery method matches, a new reset code is on its way.');
    } catch (err) {
      applyRetryAfter(err);
      setError(err instanceof Error ? err.message : 'Unable to resend the recovery code.');
    } finally {
      setSubmitting(false);
    }
  };

  const resendMFARecoverySupport = async () => {
    if (submitting || resendSeconds > 0) return;
    setError(null);
    setNotice(null);
    setSubmitting(true);
    try {
      await postAuth(() => goApi.requestMFARecoverySupport({ email }));
      startResendCooldown();
      setNotice('If this account is eligible, a new verification code is on its way to its verified primary email.');
    } catch (err) {
      applyRetryAfter(err);
      setError(err instanceof Error ? err.message : 'Unable to resend the recovery verification code.');
    } finally {
      setSubmitting(false);
    }
  };

  const isPasswordMode = mode === 'signin' || mode === 'signup';
  const showEmailInput =
    mode === 'verify' || mode === 'recovery-request' || mode === 'mfa-support-request';
  const showCodeInput =
    mode === 'verify' || mode === 'recovery-verify' || mode === 'mfa' || mode === 'mfa-support-verify';
  const showPasswordReset = mode === 'recovery-reset';
  const showRecoveryEmailSummary =
    mode === 'recovery-method' || mode === 'recovery-verify' || mode === 'recovery-reset';
  const recoveryMethodLabel =
    recoveryMethod === 'primary_email' ? 'your primary email' : 'your enrolled recovery email';
  const heading = {
    signin: 'Sign in with email',
    signup: 'Create your account',
    verify: 'Verify your email',
    'recovery-request': 'Forgot your password?',
    'recovery-method': 'Choose how to verify',
    'recovery-verify': 'Enter your recovery code',
    'recovery-reset': 'Set a new password',
    mfa: 'Verify your identity',
    'mfa-support-request': 'Recover access safely',
    'mfa-support-verify': 'Verify your recovery request',
    'mfa-support-complete': 'Recovery request submitted',
  }[mode];
  const cardTitle = isPasswordMode
    ? mode === 'signin'
      ? 'Welcome back.'
      : 'Create your account.'
    : heading;
  const cardDescription = isPasswordMode
    ? mode === 'signin'
      ? 'Sign in to pick up where you left off.'
      : 'We will ask you to verify your email before signing in.'
    : undefined;

  const cooldownLabel = `${Math.floor(resendSeconds / 60)}:${String(resendSeconds % 60).padStart(2, '0')}`;

  return (
    <main className="relative grid min-h-dvh w-full overflow-hidden bg-background lg:grid-cols-2">
      <h1 className="sr-only lg:hidden">Askolo account access</h1>
      <div className="pointer-events-none absolute inset-x-0 top-0 h-72 bg-gradient-to-br from-primary/15 via-primary/5 to-transparent" />
      <section className="relative hidden flex-col justify-center border-r border-border/60 p-10 lg:flex xl:p-16" aria-label="Askolo overview">
        <div className="absolute start-10 top-10 flex items-center gap-4 xl:start-16 xl:top-16">
          <img src={logoUrl} alt="" width={64} height={64} className="h-16 w-16 object-contain" />
          <span className="font-display text-2xl font-semibold tracking-tight">Askolo</span>
        </div>
        <div className="max-w-xl">
          <p className="mb-5 text-sm font-medium uppercase tracking-wider text-primary">A calmer way to move through the day</p>
          <h1 className="max-w-2xl font-display text-5xl font-semibold leading-tight tracking-tight xl:text-7xl">
            Keep the important things moving.
          </h1>
          <p className="mt-6 max-w-lg text-lg leading-relaxed text-muted-foreground">
            Bring your habits, plans, goals, notes, and calendar into one focused workspace that helps you decide what comes next.
          </p>
          <div className="mt-10 grid max-w-lg grid-cols-2 gap-3">
            <div className="rounded-xl border border-border bg-card/70 p-4">
              <p className="text-2xl font-display font-semibold text-primary">01</p>
              <p className="mt-2 text-sm text-muted-foreground">Choose today’s focus</p>
            </div>
            <div className="rounded-xl border border-border bg-card/70 p-4">
              <p className="text-2xl font-display font-semibold text-primary">02</p>
              <p className="mt-2 text-sm text-muted-foreground">Make steady progress</p>
            </div>
          </div>
        </div>
      </section>

      <motion.div
        initial={prefersReducedMotion ? false : { opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={prefersReducedMotion ? { duration: 0 } : { duration: 0.8, ease: 'easeOut' }}
        className="relative z-10 flex w-full flex-col items-center justify-center px-4 py-10 text-center sm:px-8 lg:min-h-dvh"
      >
        <div className="mb-10 flex items-center gap-4 lg:hidden">
          <img src={logoUrl} alt="" width={64} height={64} className="h-14 w-14 object-contain" />
          <span className="font-display text-2xl font-semibold tracking-tight">Askolo</span>
        </div>

        <CCard12AuthCard
          eyebrow={isPasswordMode ? 'Your workspace awaits' : 'Secure account access'}
          title={cardTitle}
          description={cardDescription}
        >

        {isPasswordMode ? (
          <div className="flex w-full flex-col gap-5">
            <form onSubmit={submitPasswordFlow} className="space-y-4 text-left">
              <FieldGroup className="gap-4">
                <Field>
                  <FieldLabel htmlFor="auth-email">Email address</FieldLabel>
                  <Input
                    id="auth-email"
                    type="email"
                    autoComplete="email"
                    placeholder="you@example.com"
                    value={email}
                    onChange={(event) => setEmail(event.target.value)}
                    aria-invalid={Boolean(error)}
                    aria-describedby={error ? 'auth-form-error' : undefined}
                    required
                  />
                </Field>
                <Field>
                  <div className="flex items-center justify-between gap-3">
                    <FieldLabel htmlFor="auth-password">Password</FieldLabel>
                    <Button
                      type="button"
                      variant="link"
                      size="sm"
                      className="h-auto px-0 py-0 text-xs"
                      onClick={() => changeMode('recovery-request')}
                    >
                      Forgot password?
                    </Button>
                  </div>
                  <InputGroup>
                    <InputGroupInput
                      id="auth-password"
                      type={passwordVisible ? 'text' : 'password'}
                      autoComplete={mode === 'signin' ? 'current-password' : 'new-password'}
                      placeholder="Password"
                      value={password}
                      onChange={(event) => setPassword(event.target.value)}
                      aria-invalid={Boolean(error)}
                      aria-describedby={error ? 'auth-form-error' : undefined}
                      required
                    />
                    <InputGroupAddon align="inline-end">
                      <InputGroupButton
                        type="button"
                        size="icon-sm"
                        aria-label={passwordVisible ? 'Hide password' : 'Show password'}
                        aria-pressed={passwordVisible}
                        onClick={() => setPasswordVisible((visible) => !visible)}
                      >
                        {passwordVisible
                          ? <EyeOff aria-hidden="true" />
                          : <Eye aria-hidden="true" />}
                      </InputGroupButton>
                    </InputGroupAddon>
                  </InputGroup>
                </Field>
              </FieldGroup>
              {error && <p id="auth-form-error" className="text-sm text-destructive" role="alert">{error}</p>}
              {notice && <p className="text-sm text-muted-foreground" role="status">{notice}</p>}
              <div className="flex justify-center">
                <Button
                  type="submit"
                  size="sm"
                  className="min-h-11 w-full max-w-44 rounded-full text-center"
                  disabled={submitting}
                >
                  {submitting ? 'Working…' : mode === 'signin' ? 'Sign in' : 'Create account'}
                </Button>
              </div>
            </form>
            <FieldSeparator className="text-xs">Or continue with</FieldSeparator>
            <CButton60SocialAuthButtons
              intent={providerIntent}
              onGoogleClick={() => beginProviderLogin('google', providerIntent)}
              onGithubClick={() => beginProviderLogin('github', providerIntent)}
            />
            <div className="flex flex-col items-center gap-1">
              <Button type="button" variant="link" onClick={() => changeMode(mode === 'signin' ? 'signup' : 'signin')}>
                <UserPlus className="mr-2 h-4 w-4" />
                {mode === 'signin' ? 'Create an account with email' : 'I already have an account'}
              </Button>
            </div>
          </div>
        ) : mode === 'mfa-support-complete' ? (
           <div className="mt-7 flex w-full flex-col gap-4">
            <p className="text-sm text-muted-foreground">
              We verified control of your primary email and signed out all active sessions.
              Askolo has not disabled MFA or created a new session.
            </p>
            <p className="text-sm text-muted-foreground">
              Support must complete an additional identity review before restoring access.
              An email address or password-only request cannot bypass MFA.
            </p>
            <Button type="button" onClick={() => changeMode('signin')}>
              Back to sign in
            </Button>
          </div>
        ) : (
           <div className="flex w-full flex-col gap-3">
            <form onSubmit={submitPasswordFlow} className="space-y-3 text-left">
              <p className="text-sm text-muted-foreground">
                {mode === 'verify'
                  ? 'Enter the six-digit code sent to your email.'
                  : mode === 'recovery-request'
                    ? 'Enter the primary email address on your Askolo account.'
                    : mode === 'recovery-method'
                      ? 'Choose where to send your recovery code.'
                    : mode === 'recovery-verify'
                      ? `Enter the six-digit code sent to ${recoveryMethodLabel}.`
                      : mode === 'recovery-reset'
                        ? 'Choose a strong password you have not used elsewhere.'
                          : mode === 'mfa-support-request'
                            ? 'If you have lost both your authenticator and every recovery code, we can verify control of your already-verified primary email and send the request to support. This will not sign you in.'
                            : mode === 'mfa-support-verify'
                              ? 'Enter the six-digit code sent to your verified primary email. This confirms email control only; support still must verify your identity.'
                        : mode === 'mfa'
                          ? 'Enter the six-digit code from your authenticator app, or use a recovery code.'
                          : ''}
              </p>
              {showEmailInput && (
                <>
                  <label htmlFor="flow-email" className="sr-only">Email address</label>
                  <Input
                    id="flow-email"
                    type="email"
                    autoComplete="email"
                    placeholder="you@example.com"
                    value={email}
                    onChange={(event) => setEmail(event.target.value)}
                    required
                  />
                </>
              )}
              {showRecoveryEmailSummary && (
                <div className="rounded-lg border border-border bg-muted/40 px-3 py-2 text-sm text-muted-foreground">
                  Account email: <span className="font-medium text-foreground">{email}</span>
                </div>
              )}
              {mode === 'recovery-method' && (
                <fieldset className="space-y-2">
                  <legend className="text-sm font-medium text-foreground">Recovery method</legend>
                  <label
                    className={`flex cursor-pointer items-start gap-3 rounded-lg border p-3 ${
                      recoveryMethod === 'primary_email'
                        ? 'border-primary/50 bg-primary/10'
                        : 'border-border'
                    }`}
                  >
                    <input
                      type="radio"
                      name="recovery-method"
                      value="primary_email"
                      checked={recoveryMethod === 'primary_email'}
                      onChange={() => setRecoveryMethod('primary_email')}
                      className="mt-1 accent-primary"
                    />
                    <span>
                      <span className="block text-sm font-medium text-foreground">Email a code to my primary email</span>
                      <span className="block text-xs text-muted-foreground">Recommended</span>
                    </span>
                  </label>
                  <Button
                    type="button"
                    variant="ghost"
                    className="h-auto w-full justify-between px-2 py-2 text-sm"
                    onClick={() => setMoreWaysOpen((open) => !open)}
                    aria-expanded={moreWaysOpen}
                    aria-controls="more-recovery-methods"
                  >
                    More ways to verify
                    <span aria-hidden="true">{moreWaysOpen ? '−' : '+'}</span>
                  </Button>
                  {moreWaysOpen && (
                    <div id="more-recovery-methods">
                      <label
                        className={`flex cursor-pointer items-start gap-3 rounded-lg border p-3 ${
                          recoveryMethod === 'recovery_email'
                            ? 'border-primary/50 bg-primary/10'
                            : 'border-border'
                        }`}
                      >
                        <input
                          type="radio"
                          name="recovery-method"
                          value="recovery_email"
                          checked={recoveryMethod === 'recovery_email'}
                          onChange={() => setRecoveryMethod('recovery_email')}
                          className="mt-1 accent-primary"
                        />
                        <span>
                          <span className="block text-sm font-medium text-foreground">Use my enrolled recovery email</span>
                          <span className="block text-xs text-muted-foreground">
                            This option is available only if you previously added and verified one.
                          </span>
                        </span>
                      </label>
                    </div>
                  )}
                </fieldset>
              )}
              {showCodeInput && (
                <>
                  <label htmlFor="flow-code" className="sr-only">
                    {mode === 'mfa' ? 'Authenticator or recovery code' : 'Six-digit verification code'}
                  </label>
                  <Input
                    id="flow-code"
                    inputMode={mode === 'mfa' ? 'text' : 'numeric'}
                    autoComplete={mode === 'mfa' ? 'one-time-code' : 'one-time-code'}
                    placeholder={mode === 'mfa' ? '123456 or ABCD-1234-5678-9ABC' : '123456'}
                    value={code}
                    onChange={(event) =>
                      setCode(
                        mode === 'mfa'
                          ? event.target.value.toUpperCase().replace(/[^A-F0-9-]/g, '').slice(0, 19)
                          : event.target.value.replace(/\D/g, '').slice(0, 6),
                      )
                    }
                    minLength={mode === 'mfa' ? 6 : 6}
                    maxLength={mode === 'mfa' ? 19 : 6}
                    required
                  />
                </>
              )}
              {showPasswordReset && (
                <>
                  <label htmlFor="new-password" className="sr-only">New password</label>
                  <Input
                    id="new-password"
                    type="password"
                    autoComplete="new-password"
                    placeholder="New password"
                    value={newPassword}
                    onChange={(event) => setNewPassword(event.target.value)}
                    required
                  />
                  <label htmlFor="confirm-password" className="sr-only">Confirm new password</label>
                  <Input
                    id="confirm-password"
                    type="password"
                    autoComplete="new-password"
                    placeholder="Confirm new password"
                    value={confirmPassword}
                    onChange={(event) => setConfirmPassword(event.target.value)}
                    required
                  />
                </>
              )}
              {error && <p className="text-sm text-destructive" role="alert">{error}</p>}
              {notice && <p className="text-sm text-muted-foreground" role="status">{notice}</p>}
              <Button type="submit" className="w-full" disabled={submitting}>
                {submitting
                  ? 'Working…'
                  : mode === 'verify'
                    ? 'Verify email'
                    : mode === 'recovery-request'
                      ? 'Send recovery code'
                      : mode === 'recovery-method'
                        ? 'Send recovery code'
                        : mode === 'recovery-verify'
                          ? 'Verify code'
                          : mode === 'mfa-support-request'
                            ? 'Send verification code'
                            : mode === 'mfa-support-verify'
                              ? 'Verify and contact support'
                          : mode === 'mfa'
                            ? 'Verify MFA'
                            : 'Set new password'}
              </Button>
            </form>
            {mode === 'mfa' && (
              <div className="rounded-lg border border-border bg-muted/30 p-3 text-left">
                <p className="text-sm font-medium text-foreground">Lost your authenticator and all recovery codes?</p>
                <p className="mt-1 text-xs text-muted-foreground">
                  Use the support review path. It verifies your primary email, revokes sessions, and never bypasses MFA automatically.
                </p>
                <Button type="button" variant="link" className="h-auto px-0 pt-2" onClick={() => changeMode('mfa-support-request')}>
                  Start a safe recovery review
                </Button>
              </div>
            )}
            {(mode === 'verify' || mode === 'recovery-verify' || mode === 'mfa-support-verify') && (
              <div className="flex flex-col items-center gap-1 text-center">
                <p className="text-xs text-muted-foreground" role="status" aria-live="polite">
                  {resendSeconds > 0 ? `Resend available in ${cooldownLabel}` : 'Didn’t receive the code?'}
                </p>
                <Button
                  type="button"
                  variant="link"
                  onClick={
                    mode === 'verify'
                      ? resendVerification
                      : mode === 'mfa-support-verify'
                        ? resendMFARecoverySupport
                        : resendRecovery
                  }
                  disabled={submitting || resendSeconds > 0}
                >
                  Resend {mode === 'verify' ? 'verification' : mode === 'mfa-support-verify' ? 'recovery' : 'password recovery'} code
                </Button>
              </div>
            )}
            {showRecoveryEmailSummary && (
              <Button type="button" variant="link" onClick={() => changeMode('recovery-request')}>
                Use a different email
              </Button>
            )}
            <Button type="button" variant="link" onClick={() => changeMode(mode === 'mfa-support-request' || mode === 'mfa-support-verify' ? 'mfa' : 'signin')}>
              Back to sign in
            </Button>
          </div>
        )}
        </CCard12AuthCard>
      </motion.div>
    </main>
  );
}