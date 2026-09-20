import { Button } from '@/components/ui/button';
import { motion } from 'framer-motion';
import { Github, KeyRound, Sparkles, UserPlus } from 'lucide-react';
import { useState } from 'react';
import { Input } from '@/components/ui/input';

const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');
import logoUrl from '/logo.png';

export function LoginPage() {
  const beginProviderLogin = (provider: 'google' | 'github', intent: 'signin' | 'signup' = 'signin') => {
    const returnTo = `${window.location.pathname}${window.location.search}`;
    window.location.assign(
      `${basePath}/api/auth/${provider}?intent=${intent}&returnTo=${encodeURIComponent(returnTo || '/dashboard')}`,
    );
  };
  const [showPassword, setShowPassword] = useState(false);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const signInWithPassword = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (submitting) return;
    setError(null);
    setSubmitting(true);
    try {
      const response = await fetch(`${basePath}/api/auth/password/login`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password }),
      });
      if (!response.ok) {
        const payload = (await response.json().catch(() => null)) as { error?: string } | null;
        throw new Error(payload?.error || 'Email or password is incorrect.');
      }
      window.location.assign(`${basePath}/dashboard`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to sign in right now.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="min-h-screen w-full flex bg-background relative overflow-hidden flex-col items-center justify-center p-4">
      {/* Ambient background glows */}
      <div className="absolute top-[0%] left-[-10%] w-[60%] h-[60%] bg-primary/10 rounded-full blur-[120px] pointer-events-none" />
      <div className="absolute bottom-[-10%] right-[-10%] w-[50%] h-[50%] bg-blue-600/10 rounded-full blur-[120px] pointer-events-none" />

      {/* Noise overlay */}
      <div className="absolute inset-0 opacity-[0.03] pointer-events-none bg-[url('https://grainy-gradients.vercel.app/noise.svg')]" />

      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.8, ease: 'easeOut' }}
        className="z-10 flex flex-col items-center max-w-md text-center"
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

        <div className="flex w-full max-w-xs flex-col gap-3">
          <Button
            size="lg"
            onClick={() => beginProviderLogin('google')}
            className="w-full rounded-full px-8 py-6 text-lg font-medium shadow-[0_0_40px_-10px_rgba(234,179,8,0.3)]"
            data-testid="button-google-login"
          >
            <Sparkles className="mr-2 h-5 w-5 text-primary-foreground/70" />
            Continue with Google
          </Button>
          <Button
            size="lg"
            variant="outline"
            onClick={() => beginProviderLogin('github')}
            className="w-full rounded-full px-8 py-6 text-lg font-medium border-white/10 hover:bg-white/5"
            data-testid="button-github-login"
          >
            <Github className="mr-2 h-5 w-5" />
            Continue with GitHub
          </Button>
          <Button
            type="button"
            variant="ghost"
            onClick={() => setShowPassword((value) => !value)}
            className="w-full"
          >
            <KeyRound className="mr-2 h-4 w-4" />
            {showPassword ? 'Use a provider instead' : 'Sign in with email and password'}
          </Button>
          {showPassword && (
            <form onSubmit={signInWithPassword} className="space-y-3 rounded-xl border border-border bg-card p-4 text-left">
              <Input
                type="email"
                autoComplete="email"
                placeholder="you@example.com"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                required
              />
              <Input
                type="password"
                autoComplete="current-password"
                placeholder="Password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                required
              />
              {error && <p className="text-sm text-destructive" role="alert">{error}</p>}
              <Button type="submit" className="w-full" disabled={submitting}>
                {submitting ? 'Signing in…' : 'Sign in'}
              </Button>
            </form>
          )}
          <Button
            type="button"
            variant="link"
            onClick={() => beginProviderLogin('google', 'signup')}
            className="w-full"
          >
            <UserPlus className="mr-2 h-4 w-4" />
            Create an account with Google
          </Button>
        </div>
      </motion.div>
    </div>
  );
}
