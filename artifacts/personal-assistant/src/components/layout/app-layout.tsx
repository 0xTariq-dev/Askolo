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
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
} from '@/components/ui/sheet';
import { AssistantProvider } from '@/contexts/assistant-context';
import { NotificationProvider } from '@/contexts/notification-context';
import { AssistantSidebar } from '@/components/assistant-sidebar';
import { NotificationBell } from '@/components/notification-bell';
import { useGoogleConnectionCheck } from '@/hooks/use-google-connection-check';
import { useIsMobile } from '@/hooks/use-mobile';
import logoUrl from '/logo.png';
import { isAppProductionHost, toPublicUrl } from '@/lib/site-domains';
import { useAppAuth } from '@/contexts/auth-context';

const navItems = [
  { href: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { href: '/habits', label: 'Habits', icon: CheckCircle2 },
  { href: '/goals', label: 'Goals', icon: Target },
  { href: '/plan', label: 'Daily Plan', icon: ListTodo },
  { href: '/calendar', label: 'Calendar', icon: Calendar },
  { href: '/chores', label: 'Chores', icon: ClipboardList },
  { href: '/notes', label: 'Notes', icon: StickyNote },
  { href: '/actions', label: 'Actions', icon: Zap },
  { href: '/email', label: 'Email', icon: Mail },
  { href: '/credits', label: 'AI Credits', icon: WalletCards },
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
        aria-label="Main navigation"
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
              className={cn(
                'flex items-center gap-3 rounded-md px-3 py-3 text-sm font-medium transition-all duration-200',
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring',
                isActive
                  ? 'bg-sidebar-primary/10 text-sidebar-primary'
                  : 'text-sidebar-foreground/70 hover:bg-white/5 hover:text-sidebar-foreground',
              )}
              data-testid={`nav-${item.label.toLowerCase().replace(' ', '-')}`}
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
              {item.label}
            </Link>
          );
        })}
      </nav>

      <div className="shrink-0 border-t border-border bg-sidebar p-4">
        <Link
          href="/profile"
          onClick={onNavigate}
          className="group mb-4 flex items-center gap-3 rounded-md px-2 py-2 transition-colors hover:bg-white/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring"
        >
          <Avatar className="h-9 w-9 border border-border">
            <AvatarImage src={avatarUrl} />
            <AvatarFallback className="bg-sidebar-primary/20 text-sidebar-primary">
              {initials}
            </AvatarFallback>
          </Avatar>
          <span className="flex min-w-0 flex-1 flex-col overflow-hidden">
            <span className="truncate text-sm font-medium text-sidebar-foreground transition-colors group-hover:text-sidebar-primary">
              {displayName}
            </span>
            <span className="truncate text-xs text-sidebar-foreground/60">
              {email}
            </span>
          </span>
          <UserCircle
            className="h-4 w-4 shrink-0 text-sidebar-foreground/40 transition-colors group-hover:text-sidebar-primary"
            aria-hidden="true"
          />
        </Link>
        <Button
          variant="outline"
          className="w-full justify-start border-border text-sidebar-foreground/70 hover:bg-white/5 hover:text-sidebar-foreground"
          onClick={onSignOut}
          data-testid="button-logout"
        >
          <LogOut className="mr-2 h-4 w-4" aria-hidden="true" />
          Sign Out
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

    if (isMobileNavOpen || touch.clientX <= MOBILE_EDGE_GESTURE_WIDTH) {
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

    if (!isMobileNavOpen && start.x <= MOBILE_EDGE_GESTURE_WIDTH && deltaX > 0) {
      setIsMobileNavOpen(true);
    } else if (isMobileNavOpen && deltaX < 0) {
      closeMobileNav();
    }
  };

  return (
    <div
      className="flex min-h-screen w-full bg-background text-foreground"
      onTouchStart={handleTouchStart}
      onTouchEnd={handleTouchEnd}
    >
      {/* Desktop sidebar */}
      <aside className="relative z-10 hidden w-64 shrink-0 border-r border-border bg-sidebar md:flex md:flex-col">
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
          side="left"
          id="mobile-navigation"
          onTouchStart={handleTouchStart}
          onTouchEnd={handleTouchEnd}
          className="w-[min(19rem,calc(100vw-2rem))] max-w-none border-r border-border bg-sidebar p-0"
        >
          <SheetTitle className="sr-only">Askolo navigation</SheetTitle>
          <SheetDescription className="sr-only">
            Navigate between your Askolo workspaces.
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
      <main className="relative flex h-screen flex-1 flex-col overflow-hidden">
        {/* Mobile header */}
        <header className="relative z-10 flex h-16 shrink-0 items-center justify-between border-b border-border bg-card px-4 md:hidden">
          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              size="icon"
              className="h-10 w-10 shrink-0"
              onClick={() => setIsMobileNavOpen(true)}
              aria-label="Open navigation menu"
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

        <div className="relative flex-1 overflow-y-auto bg-background p-4 md:p-8">
          {children}
        </div>
        <footer className="flex shrink-0 flex-col items-center justify-center gap-4 border-t border-border/50 bg-background px-4 py-4 text-xs text-muted-foreground sm:flex-row sm:gap-6 md:px-8">
          <span>© {new Date().getFullYear()} Askolo</span>
          <a href={toPublicUrl('/privacy')} className="transition-colors hover:text-primary">
            Privacy Policy
          </a>
          <a href={toPublicUrl('/terms')} className="transition-colors hover:text-primary">
            Terms of Service
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
