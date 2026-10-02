import { useEffect, useRef, type ReactNode } from 'react';
import { Link, useLocation } from 'wouter';
import {
  CalendarDays,
  CheckCircle2,
  ClipboardList,
  LayoutDashboard,
  ListTodo,
  LogOut,
  Mail,
  MoreHorizontal,
  NotebookPen,
  Target,
  UserCircle,
  WalletCards,
  Zap,
} from 'lucide-react';
import logoUrl from '/logo.png';
import { AssistantProvider } from '@/contexts/assistant-context';
import { KeyboardShortcutProvider } from '@/contexts/keyboard-shortcut-context';
import { NotificationProvider } from '@/contexts/notification-context';
import { useAppAuth } from '@/contexts/auth-context';
import { useLocale } from '@/contexts/locale-context';
import { useCreditBalance } from '@/hooks/use-credit-balance';
import { useGoogleConnectionCheck } from '@/hooks/use-google-connection-check';
import { AssistantSidebar } from '@/components/assistant-sidebar';
import { NotificationBell } from '@/components/notification-bell';
import { BalanceIndicator } from '@/components/credits/balance-indicator';
import { CommandMenu } from '@/components/layout/command-menu';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarSeparator,
  SidebarTrigger,
  useSidebar,
} from '@/components/ui/sidebar';
import { isAppProductionHost, toPublicUrl } from '@/lib/site-domains';

const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');

const navigation = [
  { href: '/dashboard', labelKey: 'nav.dashboard', icon: LayoutDashboard },
  { href: '/habits', labelKey: 'nav.habits', icon: CheckCircle2 },
  { href: '/goals', labelKey: 'nav.goals', icon: Target },
  { href: '/plan', labelKey: 'nav.dailyPlan', icon: ListTodo },
  { href: '/calendar', labelKey: 'nav.calendar', icon: CalendarDays },
  { href: '/chores', labelKey: 'nav.chores', icon: ClipboardList },
  { href: '/notes', labelKey: 'nav.notes', icon: NotebookPen },
  { href: '/actions', labelKey: 'nav.actions', icon: Zap },
  { href: '/email', labelKey: 'nav.email', icon: Mail },
] as const;

function isCurrentRoute(location: string, href: string) {
  return location === href || location.startsWith(`${href}/`);
}

function getInitials(firstName: string | null | undefined, lastName: string | null | undefined) {
  const initials = [firstName?.[0], lastName?.[0]].filter(Boolean).join('');
  return initials.toUpperCase() || 'U';
}

function AccountMenu({
  displayName,
  email,
  avatarUrl,
  initials,
  onSignOut,
}: {
  displayName: string;
  email: string;
  avatarUrl?: string;
  initials: string;
  onSignOut: () => void;
}) {
  const { direction, t } = useLocale();

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton
              type="button"
              size="lg"
              className="aria-expanded:bg-sidebar-accent aria-expanded:text-sidebar-accent-foreground"
              aria-label={displayName}
              data-testid="button-account-menu"
            >
              <Avatar className="border border-sidebar-border">
                <AvatarImage src={avatarUrl} alt="" />
                <AvatarFallback className="bg-sidebar-primary/20 text-sidebar-primary">
                  {initials}
                </AvatarFallback>
              </Avatar>
              <span className="grid min-w-0 flex-1 text-start leading-tight">
                <span className="truncate text-sm font-medium">
                  <bdi dir="auto">{displayName}</bdi>
                </span>
                <span className="truncate text-xs text-sidebar-foreground/70">
                  <bdi dir="ltr" lang="en">{email}</bdi>
                </span>
              </span>
              <MoreHorizontal className="ms-auto size-4 opacity-60" aria-hidden="true" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent side={direction === 'rtl' ? 'left' : 'right'} align="end">
            <DropdownMenuGroup>
              <DropdownMenuLabel>
                <bdi dir="auto">{displayName}</bdi>
              </DropdownMenuLabel>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem asChild data-testid="menu-profile">
                <Link href="/profile">
                  <UserCircle aria-hidden="true" />
                  {t('profile.account')}
                </Link>
              </DropdownMenuItem>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={onSignOut} data-testid="menu-sign-out">
              <LogOut aria-hidden="true" />
              {t('nav.signOut')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}

function NavigationPanel({
  location,
  displayName,
  email,
  avatarUrl,
  initials,
  onSignOut,
}: {
  location: string;
  displayName: string;
  email: string;
  avatarUrl?: string;
  initials: string;
  onSignOut: () => void;
}) {
  const { t } = useLocale();
  const commands = navigation.map((item) => ({
    href: item.href,
    label: t(item.labelKey),
    icon: item.icon,
  }));

  return (
    <div className="flex h-full min-h-0 flex-col bg-card text-sidebar-foreground">
      <SidebarHeader className="gap-3">
        <div className="flex items-center gap-2 px-2 pr-12 md:pr-2">
          <Link
            href="/dashboard"
            className="flex min-w-0 flex-1 items-center gap-2 rounded-md text-sidebar-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring"
            data-testid="link-brand-dashboard"
          >
            <img src={logoUrl} alt="Askolo" className="h-7 w-auto object-contain" />
            <span className="truncate font-display text-xl font-bold tracking-tight">Askolo</span>
          </Link>
        </div>
        <div className="px-2">
          <CommandMenu commands={commands} />
        </div>
      </SidebarHeader>

      <SidebarContent role="navigation" aria-label={t('nav.main')}>
        <SidebarGroup>
          <SidebarGroupLabel>{t('app.workspace')}</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {navigation.map((item) => {
                const active = isCurrentRoute(location, item.href);
                const Icon = item.icon;

                return (
                  <SidebarMenuItem key={item.href}>
                    <SidebarMenuButton
                      asChild
                      isActive={active}
                      tooltip={t(item.labelKey)}
                      className="rtl:text-right"
                      data-testid={`link-nav-${item.href.slice(1).replaceAll('/', '-')}`}
                    >
                      <Link
                        href={item.href}
                        aria-current={active ? 'page' : undefined}
                      >
                        <Icon aria-hidden="true" />
                        <span>{t(item.labelKey)}</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                );
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <SidebarSeparator className="mx-0" />
        <BalanceIndicator
          balanceUsdMicros={useCreditBalance(location)}
          label={t('nav.aiCredits')}
          className="w-full justify-center rounded-md border-sidebar-border bg-sidebar text-sidebar-foreground hover:border-sidebar-primary/40 hover:bg-sidebar-accent"
        />
        <AccountMenu
          displayName={displayName}
          email={email}
          avatarUrl={avatarUrl}
          initials={initials}
          onSignOut={onSignOut}
        />
      </SidebarFooter>
    </div>
  );
}

function ShellFrame({ children }: { children: ReactNode }) {
  const { user, signOut } = useAppAuth();
  const { direction, t } = useLocale();
  const [location] = useLocation();
  const { setOpenMobile } = useSidebar();
  const mainContentRef = useRef<HTMLDivElement>(null);
  const previousLocationRef = useRef(location);

  useGoogleConnectionCheck();

  const displayName =
    [user?.firstName, user?.lastName].filter(Boolean).join(' ') || 'User';
  const email = user?.email || '';
  const avatarUrl = user?.imageUrl || user?.profileImageUrl || undefined;
  const initials = getInitials(user?.firstName, user?.lastName);
  const handleSignOut = () => {
    void signOut({
      redirectUrl: isAppProductionHost() ? toPublicUrl('/') : basePath || '/',
    });
  };

  useEffect(() => {
    setOpenMobile(false);
  }, [location, setOpenMobile]);

  useEffect(() => {
    if (previousLocationRef.current === location) return;
    previousLocationRef.current = location;

    let observer: MutationObserver | undefined;
    let fallbackTimer: number | undefined;
    const frame = window.requestAnimationFrame(() => {
      const main = mainContentRef.current;
      if (!main) return;

      const focusRouteHeading = () => {
        const heading = main.querySelector<HTMLElement>('h1');
        if (!heading) return false;
        heading.tabIndex = -1;
        heading.dataset.routeFocusTarget = 'true';
        heading.focus();
        return true;
      };

      if (focusRouteHeading()) return;

      observer = new MutationObserver(() => {
        if (focusRouteHeading()) observer?.disconnect();
      });
      observer.observe(main, { childList: true, subtree: true });
      fallbackTimer = window.setTimeout(() => {
        observer?.disconnect();
        main.focus();
      }, 3000);
    });

    return () => {
      window.cancelAnimationFrame(frame);
      observer?.disconnect();
      if (fallbackTimer !== undefined) window.clearTimeout(fallbackTimer);
    };
  }, [location]);

  return (
    <div className="relative flex h-dvh min-h-0 w-full overflow-hidden bg-card text-foreground">
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:start-4 focus:top-4 focus:z-50 focus:rounded-md focus:bg-background focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:text-foreground focus:shadow-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
        data-testid="link-skip-to-main"
      >
        {t('nav.skipToMain')}
      </a>

      <Sidebar
        side={direction === 'rtl' ? 'right' : 'left'}
        variant="inset"
        mobileCloseButton
        mobileTitle={t('nav.mobileTitle')}
        mobileDescription={t('nav.main')}
        className="absolute h-full"
      >
        <NavigationPanel
          location={location}
          displayName={displayName}
          email={email}
          avatarUrl={avatarUrl}
          initials={initials}
          onSignOut={handleSignOut}
        />
      </Sidebar>

      <SidebarInset
        id="main-content"
        tabIndex={-1}
        className="min-w-0 overflow-hidden rounded-lg bg-muted dark:bg-background focus-visible:outline-none"
      >
        <div
          ref={mainContentRef}
          className="relative flex min-h-0 flex-1 flex-col overflow-hidden"
        >
          <div className="flex shrink-0 items-center justify-between gap-2 border-b border-sidebar/60 p-2 md:p-3">
            <SidebarTrigger
              type="button"
              aria-label={t('nav.mobileTitle')}
              data-testid="button-open-navigation"
              className="size-9 [&>svg]:rtl:-scale-x-100"
            />
            <NotificationBell className="shrink-0" />
          </div>
          <div className="relative min-h-0 flex-1 overflow-y-auto p-4 pt-1 md:p-8 md:pt-3">
            {children}
          </div>
        </div>
      </SidebarInset>

      <AssistantSidebar />
    </div>
  );
}

export function SidebarAppLayout({ children }: { children: ReactNode }) {
  return (
    <NotificationProvider>
      <AssistantProvider>
        <KeyboardShortcutProvider>
          <SidebarProvider keyboardShortcut={false} className="relative h-dvh min-h-0 w-full overflow-hidden">
            <ShellFrame>{children}</ShellFrame>
          </SidebarProvider>
        </KeyboardShortcutProvider>
      </AssistantProvider>
    </NotificationProvider>
  );
}