import { ReactNode, useEffect, useRef, useState } from 'react';
import { Link, useLocation } from 'wouter';
import {
  LayoutDashboard,
  CheckCircle2,
  Target,
  ListTodo,
  Calendar,
  ClipboardList,
  StickyNote,
  Zap,
  Mail,
  LogOut,
  Menu,
  UserCircle,
  WalletCards,
} from 'lucide-react';
import { Avatar, AvatarFallback, AvatarImage } from '@workspace/askolo-design-system/components/ui/avatar';
import { cn } from '@workspace/askolo-design-system/lib/utils';
import { Button } from '@workspace/askolo-design-system/components/ui/button';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
} from '@workspace/askolo-design-system/components/ui/sheet';
import { AssistantProvider } from '@/contexts/assistant-context';
import { NotificationProvider } from '@/contexts/notification-context';
import { AssistantSidebar } from '@/components/assistant-sidebar';
import { NotificationBell } from '@/components/notification-bell';
import { useGoogleConnectionCheck } from '@/hooks/use-google-connection-check';
import { useIsMobile } from '@/hooks/use-mobile';
import logoUrl from '/logo.png';
import { isAppProductionHost, toPublicUrl } from '@/lib/site-domains';
import { useAppAuth } from '@/contexts/auth-context';
import { useLocale } from '@/contexts/locale-context';

const navItems = [
  { href: '/dashboard', labelKey: 'nav.dashboard', icon: LayoutDashboard },
  { href: '/habits', labelKey: 'nav.habits', icon: CheckCircle2 },
  { href: '/goals', labelKey: 'nav.goals', icon: Target },
  { href: '/plan', labelKey: 'nav.dailyPlan', icon: ListTodo },
  { href: '/calendar', labelKey: 'nav.calendar', icon: Calendar },
  { href: '/chores', labelKey: 'nav.chores', icon: ClipboardList },
  { href: '/notes', labelKey: 'nav.notes', icon: StickyNote },
  { href: '/actions', labelKey: 'nav.actions', icon: Zap },
  { href: '/email', labelKey: 'nav.email', icon: Mail },
  { href: '/credits', labelKey: 'nav.aiCredits', icon: WalletCards },
];

const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');
const MOBILE_SWIPE_THRESHOLD = 64;
const MOBILE_EDGE_GESTURE_WIDTH = 28;

type NavigationPanelProps = {
  location: string;
  displayName: string;
  email: string;
  avatarUrl?: string;
  initials: string;
  onNavigate?: () => void;
  onSignOut: () => void;
  showNotifications?: boolean;
};

function NavigationPanel({
  location,
  displayName,
  email,
  avatarUrl,
  initials,
  onNavigate,
  onSignOut,
  showNotifications = false,
}: NavigationPanelProps) {
  const { t } = useLocale();

  return (
    <div className="flex h-full min-h-0 flex-col bg-sidebar text-sidebar-foreground">
      <div className="flex h-16 shrink-0 items-center justify-between border-b border-border px-6">
        <Link
          href="/dashboard"
          onClick={onNavigate}
          className="flex items-center gap-2 group"
        >
          <img src={logoUrl} alt="Askolo" className="h-7 w-auto object-contain" />
          <span className="font-display text-xl font-bold tracking-tight text-sidebar-foreground">
            Askolo
          </span>
        </Link>
        {showNotifications && <NotificationBell />}
      </div>

      <nav
        aria-label={t('nav.main')}
        className="flex-1 space-y-1 overflow-y-auto px-3 py-6"
      >
        {navItems.map((item) => {
          const isActive =
            location === item.href ||
            (item.href !== '/' && location.startsWith(item.href));
          const Icon = item.icon;

          return (
            <Link
              key={item.href}
              href={item.href}
              onClick={onNavigate}
      aria-current={isActive ? 'page' : undefined}
              className={cn(
                'flex items-center gap-3 rounded-md px-3 py-3 text-sm font-medium transition-all duration-200',
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring',
                isActive
                  ? 'bg-sidebar-primary/10 text-sidebar-primary'
                   : 'text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-foreground',
              )}
              data-testid={`nav-${item.href.slice(1).replaceAll('/', '-')}`}
            >
              <Icon
                className={cn(
                  'h-4 w-4',
                  isActive
                    ? 'text-sidebar-primary'
                    : 'text-sidebar-foreground/70',
                )}
                aria-hidden="true"
              />
              {t(item.labelKey)}
            </Link>
          );
        })}
      </nav>

      <div className="shrink-0 border-t border-border bg-sidebar p-4">
        <Link
          href="/profile"
          onClick={onNavigate}
          className="group mb-4 flex items-center gap-3 rounded-md px-2 py-2 transition-colors hover:bg-sidebar-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring"
        >
          <Avatar className="h-9 w-9 border border-border">
            <AvatarImage src={avatarUrl} />
            <AvatarFallback className="bg-sidebar-primary/20 text-sidebar-primary">
              {initials}
            </AvatarFallback>
          </Avatar>
          <span className="flex min-w-0 flex-1 flex-col overflow-hidden">
            <span className="truncate text-sm font-medium text-sidebar-foreground transition-colors group-hover:text-sidebar-primary">
              <bdi dir="auto">{displayName}</bdi>
            </span>
            <span className="truncate text-xs text-sidebar-foreground/60">
              <bdi lang="en" dir="ltr">{email}</bdi>
            </span>
          </span>
          <UserCircle
            className="h-4 w-4 shrink-0 text-sidebar-foreground/40 transition-colors group-hover:text-sidebar-primary"
            aria-hidden="true"
          />
        </Link>
        <Button
          variant="outline"
           className="w-full justify-start border-border text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-foreground"
          onClick={onSignOut}
          data-testid="button-logout"
        >
          <LogOut className="me-2 h-4 w-4" aria-hidden="true" />
          {t('nav.signOut')}
        </Button>
      </div>
    </div>
  );
}

// Inner component so it can consume both providers
function AppLayoutInner({ children }: { children: ReactNode }) {
  const { user, signOut } = useAppAuth();
  const [location] = useLocation();
  const [isMobileNavOpen, setIsMobileNavOpen] = useState(false);
  const touchStartRef = useRef<{ x: number; y: number } | null>(null);
  const isMobile = useIsMobile();
  const { direction, formatDate, t } = useLocale();

  // Run once per session to check Google connection status
  useGoogleConnectionCheck();

  const displayName =
    [user?.firstName, user?.lastName].filter(Boolean).join(' ') || 'User';
  const email = user?.email || '';
  const avatarUrl = user?.imageUrl || user?.profileImageUrl || undefined;
  const initials = user?.firstName?.[0] || 'U';
  const closeMobileNav = () => setIsMobileNavOpen(false);
  const handleSignOut = () => {
    closeMobileNav();
    signOut({
      redirectUrl: isAppProductionHost() ? toPublicUrl('/') : basePath || '/',
    });
  };

  useEffect(() => {
    closeMobileNav();
  }, [location]);

  useEffect(() => {
    if (!isMobile) setIsMobileNavOpen(false);
  }, [isMobile]);

  useEffect(() => {
    if (!isMobileNavOpen) return;

    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') closeMobileNav();
    };

    window.addEventListener('keydown', handleEscape);
    return () => window.removeEventListener('keydown', handleEscape);
  }, [isMobileNavOpen]);

  const handleTouchStart = (event: React.TouchEvent) => {
    if (!isMobile) return;
    const touch = event.touches[0];
    if (!touch) return;

    const fromInlineStart =
      direction === 'rtl'
        ? touch.clientX >= window.innerWidth - MOBILE_EDGE_GESTURE_WIDTH
        : touch.clientX <= MOBILE_EDGE_GESTURE_WIDTH;
    if (isMobileNavOpen || fromInlineStart) {
      touchStartRef.current = { x: touch.clientX, y: touch.clientY };
    }
  };

  const handleTouchEnd = (event: React.TouchEvent) => {
    if (!isMobile) return;
    const start = touchStartRef.current;
    touchStartRef.current = null;
    const touch = event.changedTouches[0];
    if (!start || !touch) return;

    const deltaX = touch.clientX - start.x;
    const deltaY = touch.clientY - start.y;
    if (
      Math.abs(deltaX) < MOBILE_SWIPE_THRESHOLD ||
      Math.abs(deltaX) <= Math.abs(deltaY)
    ) {
      return;
    }

    const fromInlineStart =
      direction === 'rtl'
        ? start.x >= window.innerWidth - MOBILE_EDGE_GESTURE_WIDTH
        : start.x <= MOBILE_EDGE_GESTURE_WIDTH;
    const opensFromEdge = direction === 'rtl' ? deltaX < 0 : deltaX > 0;
    const closesTowardEdge = direction === 'rtl' ? deltaX > 0 : deltaX < 0;
    if (!isMobileNavOpen && fromInlineStart && opensFromEdge) {
      setIsMobileNavOpen(true);
    } else if (isMobileNavOpen && closesTowardEdge) {
      closeMobileNav();
    }
  };

  return (
    <div
      className="flex h-dvh min-h-dvh w-full overflow-hidden bg-background text-foreground"
      onTouchStart={handleTouchStart}
      onTouchEnd={handleTouchEnd}
    >
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:start-4 focus:top-4 focus:z-50 focus:rounded-md focus:bg-background focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:text-foreground focus:shadow-lg"
      >
        {t('nav.skipToMain')}
      </a>

      {/* Desktop sidebar */}
      <aside className="relative z-10 hidden w-64 shrink-0 border-e border-border bg-sidebar md:flex md:flex-col">
        <NavigationPanel
          location={location}
          displayName={displayName}
          email={email}
          avatarUrl={avatarUrl}
          initials={initials}
          onSignOut={handleSignOut}
          showNotifications
        />
      </aside>

      {/* Mobile navigation drawer */}
      <Sheet open={isMobileNavOpen} onOpenChange={setIsMobileNavOpen}>
        <SheetContent
          side={direction === 'rtl' ? 'right' : 'left'}
          id="mobile-navigation"
          onTouchStart={handleTouchStart}
          onTouchEnd={handleTouchEnd}
          className="w-[min(19rem,calc(100vw-2rem))] max-w-none border-e border-border bg-sidebar p-0"
        >
          <SheetTitle className="sr-only">{t('nav.mobileTitle')}</SheetTitle>
          <SheetDescription className="sr-only">
            {t('nav.mobileDescription')}
          </SheetDescription>
          <NavigationPanel
            location={location}
            displayName={displayName}
            email={email}
            avatarUrl={avatarUrl}
            initials={initials}
            onNavigate={closeMobileNav}
            onSignOut={handleSignOut}
          />
        </SheetContent>
      </Sheet>

      {/* Main content */}
          <main id="main-content" tabIndex={-1} className="relative flex min-h-0 flex-1 flex-col overflow-hidden">
        {/* Mobile header */}
        <header className="relative z-10 flex h-16 shrink-0 items-center justify-between border-b border-border bg-card px-4 md:hidden">
          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              size="icon"
              className="h-10 w-10 shrink-0"
              onClick={() => setIsMobileNavOpen(true)}
              aria-label={t('nav.openMenu')}
              aria-expanded={isMobileNavOpen}
              aria-controls="mobile-navigation"
            >
              <Menu className="h-5 w-5" aria-hidden="true" />
            </Button>
            <Link href="/dashboard" className="flex items-center gap-2 group">
              <img src={logoUrl} alt="Askolo" className="h-7 w-auto object-contain" />
              <span className="font-display text-xl font-bold tracking-tight transition-colors group-hover:text-primary">
                Askolo
              </span>
            </Link>
          </div>
          <NotificationBell />
        </header>

        <div className="relative flex-1 overflow-y-auto bg-background">
          <div className="pointer-events-none absolute inset-x-0 top-0 h-48 bg-gradient-to-b from-primary/10 via-primary/5 to-transparent" />
          <div className="relative flex min-h-full flex-col">
            <header className="flex shrink-0 items-center justify-between gap-4 border-b border-border/60 bg-background/80 px-4 py-5 backdrop-blur-md md:px-8">
              <div>
                 <p className="text-xs font-medium uppercase tracking-wider text-primary">{t('app.workspace')}</p>
                <p className="mt-1 text-sm text-muted-foreground">
                   {(() => {
                     const item = navItems.find((entry) => location === entry.href || location.startsWith(`${entry.href}/`));
                     return item ? t(item.labelKey) : t('app.overview');
                   })()}
                </p>
              </div>
              <div className="hidden items-center gap-2 text-end text-xs text-muted-foreground sm:flex">
                <span className="rounded-full border border-border bg-card px-3 py-1.5">
                  <bdi>{formatDate(new Date(), { weekday: 'short', month: 'short', day: 'numeric' })}</bdi>
                </span>
              </div>
            </header>
            <div className="relative flex-1 p-4 md:p-8">
              {children}
            </div>
          </div>
        </div>
        <footer className="flex shrink-0 flex-col items-center justify-center gap-4 border-t border-border/50 bg-background px-4 py-4 text-xs text-muted-foreground sm:flex-row sm:gap-6 md:px-8">
          <span>© {new Date().getFullYear()} Askolo</span>
          <a href={toPublicUrl('/privacy')} className="transition-colors hover:text-primary">
            {t('common.privacyPolicy')}
          </a>
          <a href={toPublicUrl('/terms')} className="transition-colors hover:text-primary">
            {t('common.termsOfService')}
          </a>
        </footer>
      </main>

      {/* Persistent AI assistant sidebar */}
      <AssistantSidebar />
    </div>
  );
}

export function AppLayout({ children }: { children: ReactNode }) {
  return (
    <NotificationProvider>
      <AssistantProvider>
        <AppLayoutInner>{children}</AppLayoutInner>
      </AssistantProvider>
    </NotificationProvider>
  );
}
