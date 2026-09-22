import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { motion } from 'framer-motion';
import { Github, KeyRound, Sparkles, UserPlus } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useAppAuth } from '@/contexts/auth-context';

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
  | 'mfa';

type RecoveryMethod = 'primary_email' | 'recovery_email';

type AuthPayload = {
  error?: string;
  status?: string;
};

const resendCooldownSeconds = 2 * 60;

class AuthRequestError extends Error {
  retryAfterSeconds: number;

  constructor(message: string, retryAfterSeconds = 0) {
    super(message);
    this.name = 'AuthRequestError';
    this.retryAfterSeconds = retryAfterSeconds;
  }
}

async function postAuth(path: string, body: Record<string, string>): Promise<AuthPayload> {
  const response = await fetch(`${basePath}${path}`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  const payload = (await response.json().catch(() => null)) as AuthPayload | null;
  if (!response.ok) {
    const retryAfterHeader = Number.parseInt(response.headers.get('Retry-After') || '', 10);
    throw new AuthRequestError(
      payload?.error || 'Unable to complete that request right now.',
      Number.isFinite(retryAfterHeader) ? retryAfterHeader : 0,
    );
  }
  return payload ?? {};
}

export function LoginPage() {
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
        const payload = await postAuth('/api/auth/password/login', { email, password });
        if (payload.status === 'mfa_required') {
          setMode('mfa');
          setNotice('Enter an authenticator code or one of your recovery codes to continue.');
        } else {
          window.location.assign(`${basePath}/dashboard`);
        }
      } else if (mode === 'signup') {
        await postAuth('/api/auth/password/signup', { email, password });
        setMode('verify');
        startResendCooldown();
        setNotice('If an account can be created for this address, a verification code is on its way.');
      } else if (mode === 'verify') {
        await postAuth('/api/auth/email/verify', { email, code });
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
        await postAuth('/api/auth/password/recovery/request', { email, method: recoveryMethod });
        setMode('recovery-verify');
        startResendCooldown();
        setNotice('If an eligible recovery method matches, a reset code is on its way.');
      } else if (mode === 'recovery-verify') {
        await postAuth('/api/auth/password/recovery/verify', { email, method: recoveryMethod, code });
        setMode('recovery-reset');
        setResendAvailableAt(null);
        setNotice('Code verified. Choose a new password for your Askolo account.');
      } else if (mode === 'mfa') {
        await postAuth('/api/auth/mfa/verify', { code });
        window.location.assign(`${basePath}/dashboard`);
      } else {
        if (newPassword !== confirmPassword) {
          throw new Error('The passwords do not match.');
        }
        await postAuth('/api/auth/password/recovery/reset', {
          email,
          method: recoveryMethod,
          code,
          password: newPassword,
        });
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
      await postAuth('/api/auth/email/resend', { email });
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
      await postAuth('/api/auth/password/recovery/request', { email, method: recoveryMethod });
      startResendCooldown();
      setNotice('If an eligible recovery method matches, a new reset code is on its way.');
    } catch (err) {
      applyRetryAfter(err);
      setError(err instanceof Error ? err.message : 'Unable to resend the recovery code.');
    } finally {
      setSubmitting(false);
    }
  };

  const isPasswordMode = mode === 'signin' || mode === 'signup';
  const showEmailInput = mode === 'verify' || mode === 'recovery-request';
  const showCodeInput = mode === 'verify' || mode === 'recovery-verify' || mode === 'mfa';
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
  }[mode];

  const cooldownLabel = `${Math.floor(resendSeconds / 60)}:${String(resendSeconds % 60).padStart(2, '0')}`;

  return (
    <main className="min-h-screen w-full flex bg-background relative overflow-hidden flex-col items-center justify-center p-4">
      <div className="absolute top-[0%] left-[-10%] w-[60%] h-[60%] bg-primary/10 rounded-full blur-[120px] pointer-events-none" />
      <div className="absolute bottom-[-10%] right-[-10%] w-[50%] h-[50%] bg-blue-600/10 rounded-full blur-[120px] pointer-events-none" />
      <div className="absolute inset-0 opacity-[0.03] pointer-events-none bg-[url('https://grainy-gradients.vercel.app/noise.svg')]" />

      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.8, ease: 'easeOut' }}
        className="z-10 flex flex-col items-center w-full max-w-md text-center"
      >
        <div className="h-20 w-20 bg-card border border-white/10 rounded-2xl flex items-center justify-center mb-8 shadow-2xl shadow-black/50 relative overflow-hidden">
          <div className="absolute inset-0 bg-gradient-to-tr from-primary/20 to-transparent opacity-50" />
          <img src={logoUrl} alt="Askolo" className="h-10 w-10 object-contain relative z-10" />
        </div>

        <h1 className="text-4xl md:text-5xl font-display font-bold text-foreground mb-4 tracking-tight">
          Welcome to <span className="text-primary">Askolo</span>
        </h1>
        <p className="text-muted-foreground text-lg mb-10 max-w-sm font-sans">
          Your beautifully designed mission control for habits, goals, and daily focus.
        </p>

        {isPasswordMode ? (
          <div className="flex w-full max-w-xs flex-col gap-3">
            <Button
              size="lg"
              onClick={() => beginProviderLogin('google', providerIntent)}
              className="w-full rounded-full px-8 py-6 text-lg font-medium shadow-[0_0_40px_-10px_rgba(234,179,8,0.3)]"
              data-testid="button-google-login"
            >
              <Sparkles className="mr-2 h-5 w-5 text-primary-foreground/70" />
              Continue with Google
            </Button>
            <Button
              size="lg"
              variant="outline"
              onClick={() => beginProviderLogin('github', providerIntent)}
              className="w-full rounded-full px-8 py-6 text-lg font-medium border-white/10 hover:bg-white/5"
              data-testid="button-github-login"
            >
              <Github className="mr-2 h-5 w-5" />
              Continue with GitHub
            </Button>
            <form onSubmit={submitPasswordFlow} className="space-y-3 rounded-xl border border-border bg-card p-4 text-left">
              <h2 className="text-lg font-semibold text-foreground">{heading}</h2>
              <p className="text-sm text-muted-foreground">
                {mode === 'signin'
                  ? 'Use your Askolo email and password.'
                  : 'We will ask you to verify your email before signing in.'}
              </p>
              <label htmlFor="auth-email" className="sr-only">Email address</label>
              <Input
                id="auth-email"
                type="email"
                autoComplete="email"
                placeholder="you@example.com"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                required
              />
              <label htmlFor="auth-password" className="sr-only">Password</label>
              <Input
                id="auth-password"
                type="password"
                autoComplete={mode === 'signin' ? 'current-password' : 'new-password'}
                placeholder="Password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                required
              />
              {error && <p className="text-sm text-destructive" role="alert">{error}</p>}
              {notice && <p className="text-sm text-muted-foreground" role="status">{notice}</p>}
              <Button type="submit" className="w-full" disabled={submitting}>
                {submitting ? 'Working…' : mode === 'signin' ? 'Sign in' : 'Create account'}
              </Button>
            </form>
            <div className="flex flex-col items-center gap-1">
              <Button type="button" variant="link" onClick={() => changeMode(mode === 'signin' ? 'signup' : 'signin')}>
                <UserPlus className="mr-2 h-4 w-4" />
                {mode === 'signin' ? 'Create an account with email' : 'I already have an account'}
              </Button>
              <Button type="button" variant="link" onClick={() => changeMode('recovery-request')}>
                Forgot your password?
              </Button>
            </div>
          </div>
        ) : (
          <div className="flex w-full max-w-xs flex-col gap-3">
            <form onSubmit={submitPasswordFlow} className="space-y-3 rounded-xl border border-border bg-card p-4 text-left">
              <h2 className="text-lg font-semibold text-foreground">{heading}</h2>
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
                          : mode === 'mfa'
                            ? 'Verify MFA'
                            : 'Set new password'}
              </Button>
            </form>
            {(mode === 'verify' || mode === 'recovery-verify') && (
              <div className="flex flex-col items-center gap-1 text-center">
                <p className="text-xs text-muted-foreground" role="status" aria-live="polite">
                  {resendSeconds > 0 ? `Resend available in ${cooldownLabel}` : 'Didn’t receive the code?'}
                </p>
                <Button
                  type="button"
                  variant="link"
                  onClick={mode === 'verify' ? resendVerification : resendRecovery}
                  disabled={submitting || resendSeconds > 0}
                >
                  Resend {mode === 'verify' ? 'verification' : 'recovery'} code
                </Button>
              </div>
            )}
            {showRecoveryEmailSummary && (
              <Button type="button" variant="link" onClick={() => changeMode('recovery-request')}>
                Use a different email
              </Button>
            )}
            <Button type="button" variant="link" onClick={() => changeMode('signin')}>
              Back to sign in
            </Button>
          </div>
        )}
      </motion.div>
    </main>
  );
}