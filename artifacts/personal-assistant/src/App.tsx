import { lazy, Suspense, useEffect, useRef } from 'react';
import { AuthenticateWithRedirectCallback, ClerkProvider, SignIn, SignUp, useAuth, useClerk } from '@clerk/react';
import { publishableKeyFromHost } from '@clerk/react/internal';
import { shadcn } from '@clerk/themes';
import { QueryClient, QueryClientProvider, useQueryClient } from '@tanstack/react-query';
import { Toaster } from '@/components/ui/toaster';
import { TooltipProvider } from '@/components/ui/tooltip';
import { Route, Switch, Router as WouterRouter, useLocation, Redirect } from 'wouter';
import { Loader2 } from 'lucide-react';

import { LandingPage } from '@/pages/landing';
import {
  isAppProductionHost,
  isPublicProductionHost,
  toAppUrl,
  toPublicUrl,
} from '@/lib/site-domains';

const AppLayout = lazy(() =>
  import('@/components/layout/app-layout').then(({ AppLayout }) => ({ default: AppLayout })),
);
const DashboardPage = lazy(() =>
  import('@/pages/dashboard').then(({ DashboardPage }) => ({ default: DashboardPage })),
);
const HabitsPage = lazy(() =>
  import('@/pages/habits').then(({ HabitsPage }) => ({ default: HabitsPage })),
);
const GoalsPage = lazy(() =>
  import('@/pages/goals').then(({ GoalsPage }) => ({ default: GoalsPage })),
);
const PlanPage = lazy(() =>
  import('@/pages/plan').then(({ PlanPage }) => ({ default: PlanPage })),
);
const CalendarPage = lazy(() =>
  import('@/pages/calendar').then(({ CalendarPage }) => ({ default: CalendarPage })),
);
const ChoresPage = lazy(() =>
  import('@/pages/chores').then(({ ChoresPage }) => ({ default: ChoresPage })),
);
const NotesPage = lazy(() =>
  import('@/pages/notes').then(({ NotesPage }) => ({ default: NotesPage })),
);
const ActionsPage = lazy(() =>
  import('@/pages/actions').then(({ ActionsPage }) => ({ default: ActionsPage })),
);
const EmailPage = lazy(() =>
  import('@/pages/email').then(({ EmailPage }) => ({ default: EmailPage })),
);
const ProfilePage = lazy(() =>
  import('@/pages/profile').then(({ ProfilePage }) => ({ default: ProfilePage })),
);
const PrivacyPage = lazy(() =>
  import('@/pages/privacy').then(({ PrivacyPage }) => ({ default: PrivacyPage })),
);
const TermsPage = lazy(() =>
  import('@/pages/terms').then(({ TermsPage }) => ({ default: TermsPage })),
);
const LoginPage = lazy(() =>
  import('@/pages/login').then(({ LoginPage }) => ({ default: LoginPage })),
);

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
    logoLinkUrl: isAppProductionHost() ? toPublicUrl('/') : basePath || '/',
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

function RouteLoadingState() {
  return (
    <div className="min-h-screen w-full flex items-center justify-center bg-background" aria-label="Loading page">
      <Loader2 className="h-8 w-8 animate-spin text-primary" aria-hidden="true" />
    </div>
  );
}

function ExternalRedirect({ href }: { href: string }) {
  useEffect(() => {
    window.location.replace(href);
  }, [href]);

  return <RouteLoadingState />;
}

function currentPathWithQuery(): string {
  return `${window.location.pathname}${window.location.search}${window.location.hash}`;
}

function PublicSiteRoutes() {
  const currentPath = window.location.pathname || '/';

  if (window.location.hostname.toLowerCase() === 'www.askolo.app') {
    return <ExternalRedirect href={toPublicUrl(currentPathWithQuery())} />;
  }

  if (!['/', '/privacy', '/terms'].includes(currentPath)) {
    return <ExternalRedirect href={toAppUrl(currentPathWithQuery())} />;
  }

  return (
    <Suspense fallback={<RouteLoadingState />}>
      <Switch>
        <Route path="/" component={LandingPage} />
        <Route path="/privacy" component={PrivacyPage} />
        <Route path="/terms" component={TermsPage} />
      </Switch>
    </Suspense>
  );
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
        <Route path="/assistant"><Redirect to="/dashboard" /></Route>
        <Route path="/email" component={EmailPage} />
        <Route path="/profile" component={ProfilePage} />
      </Switch>
    </AppLayout>
  );
}

function ClerkProviderWithRoutes() {
  const [, setLocation] = useLocation();
  const splitAppHost = isAppProductionHost();

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
          <Suspense fallback={<RouteLoadingState />}>
            <Switch>
              {splitAppHost ? (
                <>
                  <Route path="/" component={() => <ExternalRedirect href={toPublicUrl('/')} />} />
                  <Route path="/privacy" component={() => <ExternalRedirect href={toPublicUrl('/privacy')} />} />
                  <Route path="/terms" component={() => <ExternalRedirect href={toPublicUrl('/terms')} />} />
                </>
              ) : (
                <>
                  {/* Public pages in the Replit preview/development host */}
                  <Route path="/" component={LandingPage} />
                  <Route path="/privacy" component={PrivacyPage} />
                  <Route path="/terms" component={TermsPage} />
                </>
              )}
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
          </Suspense>
          <Toaster />
        </TooltipProvider>
      </QueryClientProvider>
    </ClerkProvider>
  );
}

function App() {
  return (
    <WouterRouter base={basePath}>
      {isPublicProductionHost() ? <PublicSiteRoutes /> : <ClerkProviderWithRoutes />}
    </WouterRouter>
  );
}

export default App;
