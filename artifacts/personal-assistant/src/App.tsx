import { Suspense, useCallback, useEffect, useState, type ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { getQueryRetryDelay, shouldRetryQuery } from '@workspace/api-client-react';
import {
  ThemeProvider,
  isThemeMode,
  type ResolvedThemePreferences,
} from '@/lib/theme';
import { Route, Switch, Router as WouterRouter, Redirect, useLocation } from 'wouter';
import { Loader2 } from 'lucide-react';
import { Toaster } from '@/components/ui/toaster';

import {
  isAppProductionHost,
  isPublicProductionHost,
  toAppUrl,
  toPublicUrl,
} from '@/lib/site-domains';
import {
  loadThemePreferences,
  THEME_QUERY_PARAMETER,
  THEME_STORAGE_KEY,
} from '@/lib/theme-preferences';
import { NativeAuthProvider, useAppAuth } from '@/contexts/auth-context';
import { LocaleProvider } from '@/contexts/locale-context';
import { SpaceProvider } from '@/contexts/space-context';
import type { Locale } from '@/lib/locale';
import {
  AgentPage,
  AdminCreditsPage,
  CalendarPage,
  CreditsPage,
  DashboardPage,
  EmailPage,
  GoalsPage,
  InviteAcceptancePage,
  LandingPage,
  LoginPage,
  NotesPage,
  PlanPage,
  PrivacyPage,
  ProfilePage,
  SpaceDetailsPage,
  SpaceSlugRoute,
  SpaceNotificationsPage,
  SpaceMembersPage,
  SidebarAppLayout,
  TermsPage,
} from '@/routes/lazy-pages';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      gcTime: 10 * 60_000,
      refetchOnWindowFocus: false,
      refetchOnReconnect: true,
      retry: shouldRetryQuery,
      retryDelay: getQueryRetryDelay,
    },
  },
});
const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');

function PersistentThemeProvider({ children }: { children: ReactNode }) {
  const [preferences, setPreferences] = useState<ResolvedThemePreferences>(
    loadThemePreferences,
  );

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (!isThemeMode(params.get(THEME_QUERY_PARAMETER))) return;

    params.delete(THEME_QUERY_PARAMETER);
    const search = params.toString();
    window.history.replaceState(
      window.history.state,
      '',
      `${window.location.pathname}${search ? `?${search}` : ''}${window.location.hash}`,
    );
  }, []);

  useEffect(() => {
    try {
      window.localStorage.setItem(THEME_STORAGE_KEY, JSON.stringify(preferences));
    } catch {
      console.warn('Askolo could not save theme preferences.');
    }
  }, [preferences]);

  return <ThemeProvider value={preferences} onChange={setPreferences}>{children}</ThemeProvider>;
}

function RouteLoadingState() {
  return (
    <div
      className="min-h-screen w-full flex items-center justify-center bg-background"
      role="status"
    >
      <Loader2 className="h-8 w-8 animate-spin text-primary" aria-hidden="true" />
      <span className="sr-only">Loading page</span>
    </div>
  );
}

function RouteContentLoadingState() {
  return (
    <div className="flex min-h-64 w-full items-center justify-center" role="status">
      <Loader2 className="h-7 w-7 animate-spin text-primary" aria-hidden="true" />
      <span className="sr-only">Loading page</span>
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
  const [location] = useLocation();
  const currentPath = location || '/';

  if (window.location.hostname.toLowerCase() === 'www.askolo.app') {
    return <ExternalRedirect href={toPublicUrl(currentPathWithQuery())} />;
  }

  if (!['/', '/privacy', '/terms'].includes(currentPath)) {
    if (isAppProductionHost()) {
      return <NativeAuthWithRoutes />;
    }
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

function AuthenticatedPageRoutes() {
  return (
    <Switch>
      <Route path="/sign-in"><Redirect to="/home" /></Route>
      <Route path="/sign-up"><Redirect to="/home" /></Route>
      <Route path="/home" component={DashboardPage} />
      <Route path="/dashboard"><Redirect to="/home" /></Route>
      <Route path="/habits"><Redirect to="/agenda?tab=tasks&kind=habit" /></Route>
      <Route path="/goals" component={GoalsPage} />
      <Route path="/agenda" component={PlanPage} />
      <Route path="/plan"><Redirect to="/agenda" /></Route>
      <Route path="/calendar" component={CalendarPage} />
      <Route path="/notes" component={NotesPage} />
       <Route path="/agent" component={AgentPage} />
       <Route path="/assistant"><Redirect to="/agent" /></Route>
      <Route path="/email" component={EmailPage} />
      <Route path="/profile"><Redirect to="/settings/account/profile" /></Route>
      <Route path="/settings/account/profile" component={ProfilePage} />
      <Route path="/settings/account/security" component={ProfilePage} />
      <Route path="/settings/account/privacy" component={ProfilePage} />
       <Route path="/settings/space/details" component={SpaceDetailsPage} />
      <Route path="/settings/space/notifications" component={SpaceNotificationsPage} />
      <Route path="/settings/space/members" component={SpaceMembersPage} />
      <Route path="/invite/:token" component={InviteAcceptancePage} />
      <Route path="/s/:slug" component={SpaceSlugRoute} />
      <Route path="/credits"><Redirect to="/settings/space/usage" /></Route>
      <Route path="/settings/space/usage" component={CreditsPage} />
      <Route path="/settings/space/billing" component={CreditsPage} />
      <Route path="/admin/credits" component={AdminCreditsPage} />
    </Switch>
  );
}

function NativeProtectedRoutes() {
  const { isLoaded, isSignedIn } = useAppAuth();

  if (!isLoaded) return <RouteLoadingState />;
  if (!isSignedIn) return <LoginPage />;

  return (
    <AccountLocaleProvider>
      <SpaceProvider>
        <SidebarAppLayout>
          <Suspense fallback={<RouteContentLoadingState />}>
            <AuthenticatedPageRoutes />
          </Suspense>
        </SidebarAppLayout>
      </SpaceProvider>
    </AccountLocaleProvider>
  );
}

function AccountLocaleProvider({ children }: { children: ReactNode }) {
  const { user, updateProfile } = useAppAuth();
  const persistAccountLocale = useCallback(
    (locale: Locale) => updateProfile({ preferredLocale: locale }),
    [updateProfile],
  );
  return (
    <LocaleProvider
      initialLocale={user?.preferredLocale}
      localeScope={user?.id}
      persistAccountLocale={persistAccountLocale}
    >
      {children}
    </LocaleProvider>
  );
}

function NativeAuthWithRoutes() {
  return (
    <NativeAuthProvider>
      <QueryClientProvider client={queryClient}>
        <Suspense fallback={<RouteLoadingState />}>
          <Switch>
            <Route path="/" component={() => <ExternalRedirect href={toPublicUrl('/')} />} />
            <Route path="/privacy" component={() => <ExternalRedirect href={toPublicUrl('/privacy')} />} />
            <Route path="/terms" component={() => <ExternalRedirect href={toPublicUrl('/terms')} />} />
            <Route component={NativeProtectedRoutes} />
          </Switch>
        </Suspense>
        <Toaster />
      </QueryClientProvider>
    </NativeAuthProvider>
  );
}

function App() {
  return (
    <PersistentThemeProvider>
      <LocaleProvider>
        <WouterRouter base={basePath}>
          {isPublicProductionHost() ? (
            <PublicSiteRoutes />
          ) : (
            <NativeAuthWithRoutes />
          )}
        </WouterRouter>
      </LocaleProvider>
    </PersistentThemeProvider>
  );
}

export default App;