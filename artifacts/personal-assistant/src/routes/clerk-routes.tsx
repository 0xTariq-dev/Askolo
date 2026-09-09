import { Suspense, useEffect, useRef } from 'react';
import {
  AuthenticateWithRedirectCallback,
  ClerkProvider,
  SignIn,
  SignUp,
  useAuth,
  useClerk,
} from '@clerk/react';
import { publishableKeyFromHost } from '@clerk/react/internal';
import { shadcn } from '@clerk/themes';
import { QueryClient, QueryClientProvider, useQueryClient } from '@tanstack/react-query';
import { Route, Switch, Redirect, useLocation } from 'wouter';
import { Loader2 } from 'lucide-react';

import { Toaster } from '@/components/ui/toaster';
import { TooltipProvider } from '@/components/ui/tooltip';
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