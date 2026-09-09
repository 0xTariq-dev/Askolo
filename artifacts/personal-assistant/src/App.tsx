import { lazy, Suspense, useEffect } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Toaster } from '@/components/ui/toaster';
import { TooltipProvider } from '@/components/ui/tooltip';
import { Route, Switch, Router as WouterRouter, Redirect } from 'wouter';
import { Loader2 } from 'lucide-react';

import {
  isAppProductionHost,
  isPublicProductionHost,
  toAppUrl,
  toPublicUrl,
} from '@/lib/site-domains';
import { NativeAuthProvider, useAppAuth } from '@/contexts/auth-context';
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
  LoginPage,
  NotesPage,
  PlanPage,
  PrivacyPage,
  ProfilePage,
  TermsPage,
} from '@/routes/lazy-pages';

const ClerkRoutes = lazy(() => import('@/routes/clerk-routes'));
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      gcTime: 10 * 60_000,
      refetchOnWindowFocus: false,
      refetchOnReconnect: true,
      retry: 1,
    },
  },
});
const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');

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

function NativeProtectedRoutes() {
  const { isLoaded, isSignedIn } = useAppAuth();

  if (!isLoaded) return <RouteLoadingState />;
  if (!isSignedIn) return <LoginPage />;

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

function NativeAuthWithRoutes() {
  return (
    <NativeAuthProvider>
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <Suspense fallback={<RouteLoadingState />}>
            <Switch>
              <Route path="/" component={() => <ExternalRedirect href={toPublicUrl('/')} />} />
              <Route path="/privacy" component={() => <ExternalRedirect href={toPublicUrl('/privacy')} />} />
              <Route path="/terms" component={() => <ExternalRedirect href={toPublicUrl('/terms')} />} />
              <Route component={NativeProtectedRoutes} />
            </Switch>
          </Suspense>
          <Toaster />
        </TooltipProvider>
      </QueryClientProvider>
    </NativeAuthProvider>
  );
}

function App() {
  return (
    <WouterRouter base={basePath}>
      {isPublicProductionHost() ? (
        <PublicSiteRoutes />
      ) : isAppProductionHost() ? (
        <NativeAuthWithRoutes />
      ) : (
        <Suspense fallback={<RouteLoadingState />}>
          <ClerkRoutes queryClient={queryClient} />
        </Suspense>
      )}
    </WouterRouter>
  );
}

export default App;