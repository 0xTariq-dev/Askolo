import { Suspense, useEffect, useState, type ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { getQueryRetryDelay, shouldRetryQuery } from '@workspace/api-client-react';
import {
  DEFAULT_THEME_PREFERENCES,
  ThemeProvider,
  isSurfaceStyleId,
  isThemeId,
  isThemeMode,
  type ResolvedThemePreferences,
  type ThemeId,
} from '@workspace/askolo-design-system/theme';
import { Route, Switch, Router as WouterRouter, Redirect } from 'wouter';
import { Loader2 } from 'lucide-react';
import { Toaster } from '@workspace/askolo-design-system/components/ui/toaster';

import {
  isAppProductionHost,
  isPublicProductionHost,
  toAppUrl,
  toPublicUrl,
} from '@/lib/site-domains';
import { NativeAuthProvider, useAppAuth } from '@/contexts/auth-context';
import {
  ActionsPage,
  AdminCreditsPage,
  AppLayout,
  CalendarPage,
  ChoresPage,
  CreditsPage,
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
const THEME_STORAGE_KEY = 'askolo-theme-preferences';
const THEME_QUERY_PARAMETER = 'theme';

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function loadThemePreferences(): ResolvedThemePreferences {
  const defaults = DEFAULT_THEME_PREFERENCES;
  let stored: Record<string, unknown> = {};
  let queryMode: string | null = null;

  if (typeof window !== 'undefined') {
    queryMode = new URLSearchParams(window.location.search).get(THEME_QUERY_PARAMETER);

    try {
      const serialized = window.localStorage.getItem(THEME_STORAGE_KEY);
      const parsed: unknown = serialized ? JSON.parse(serialized) : undefined;
      if (isRecord(parsed)) stored = parsed;
    } catch {
      console.warn('Askolo could not read saved theme preferences.');
    }
  }

  const rawSurfaceStyles = isRecord(stored.surfaceStyles) ? stored.surfaceStyles : {};
  const rawSpaceOverrides = isRecord(stored.spaceOverrides) ? stored.spaceOverrides : {};
  const spaceOverrides = Object.create(null) as Record<string, ThemeId>;
  for (const [spaceId, themeId] of Object.entries(rawSpaceOverrides)) {
    if (spaceId.trim() && spaceId.length <= 128 && isThemeId(themeId)) {
      spaceOverrides[spaceId] = themeId;
    }
  }

  return {
    accountThemeId: isThemeId(stored.accountThemeId)
      ? stored.accountThemeId
      : defaults.accountThemeId,
    mode: isThemeMode(queryMode)
      ? queryMode
      : isThemeMode(stored.mode)
        ? stored.mode
        : defaults.mode,
    surfaceStyles: {
      light: isSurfaceStyleId(rawSurfaceStyles.light)
        ? rawSurfaceStyles.light
        : defaults.surfaceStyles.light,
      dark: isSurfaceStyleId(rawSurfaceStyles.dark)
        ? rawSurfaceStyles.dark
        : defaults.surfaceStyles.dark,
    },
    spaceOverrides,
  };
}

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
        <Route path="/credits" component={CreditsPage} />
        <Route path="/admin/credits" component={AdminCreditsPage} />
      </Switch>
    </AppLayout>
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
      <WouterRouter base={basePath}>
        {isPublicProductionHost() ? (
          <PublicSiteRoutes />
        ) : (
          <NativeAuthWithRoutes />
        )}
      </WouterRouter>
    </PersistentThemeProvider>
  );
}

export default App;