import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { motion } from 'framer-motion';
import { Github, KeyRound, Sparkles, UserPlus } from 'lucide-react';
import { useState } from 'react';
import { useAppAuth } from '@/contexts/auth-context';

const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');
import logoUrl from '/logo.png';

type PasswordMode = 'signin' | 'signup' | 'verify' | 'recovery-request' | 'recovery-reset' | 'mfa';

type AuthPayload = {
  error?: string;
  status?: string;
};

async function postAuth(path: string, body: Record<string, string>): Promise<AuthPayload> {
  const response = await fetch(`${basePath}${path}`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  const payload = (await response.json().catch(() => null)) as AuthPayload | null;
  if (!response.ok) {
    throw new Error(payload?.error || 'Unable to complete that request right now.');
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
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const providerIntent = mode === 'signup' ? 'signup' : 'signin';

  const changeMode = (nextMode: PasswordMode) => {
    setMode(nextMode);
    setError(null);
    setNotice(null);
    if (nextMode === 'signin' || nextMode === 'signup') {
      setCode('');
      setNewPassword('');
      setConfirmPassword('');
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
        setNotice('If an account can be created for this address, a verification code is on its way.');
      } else if (mode === 'verify') {
        await postAuth('/api/auth/email/verify', { email, code });
        setMode('signin');
        setPassword('');
        setCode('');
        setNotice('Your email is verified. You can sign in now.');
      } else if (mode === 'recovery-request') {
        await postAuth('/api/auth/password/recovery/request', { email });
        setMode('recovery-reset');
        setNotice('If a verified recovery address matches, a reset code is on its way.');
      } else if (mode === 'mfa') {
        await postAuth('/api/auth/mfa/verify', { code });
        window.location.assign(`${basePath}/dashboard`);
      } else {
        if (newPassword !== confirmPassword) {
          throw new Error('The passwords do not match.');
        }
        await postAuth('/api/auth/password/recovery/reset', { email, code, password: newPassword });
        setMode('signin');
        setPassword('');
        setNewPassword('');
        setConfirmPassword('');
        setCode('');
        setNotice('Your password was reset. Sign in with the new password.');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to complete that request right now.');
    } finally {
      setSubmitting(false);
    }
  };

  const resendVerification = async () => {
    if (submitting) return;
    setError(null);
    setNotice(null);
    setSubmitting(true);
    try {
      await postAuth('/api/auth/email/resend', { email });
      setNotice('If this account is waiting for verification, a new code is on its way.');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to resend the verification code.');
    } finally {
      setSubmitting(false);
    }
  };

  const isPasswordMode = mode === 'signin' || mode === 'signup';
  const heading = {
    signin: 'Sign in with email',
    signup: 'Create your account',
    verify: 'Verify your email',
    'recovery-request': 'Recover your account',
    'recovery-reset': 'Choose a new password',
    mfa: 'Verify your identity',
  }[mode];

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
                    ? 'Use the independently verified recovery email on your account.'
                    : mode === 'mfa'
                      ? 'Enter the six-digit code from your authenticator app, or use a recovery code.'
                    : 'Enter the code from your recovery email and choose a strong password.'}
              </p>
              {mode !== 'mfa' && (
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
              {mode !== 'recovery-request' && (
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
              {mode === 'recovery-reset' && (
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
                      : mode === 'mfa'
                        ? 'Verify MFA'
                        : 'Reset password'}
              </Button>
            </form>
            {mode === 'verify' && (
              <Button type="button" variant="link" onClick={resendVerification} disabled={submitting}>
                Resend verification code
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