import { Suspense, useEffect, useRef, useState, type FormEvent } from 'react';
import {
  AuthenticateWithRedirectCallback,
  ClerkProvider,
  SignIn,
  SignUp,
  useAuth,
  useClerk,
  useSignIn,
} from '@clerk/react';
import { publishableKeyFromHost } from '@clerk/react/internal';
import { shadcn } from '@clerk/themes';
import { QueryClient, QueryClientProvider, useQueryClient } from '@tanstack/react-query';
import { Route, Switch, Redirect, useLocation } from 'wouter';
import { Loader2 } from 'lucide-react';

import { Toaster } from '@/components/ui/toaster';
import { TooltipProvider } from '@/components/ui/tooltip';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { ClerkAuthBridge } from '@/contexts/clerk-auth-context';
import { isAppProductionHost } from '@/lib/site-domains';
import '@clerk/themes/shadcn.css';
import {
  ActionsPage,
  AppLayout,
  CalendarPage,
  ChoresPage,
  DashboardPage,
  EmailPage,
  GoalsPage,
  HabitsPage,
  LandingPage,
  NotesPage,
  PlanPage,
  PrivacyPage,
  ProfilePage,
  TermsPage,
} from '@/routes/lazy-pages';

const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');
const clerkPubKey = publishableKeyFromHost(
  window.location.hostname,
  import.meta.env.VITE_CLERK_PUBLISHABLE_KEY,
);
const clerkProxyUrl = import.meta.env.VITE_CLERK_PROXY_URL;

function stripBase(path: string): string {
  return basePath && path.startsWith(basePath)
    ? path.slice(basePath.length) || '/'
    : path;
}

function RouteLoadingState() {
  return (
    <div
      className="min-h-screen w-full flex items-center justify-center bg-background"
      aria-label="Loading page"
    >
      <Loader2 className="h-8 w-8 animate-spin text-primary" aria-hidden="true" />
    </div>
  );
}

function LocalPasswordSignIn({ onBack }: { onBack: () => void }) {
  const { signIn, fetchStatus } = useSignIn();
  const [identifier, setIdentifier] = useState('');
  const [password, setPassword] = useState('');
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (isSubmitting) return;

    setErrorMessage(null);
    setIsSubmitting(true);

    try {
      const result = await signIn.password({ identifier, password });
      if (result.error) {
        setErrorMessage(
          result.error.longMessage ||
            result.error.message ||
            'Unable to sign in with that email and password.',
        );
        return;
      }

      if (signIn.status === 'complete') {
        const finalizeResult = await signIn.finalize();
        if (finalizeResult.error) {
          setErrorMessage(
            finalizeResult.error.longMessage ||
              finalizeResult.error.message ||
              'Unable to activate the signed-in session.',
          );
          return;
        }
        window.location.assign(`${basePath}/dashboard`);
        return;
      }

      setErrorMessage(
        'This account needs an additional verification step. Use the standard sign-in options instead.',
      );
    } catch {
      setErrorMessage('Unable to sign in right now. Please try again.');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="mx-auto w-full max-w-md rounded-xl border border-border bg-card p-6 shadow-xl">
      <div className="mb-6">
        <h2 className="text-xl font-semibold text-foreground">
          Sign in with email and password
        </h2>
        <p className="mt-2 text-sm text-muted-foreground">
          Use your local Clerk development account to test the app.
        </p>
      </div>

      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="space-y-2">
          <label htmlFor="local-sign-in-identifier" className="text-sm font-medium">
            Email or username
          </label>
          <Input
            id="local-sign-in-identifier"
            name="identifier"
            type="text"
            autoComplete="username"
            value={identifier}
            onChange={(event) => setIdentifier(event.target.value)}
            placeholder="you@example.com"
            required
          />
        </div>

        <div className="space-y-2">
          <label htmlFor="local-sign-in-password" className="text-sm font-medium">
            Password
          </label>
          <Input
            id="local-sign-in-password"
            name="password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            placeholder="Enter your password"
            required
          />
        </div>

        {errorMessage && (
          <p role="alert" className="text-sm text-destructive">
            {errorMessage}
          </p>
        )}

        <Button
          type="submit"
          className="w-full"
          disabled={fetchStatus === 'fetching' || isSubmitting || !identifier || !password}
        >
          {isSubmitting ? 'Signing in…' : 'Sign in'}
        </Button>
      </form>

      <Button type="button" variant="link" className="mt-4 w-full" onClick={onBack}>
        Use Google or other sign-in options
      </Button>
    </div>
  );
}

function SignInPage() {
  const [showPasswordForm, setShowPasswordForm] = useState(false);

  return (
    <div className="min-h-screen w-full flex items-center justify-center bg-background px-4 relative overflow-hidden">
      <div className="absolute top-[0%] left-[-10%] w-[60%] h-[60%] bg-primary/10 rounded-full blur-[120px] pointer-events-none" />
      <div className="absolute bottom-[-10%] right-[-10%] w-[50%] h-[50%] bg-blue-600/10 rounded-full blur-[120px] pointer-events-none" />
      <div className="relative z-10 w-full">
        {showPasswordForm ? (
          <LocalPasswordSignIn onBack={() => setShowPasswordForm(false)} />
        ) : (
          <>
            <SignIn
              routing="path"
              path={`${basePath}/sign-in`}
              signUpUrl={`${basePath}/sign-up`}
              fallbackRedirectUrl={`${basePath}/dashboard`}
            />
            <Button
              type="button"
              variant="link"
              className="mx-auto mt-4 block text-sm"
              onClick={() => setShowPasswordForm(true)}
              data-testid="button-password-login"
            >
              Use email and password
            </Button>
          </>
        )}
      </div>
    </div>
  );
}

function SignUpPage() {
  return (
    <div className="min-h-screen w-full flex items-center justify-center bg-background px-4 relative overflow-hidden">
      <div className="absolute top-[0%] left-[-10%] w-[60%] h-[60%] bg-primary/10 rounded-full blur-[120px] pointer-events-none" />
      <div className="absolute bottom-[-10%] right-[-10%] w-[50%] h-[50%] bg-blue-600/10 rounded-full blur-[120px] pointer-events-none" />
      <div className="relative z-10 w-full">
        <SignUp
          routing="path"
          path={`${basePath}/sign-up`}
          signInUrl={`${basePath}/sign-in`}
          fallbackRedirectUrl={`${basePath}/dashboard`}
        />
      </div>
    </div>
  );
}

function SsoCallbackPage() {
  return (
    <div className="min-h-screen w-full flex items-center justify-center bg-background px-4">
      <AuthenticateWithRedirectCallback
        signInUrl={`${basePath}/sign-in`}
        signUpUrl={`${basePath}/sign-up`}
        signInFallbackRedirectUrl={`${basePath}/dashboard`}
        signUpFallbackRedirectUrl={`${basePath}/dashboard`}
      />
    </div>
  );
}

function ClerkQueryClientCacheInvalidator() {
  const { addListener } = useClerk();
  const queryClient = useQueryClient();
  const prevUserIdRef = useRef<string | null | undefined>(undefined);

  useEffect(() => {
    const unsubscribe = addListener(({ user }) => {
      const userId = user?.id ?? null;
      if (
        prevUserIdRef.current !== undefined &&
        prevUserIdRef.current !== userId
      ) {
        queryClient.clear();
      }
      prevUserIdRef.current = userId;
    });
    return unsubscribe;
  }, [addListener, queryClient]);

  return null;
}

function ProtectedRoutes() {
  const { isLoaded, isSignedIn } = useAuth();

  if (!isLoaded) return <RouteLoadingState />;
  if (!isSignedIn) return <SignInPage />;

  return (
    <AppLayout>
      <Switch>
        <Route path="/dashboard" component={DashboardPage} />
        <Route path="/habits" component={HabitsPage} />
        <Route path="/goals" component={GoalsPage} />
        <Route path="/plan" component={PlanPage} />
        <Route path="/calendar" component={CalendarPage} />
        <Route path="/chores" component={ChoresPage} />
        <Route path="/notes" component={NotesPage} />
        <Route path="/actions" component={ActionsPage} />
        <Route path="/assistant"><Redirect to="/dashboard" /></Route>
        <Route path="/email" component={EmailPage} />
        <Route path="/profile" component={ProfilePage} />
      </Switch>
    </AppLayout>
  );
}

export default function ClerkRoutes({ queryClient }: { queryClient: QueryClient }) {
  const [, setLocation] = useLocation();
  const splitAppHost = isAppProductionHost();

  return (
    <ClerkProvider
      publishableKey={clerkPubKey}
      proxyUrl={clerkProxyUrl}
      appearance={{
        theme: shadcn,
        cssLayerName: 'clerk',
        options: {
          logoPlacement: 'inside',
          logoLinkUrl: basePath || '/',
          logoImageUrl: `${window.location.origin}${basePath}/logo.png`,
          socialButtonsPlacement: 'top',
          socialButtonsVariant: 'blockButton',
        },
        variables: {
          colorPrimary: 'hsl(38, 92%, 50%)',
          colorForeground: 'hsl(210, 40%, 98%)',
          colorMutedForeground: 'hsl(215, 16%, 65%)',
          colorDanger: 'hsl(0, 84%, 60%)',
          colorBackground: 'hsl(221, 38%, 10%)',
          colorInput: 'hsl(215, 28%, 17%)',
          colorInputForeground: 'hsl(210, 40%, 98%)',
          colorNeutral: 'hsl(215, 28%, 17%)',
          fontFamily: "'Outfit', sans-serif",
          borderRadius: '0.75rem',
        },
      }}
      signInUrl={`${basePath}/sign-in`}
      signUpUrl={`${basePath}/sign-up`}
      routerPush={(to) => setLocation(stripBase(to))}
      routerReplace={(to) => setLocation(stripBase(to), { replace: true })}
    >
      <QueryClientProvider client={queryClient}>
        <ClerkQueryClientCacheInvalidator />
        <ClerkAuthBridge>
          <TooltipProvider>
            <SuspendedRoutes splitAppHost={splitAppHost} />
            <Toaster />
          </TooltipProvider>
        </ClerkAuthBridge>
      </QueryClientProvider>
    </ClerkProvider>
  );
}

function SuspendedRoutes({ splitAppHost }: { splitAppHost: boolean }) {
  return (
    <Suspense fallback={<RouteLoadingState />}>
      <Switch>
        {splitAppHost ? (
          <>
            <Route path="/" component={() => <Redirect to="/dashboard" />} />
            <Route path="/privacy" component={() => <Redirect to="/dashboard" />} />
            <Route path="/terms" component={() => <Redirect to="/dashboard" />} />
          </>
        ) : (
          <>
            <Route path="/" component={LandingPage} />
            <Route path="/privacy" component={PrivacyPage} />
            <Route path="/terms" component={TermsPage} />
          </>
        )}
        <Route path="/sign-in/*?" component={SignInPage} />
        <Route path="/sign-up/*?" component={SignUpPage} />
        <Route path="/sso-callback" component={SsoCallbackPage} />
        <Route component={ProtectedRoutes} />
      </Switch>
    </Suspense>
  );
}