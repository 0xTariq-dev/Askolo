import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldSeparator,
} from '@/components/ui/field';
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@/components/ui/input-group';
import { Checkbox } from '@/components/ui/checkbox';
import { motion, useReducedMotion } from 'framer-motion';
import { Eye, EyeOff, UserPlus } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { ApiError } from '@workspace/api-client-react';
import { useAppAuth } from '@/contexts/auth-context';
import {
  ensureDeviceFingerprint,
  getApiErrorMessage,
  getRetryAfterSeconds,
  goApi,
} from '@/lib/go-api';
import { CCard12AuthCard } from '@/components/examples/c-card-12';
import { CButton60SocialAuthButtons } from '@/components/examples/c-button-60';
import { CInputOtp6 } from '@/components/examples/c-input-otp-6';
import { CInput23PasswordFields } from '@/components/examples/c-input-23';
import { TurnstileWidget } from '@/components/auth/turnstile-widget';
import { turnstileActionForMode, type TurnstileAction } from '@/lib/auth-turnstile';

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
  | 'mfa-recovery-request'
  | 'mfa-recovery-verify'
  | 'mfa-recovery-complete';

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
    ensureDeviceFingerprint();
    const returnTo = `${basePath}/dashboard${window.location.search}`;
    window.location.assign(
      `${basePath}/api/auth/${provider}?intent=${intent}&returnTo=${encodeURIComponent(returnTo)}`,
    );
  };
  const [mode, setMode] = useState<PasswordMode>(initialMode);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [passwordVisible, setPasswordVisible] = useState(false);
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [code, setCode] = useState('');
  const [mfaUsingRecoveryCode, setMfaUsingRecoveryCode] = useState(false);
  const [trustDevice, setTrustDevice] = useState(false);
  const [recoveryMethod, setRecoveryMethod] = useState<RecoveryMethod>('primary_email');
  const [moreWaysOpen, setMoreWaysOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [resendAvailableAt, setResendAvailableAt] = useState<number | null>(null);
  const [resendSeconds, setResendSeconds] = useState(0);
  const turnstileAction = turnstileActionForMode(mode);
  const turnstileActionRef = useRef(turnstileAction);
  turnstileActionRef.current = turnstileAction;
  const [turnstileCredential, setTurnstileCredential] = useState<{
    action: TurnstileAction;
    token: string;
  } | null>(null);
  const turnstileCredentialRef = useRef<typeof turnstileCredential>(null);
  const [turnstileGeneration, setTurnstileGeneration] = useState(0);
  const providerIntent = mode === 'signup' ? 'signup' : 'signin';

  const receiveTurnstileToken = useCallback((action: TurnstileAction, token: string | null) => {
    if (turnstileActionRef.current !== action) return;
    const credential = token ? { action, token } : null;
    turnstileCredentialRef.current = credential;
    setTurnstileCredential(credential);
  }, []);

  const resetTurnstile = useCallback(() => {
    turnstileCredentialRef.current = null;
    setTurnstileCredential(null);
    setTurnstileGeneration((generation) => generation + 1);
  }, []);

  const requiredTurnstileToken = useCallback((action: TurnstileAction) => {
    const credential = turnstileCredentialRef.current;
    if (!credential || credential.action !== action) {
      throw new Error('Complete the security check before continuing.');
    }
    return credential.token;
  }, []);

  useEffect(() => {
    resetTurnstile();
  }, [resetTurnstile, turnstileAction]);

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
    resetTurnstile();
    setMode(nextMode);
    setPasswordVisible(false);
    setError(null);
    setNotice(null);
    if (nextMode === 'signin' || nextMode === 'signup' || nextMode === 'recovery-request') {
      setPassword('');
      setCode('');
      setNewPassword('');
      setConfirmPassword('');
      setMfaUsingRecoveryCode(false);
      setRecoveryMethod('primary_email');
      setMoreWaysOpen(false);
      setResendAvailableAt(null);
      setResendSeconds(0);
    }
    if (nextMode === 'mfa-recovery-request') {
      setPassword('');
      setCode('');
    }
  };

  const submitPasswordFlow = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (submitting) return;
    setError(null);
    setNotice(null);
    setSubmitting(true);
    let turnstileSubmitted = false;
    try {
      if (mode === 'signin') {
        const turnstileToken = requiredTurnstileToken('login');
        turnstileSubmitted = true;
        const payload = await postAuth(() => goApi.passwordLogin({ email, password, turnstileToken }));
        if (payload.status === 'mfa_required') {
          setMode('mfa');
          setMfaUsingRecoveryCode(false);
          setCode('');
          setPassword('');
          setNotice('Enter an authenticator code or one of your recovery codes to continue.');
        } else {
          window.location.assign(`${basePath}/dashboard`);
        }
      } else if (mode === 'signup') {
        if (password !== confirmPassword) {
          throw new Error('The passwords do not match.');
        }
        const turnstileToken = requiredTurnstileToken('signup');
        turnstileSubmitted = true;
        await postAuth(() => goApi.passwordSignup({ email, password, turnstileToken }));
        setPassword('');
        setConfirmPassword('');
        setCode('');
        setMode('verify');
        startResendCooldown();
        setNotice('If an account can be created for this address, a verification email may arrive shortly. Check spam if you don’t see it.');
      } else if (mode === 'verify') {
        await postAuth(() => goApi.verifyEmail({ email, code }));
        setMode('signin');
        setPassword('');
        setCode('');
        setResendAvailableAt(null);
        setNotice('Your email is verified. You can sign in now.');
      } else if (mode === 'recovery-request') {
        const turnstileToken = requiredTurnstileToken('password_recovery');
        turnstileSubmitted = true;
        await postAuth(() => goApi.requestPasswordRecovery({
          email,
          method: 'primary_email',
          turnstileToken,
        }));
        setRecoveryMethod('primary_email');
        setMode('recovery-verify');
        startResendCooldown();
        setNotice('If your account is eligible, a password-reset code is on its way. You do not need your current password to choose a new one.');
      } else if (mode === 'recovery-method') {
        const turnstileToken = requiredTurnstileToken('password_recovery');
        turnstileSubmitted = true;
        await postAuth(() => goApi.requestPasswordRecovery({
          email,
          method: recoveryMethod,
          turnstileToken,
        }));
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
      } else if (mode === 'mfa-recovery-request') {
        const turnstileToken = requiredTurnstileToken('mfa_recovery');
        turnstileSubmitted = true;
        await postAuth(() => goApi.requestMFARecovery({
          email,
          currentPassword: password,
          turnstileToken,
        }));
        setMode('mfa-recovery-verify');
        startResendCooldown();
        setNotice('If this account is eligible, a recovery code is on its way.');
      } else if (mode === 'mfa-recovery-verify') {
        await postAuth(() => goApi.verifyMFARecovery({
          email,
          currentPassword: password,
          code,
        }));
        setMode('mfa-recovery-complete');
        setPassword('');
        setCode('');
        setResendAvailableAt(null);
        setNotice('If this account is eligible, recovery was completed. Sign in again to continue.');
      } else if (mode === 'mfa') {
        await postAuth(() => goApi.verifyMFA({ code, trustDevice }));
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
      const isMFARecovery =
        mode === 'mfa-recovery-request' || mode === 'mfa-recovery-verify';
      setError(
        isMFARecovery
          ? 'The recovery request could not be completed. Check your details and try again.'
          : err instanceof Error
            ? err.message
            : 'Unable to complete that request right now.',
      );
    } finally {
      if (turnstileSubmitted) resetTurnstile();
      setSubmitting(false);
    }
  };

  const resendVerification = async () => {
    if (submitting || resendSeconds > 0) return;
    setError(null);
    setNotice(null);
    setSubmitting(true);
    let turnstileSubmitted = false;
    try {
      const turnstileToken = requiredTurnstileToken('email_resend');
      turnstileSubmitted = true;
      await postAuth(() => goApi.resendEmailVerification({ email, turnstileToken }));
      startResendCooldown();
      setNotice('If this account is eligible for verification, a new code should arrive. Delivery may be delayed; check spam or try again after the timer.');
    } catch (err) {
      applyRetryAfter(err);
      setError(err instanceof Error ? err.message : 'Unable to resend the verification code.');
    } finally {
      if (turnstileSubmitted) resetTurnstile();
      setSubmitting(false);
    }
  };

  const resendRecovery = async () => {
    if (submitting || resendSeconds > 0) return;
    setError(null);
    setNotice(null);
    setSubmitting(true);
    let turnstileSubmitted = false;
    try {
      const turnstileToken = requiredTurnstileToken('password_recovery');
      turnstileSubmitted = true;
      await postAuth(() => goApi.requestPasswordRecovery({
        email,
        method: recoveryMethod,
        turnstileToken,
      }));
      startResendCooldown();
      setNotice('If an eligible recovery method matches, a new reset code is on its way.');
    } catch (err) {
      applyRetryAfter(err);
      setError(err instanceof Error ? err.message : 'Unable to resend the recovery code.');
    } finally {
      if (turnstileSubmitted) resetTurnstile();
      setSubmitting(false);
    }
  };

  const resendMFARecovery = async () => {
    if (submitting || resendSeconds > 0) return;
    setError(null);
    setNotice(null);
    setSubmitting(true);
    let turnstileSubmitted = false;
    try {
      const turnstileToken = requiredTurnstileToken('mfa_recovery');
      turnstileSubmitted = true;
      await postAuth(() => goApi.requestMFARecovery({
        email,
        currentPassword: password,
        turnstileToken,
      }));
      startResendCooldown();
      setNotice('If this account is eligible, a new recovery code is on its way.');
    } catch (err) {
      applyRetryAfter(err);
      setError(
        'The recovery request could not be completed. Check your details and try again.',
      );
    } finally {
      if (turnstileSubmitted) resetTurnstile();
      setSubmitting(false);
    }
  };

  const isPasswordMode = mode === 'signin' || mode === 'signup';
  const showEmailInput =
    mode === 'verify' ||
    mode === 'recovery-request' ||
    mode === 'mfa-recovery-request' ||
    mode === 'mfa-recovery-verify';
  const showOtpCodeInput =
    mode === 'verify' ||
    mode === 'recovery-verify' ||
    mode === 'mfa-recovery-verify' ||
    (mode === 'mfa' && !mfaUsingRecoveryCode);
  const showMFARecoveryPassword =
    mode === 'mfa-recovery-request' || mode === 'mfa-recovery-verify';
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
    'mfa-recovery-request': 'Recover MFA access',
    'mfa-recovery-verify': 'Verify MFA recovery',
    'mfa-recovery-complete': 'MFA recovery complete',
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
      <section dir="auto" lang="en" className="relative hidden flex-col justify-between border-e border-border/60 p-10 lg:flex xl:p-16" aria-label="Askolo overview">
        <div className="flex items-center gap-4">
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
            <form onSubmit={submitPasswordFlow} className="space-y-4 text-start">
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
                {mode === 'signup' ? (
                  <CInput23PasswordFields
                    idPrefix="signup"
                    password={password}
                    confirmation={confirmPassword}
                    onPasswordChange={setPassword}
                    onConfirmationChange={setConfirmPassword}
                    passwordLabel="Password"
                    confirmationLabel="Confirm password"
                    errorDescribedBy={error ? 'auth-form-error' : undefined}
                  />
                ) : (
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
                        autoComplete="current-password"
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
                )}
              </FieldGroup>
              {turnstileAction && (
                <TurnstileWidget
                  key={`${turnstileAction}-${turnstileGeneration}`}
                  action={turnstileAction}
                  tokenReady={turnstileCredential?.action === turnstileAction}
                  onToken={receiveTurnstileToken}
                />
              )}
              {error && <p id="auth-form-error" className="text-sm text-destructive" role="alert">{error}</p>}
              {notice && <p className="text-sm text-muted-foreground" role="status">{notice}</p>}
              <div className="flex justify-center">
                <Button
                  type="submit"
                  size="sm"
                  className="min-h-11 w-full max-w-44 rounded-full text-center"
                  disabled={
                    submitting ||
                    (mode === 'signup' && password !== confirmPassword)
                  }
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
        ) : mode === 'mfa-recovery-complete' ? (
           <div className="mt-7 flex w-full flex-col gap-4">
            <p className="text-sm text-muted-foreground">
              If the account was eligible, we verified the recovery request and signed out any affected sessions.
              Askolo has not created a new session.
            </p>
            <p className="text-sm text-muted-foreground">
              No account eligibility details are shown here. Return to sign in and use your updated account credentials.
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
                  ? 'If this address is eligible for verification, we’ll send a six-digit code. We can’t confirm inbox delivery here. If no code arrives, delivery may be delayed or the account may not be eligible. Check spam; this same guidance is shown for every address.'
                  : mode === 'recovery-request'
                    ? 'Enter the primary email address on your Askolo account. You do not need your current password to receive a reset code.'
                    : mode === 'recovery-method'
                      ? 'Choose where to send your recovery code.'
                    : mode === 'recovery-verify'
                      ? `Enter the six-digit code sent to ${recoveryMethodLabel}.`
                      : mode === 'recovery-reset'
                        ? 'Choose a strong password you have not used elsewhere.'
                          : mode === 'mfa-recovery-request'
                            ? 'Enter your account email and password. If this account is eligible, a recovery code will be sent.'
                            : mode === 'mfa-recovery-verify'
                              ? 'Enter the code from your recovery email. Account eligibility is not disclosed.'
                        : mode === 'mfa'
                          ? mfaUsingRecoveryCode
                            ? 'Enter one of your unused MFA recovery codes.'
                            : 'Enter the current six-digit code from your authenticator app.'
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
              {showMFARecoveryPassword && (
                <>
                  <label htmlFor="mfa-recovery-password" className="sr-only">Account password</label>
                  <Input
                    id="mfa-recovery-password"
                    type="password"
                    autoComplete="current-password"
                    placeholder="Account password"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
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
                      onChange={() => {
                        setRecoveryMethod('primary_email');
                        resetTurnstile();
                      }}
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
                          onChange={() => {
                            setRecoveryMethod('recovery_email');
                            resetTurnstile();
                          }}
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
              {showOtpCodeInput && (
                <CInputOtp6
                  id="flow-code"
                  label={
                    mode === 'verify'
                      ? 'Email verification code'
                      : mode === 'recovery-verify'
                        ? 'Password reset code'
                        : mode === 'mfa-recovery-verify'
                          ? 'MFA recovery verification code'
                          : 'Authenticator code'
                  }
                  description={
                    mode === 'verify'
                      ? 'Enter the six-digit code sent to your email address.'
                      : mode === 'recovery-verify'
                        ? `Enter the six-digit code sent to ${recoveryMethodLabel}.`
                        : mode === 'mfa-recovery-verify'
                          ? 'Enter the six-digit code from your recovery email.'
                          : 'Enter the current six-digit code from your authenticator app.'
                  }
                  value={code}
                  onChange={setCode}
                  disabled={submitting}
                  ariaDescribedBy={error ? 'auth-form-error' : undefined}
                  ariaInvalid={Boolean(error)}
                  testId="input-flow-code"
                />
              )}
              {mode === 'mfa' && mfaUsingRecoveryCode && (
                <div className="space-y-2">
                  <label htmlFor="flow-code" className="text-sm font-medium">
                    MFA recovery code
                  </label>
                  <Input
                    id="flow-code"
                    inputMode="text"
                    autoComplete="one-time-code"
                    placeholder="ABCD-1234-5678-9ABC"
                    value={code}
                    onChange={(event) => setCode(event.target.value.toUpperCase().replace(/[^A-F0-9-]/g, '').slice(0, 19))}
                    minLength={6}
                    maxLength={19}
                    required
                    disabled={submitting}
                    aria-describedby={error ? 'auth-form-error' : undefined}
                    aria-invalid={Boolean(error)}
                    data-testid="input-mfa-recovery-code"
                  />
                </div>
              )}
              {mode === 'mfa' && (
                <Button
                  type="button"
                  variant="link"
                  size="sm"
                  className="h-auto justify-start px-0"
                  onClick={() => {
                    setMfaUsingRecoveryCode((usingRecoveryCode) => !usingRecoveryCode);
                    setCode('');
                    setError(null);
                  }}
                  disabled={submitting}
                  data-testid="button-toggle-mfa-code-type"
                >
                  {mfaUsingRecoveryCode
                    ? 'Use an authenticator code instead'
                    : 'Use a recovery code instead'}
                </Button>
              )}
              {showPasswordReset && (
                <CInput23PasswordFields
                  idPrefix="reset"
                  password={newPassword}
                  confirmation={confirmPassword}
                  onPasswordChange={setNewPassword}
                  onConfirmationChange={setConfirmPassword}
                  passwordLabel="New password"
                  confirmationLabel="Confirm new password"
                  passwordPlaceholder="Create a strong password"
                  confirmationPlaceholder="Re-enter your new password"
                  errorDescribedBy={error ? 'auth-form-error' : undefined}
                />
              )}
              {(mode === 'recovery-request' ||
                mode === 'recovery-method' ||
                mode === 'mfa-recovery-request') &&
                turnstileAction && (
                <TurnstileWidget
                  key={`${turnstileAction}-${turnstileGeneration}`}
                  action={turnstileAction}
                  tokenReady={turnstileCredential?.action === turnstileAction}
                  onToken={receiveTurnstileToken}
                />
              )}
              {error && <p className="text-sm text-destructive" role="alert">{error}</p>}
              {notice && <p className="text-sm text-muted-foreground" role="status">{notice}</p>}
              <Button
                type="submit"
                className="w-full"
                disabled={
                  submitting ||
                  (showPasswordReset && newPassword !== confirmPassword)
                }
              >
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
                            : mode === 'mfa-recovery-request'
                             ? 'Send recovery code'
                             : mode === 'mfa-recovery-verify'
                               ? 'Verify recovery'
                          : mode === 'mfa'
                            ? 'Verify MFA'
                            : 'Set new password'}
              </Button>
            </form>
            {(mode === 'verify' || mode === 'recovery-verify' || mode === 'mfa-recovery-verify') &&
              turnstileAction && (
                <TurnstileWidget
                  key={`${turnstileAction}-${turnstileGeneration}`}
                  action={turnstileAction}
                  tokenReady={turnstileCredential?.action === turnstileAction}
                  onToken={receiveTurnstileToken}
                />
              )}
            {mode === 'recovery-request' && (
              <Button
                type="button"
                variant="link"
                className="min-h-11"
                onClick={() => changeMode('recovery-method')}
              >
                More recovery options
              </Button>
            )}
            {mode === 'mfa' && (
              <div className="rounded-lg border border-border bg-muted/30 p-3 text-left">
                <div className="flex items-start gap-3">
                  <Checkbox
                    id="trust-this-browser"
                    checked={trustDevice}
                    onCheckedChange={(checked) => setTrustDevice(checked === true)}
                    aria-describedby="trust-this-browser-description"
                  />
                  <div>
                    <label htmlFor="trust-this-browser" className="text-sm font-medium text-foreground">
                      Trust this browser
                    </label>
                    <p id="trust-this-browser-description" className="mt-1 text-xs text-muted-foreground">
                      Skip the MFA challenge on this browser until the trusted device expires.
                    </p>
                  </div>
                </div>
                <p className="mt-4 text-sm font-medium text-foreground">Lost your authenticator and recovery codes?</p>
                <p className="mt-1 text-xs text-muted-foreground">
                  Request a self-service recovery code with your account email and password. This does not sign you in automatically.
                </p>
                <Button type="button" variant="link" className="h-auto px-0 pt-2" onClick={() => changeMode('mfa-recovery-request')}>
                  Start MFA recovery
                </Button>
              </div>
            )}
            {(mode === 'verify' || mode === 'recovery-verify' || mode === 'mfa-recovery-verify') && (
              <div className="flex flex-col items-center gap-1 text-center">
                <p className="text-xs text-muted-foreground" role="status" aria-live="polite">
                  {resendSeconds > 0 ? `Resend available in ${cooldownLabel}` : 'Didn’t receive the code?'}
                </p>
                <Button
                  type="button"
                  variant="link"
                  onClick={
                    mode === 'verify'
                      ? () => resendVerification()
                      : mode === 'mfa-recovery-verify'
                        ? resendMFARecovery
                        : resendRecovery
                  }
                  disabled={submitting || resendSeconds > 0}
                >
                  Resend {mode === 'verify' ? 'verification' : mode === 'mfa-recovery-verify' ? 'MFA recovery' : 'password recovery'} code
                </Button>
              </div>
            )}
            {showRecoveryEmailSummary && (
              <Button type="button" variant="link" onClick={() => changeMode('recovery-request')}>
                Use a different email
              </Button>
            )}
            <Button type="button" variant="link" onClick={() => changeMode(mode === 'mfa-recovery-request' || mode === 'mfa-recovery-verify' ? 'mfa' : 'signin')}>
              Back to sign in
            </Button>
          </div>
        )}
        </CCard12AuthCard>
      </motion.div>
    </main>
  );
}