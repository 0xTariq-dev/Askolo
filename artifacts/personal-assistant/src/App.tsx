import { useEffect, useRef } from 'react';
import { AuthenticateWithRedirectCallback, ClerkProvider, SignIn, SignUp, useAuth, useClerk } from '@clerk/react';
import { publishableKeyFromHost } from '@clerk/react/internal';
import { shadcn } from '@clerk/themes';
import { QueryClient, QueryClientProvider, useQueryClient } from '@tanstack/react-query';
import { Toaster } from '@/components/ui/toaster';
import { TooltipProvider } from '@/components/ui/tooltip';
import { Route, Switch, Router as WouterRouter, useLocation, Redirect } from 'wouter';
import { AppLayout } from '@/components/layout/app-layout';
import { Loader2 } from 'lucide-react';

import { DashboardPage } from '@/pages/dashboard';
import { HabitsPage } from '@/pages/habits';
import { GoalsPage } from '@/pages/goals';
import { PlanPage } from '@/pages/plan';
import { CalendarPage } from '@/pages/calendar';
import { ChoresPage } from '@/pages/chores';
import { NotesPage } from '@/pages/notes';
import { ActionsPage } from '@/pages/actions';
import { AssistantPage } from '@/pages/assistant';
import { EmailPage } from '@/pages/email';
import { PrivacyPage } from '@/pages/privacy';
import { TermsPage } from '@/pages/terms';
import { LoginPage } from '@/pages/login';
import { LandingPage } from '@/pages/landing';

const queryClient = new QueryClient();

// REQUIRED — copy verbatim. Resolves the key from window.location.hostname so the
// same build serves multiple Clerk custom domains.
const clerkPubKey = publishableKeyFromHost(
  window.location.hostname,
  import.meta.env.VITE_CLERK_PUBLISHABLE_KEY,
);

// REQUIRED — copy verbatim. Empty in dev (Clerk hits dev FAPI directly), auto-set
// in prod. Do NOT gate on import.meta.env.PROD / NODE_ENV.
const clerkProxyUrl = import.meta.env.VITE_CLERK_PROXY_URL;

const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');

// Clerk passes full paths to routerPush/routerReplace, but wouter's
// setLocation prepends the base — strip it to avoid doubling.
function stripBase(path: string): string {
  return basePath && path.startsWith(basePath)
    ? path.slice(basePath.length) || '/'
    : path;
}

if (!clerkPubKey) {
  throw new Error('Missing VITE_CLERK_PUBLISHABLE_KEY');
}

const clerkAppearance = {
  theme: shadcn,
  cssLayerName: 'clerk',
  options: {
    logoPlacement: 'inside' as const,
    logoLinkUrl: basePath || '/',
    logoImageUrl: `${window.location.origin}${basePath}/logo.png`,
    socialButtonsPlacement: 'top' as const,
    socialButtonsVariant: 'blockButton' as const,
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
  elements: {
    rootBox: 'w-full flex justify-center',
    cardBox: 'rounded-2xl w-[440px] max-w-full overflow-hidden shadow-2xl shadow-black/50',
    card: '!shadow-none !border-0 !rounded-none',
    footer: '!shadow-none !border-0 !rounded-none',
    headerTitle: 'text-foreground font-display font-bold',
    headerSubtitle: 'text-muted-foreground',
    socialButtonsBlockButtonText: 'text-foreground font-medium',
    formFieldLabel: 'text-foreground',
    footerActionLink: 'text-primary hover:text-primary/80',
    footerActionText: 'text-muted-foreground',
    dividerText: 'text-muted-foreground',
    identityPreviewEditButton: 'text-primary',
    formFieldSuccessText: 'text-green-400',
    alertText: 'text-foreground',
    logoBox: 'mb-2',
    logoImage: 'h-10 w-10',
    socialButtonsBlockButton: 'border-border hover:bg-white/5 transition-colors',
    formButtonPrimary:
      'bg-primary text-primary-foreground hover:bg-primary/90 transition-colors font-medium',
    formFieldInput: 'bg-input border-border text-foreground',
    footerAction: 'bg-transparent',
    dividerLine: 'bg-border',
    alert: 'bg-card border-border',
    otpCodeFieldInput: 'bg-input border-border text-foreground',
    formFieldRow: '',
    main: '',
  },
};

function SignInPage() {
  return (
    <div className="min-h-screen w-full flex items-center justify-center bg-background px-4 relative overflow-hidden">
      <div className="absolute top-[0%] left-[-10%] w-[60%] h-[60%] bg-primary/10 rounded-full blur-[120px] pointer-events-none" />
      <div className="absolute bottom-[-10%] right-[-10%] w-[50%] h-[50%] bg-blue-600/10 rounded-full blur-[120px] pointer-events-none" />
      <div className="relative z-10 w-full">
        <SignIn
          routing="path"
          path={`${basePath}/sign-in`}
          signUpUrl={`${basePath}/sign-up`}
          fallbackRedirectUrl={`${basePath}/dashboard`}
        />
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

// Invalidate QueryClient cache when the signed-in user changes.
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

  if (!isLoaded) {
    return (
      <div className="min-h-screen w-full flex items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    );
  }

  if (!isSignedIn) {
    return <LoginPage />;
  }

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
        <Route path="/assistant" component={AssistantPage} />
        <Route path="/email" component={EmailPage} />
      </Switch>
    </AppLayout>
  );
}

function ClerkProviderWithRoutes() {
  const [, setLocation] = useLocation();

  return (
    <ClerkProvider
      publishableKey={clerkPubKey}
      proxyUrl={clerkProxyUrl}
      appearance={clerkAppearance}
      signInUrl={`${basePath}/sign-in`}
      signUpUrl={`${basePath}/sign-up`}
      localization={{
        signIn: {
          start: {
            title: 'Welcome back to Askolo',
            subtitle: 'Sign in to your mission control',
          },
        },
        signUp: {
          start: {
            title: 'Join Askolo',
            subtitle: 'Your mission control for habits, goals, and focus',
          },
        },
      }}
      routerPush={(to) => setLocation(stripBase(to))}
      routerReplace={(to) => setLocation(stripBase(to), { replace: true })}
    >
      <QueryClientProvider client={queryClient}>
        <ClerkQueryClientCacheInvalidator />
        <TooltipProvider>
          <Switch>
            {/* Public pages */}
            <Route path="/" component={LandingPage} />
            <Route path="/privacy" component={PrivacyPage} />
            <Route path="/terms" component={TermsPage} />
            {/* REQUIRED — /sign-in/*? and /sign-up/*? must match exactly.
                The /*? optional wildcard is the only wouter syntax that
                handles Clerk's OAuth sub-paths. */}
            <Route path="/sign-in/*?" component={SignInPage} />
            <Route path="/sign-up/*?" component={SignUpPage} />
            {/* Clerk OAuth callback route */}
            <Route path="/sso-callback" component={SsoCallbackPage} />
            {/* Protected app routes */}
            <Route component={ProtectedRoutes} />
          </Switch>
          <Toaster />
        </TooltipProvider>
      </QueryClientProvider>
    </ClerkProvider>
  );
}

function App() {
  return (
    <WouterRouter base={basePath}>
      <ClerkProviderWithRoutes />
    </WouterRouter>
  );
}

export default App;
