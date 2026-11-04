import { useEffect, useRef, type ReactNode } from 'react';
import { Link, useLocation } from 'wouter';
import {
  ChevronDown,
  LayoutDashboard,
  ListTodo,
  LogOut,
  Mail,
  MoreHorizontal,
  MessageCircle,
  NotebookPen,
  UserCircle,
} from 'lucide-react';
import { IconPalette, IconStack2, IconWallet } from '@tabler/icons-react';
import logoUrl from '/icons/logo-display.png';
import { AssistantProvider } from '@/contexts/assistant-context';
import { ConfirmDialogProvider, useConfirmDialog } from '@/contexts/confirm-dialog-context';
import { KeyboardShortcutProvider } from '@/contexts/keyboard-shortcut-context';
import { NotificationProvider } from '@/contexts/notification-context';
import { useAppAuth } from '@/contexts/auth-context';
import { useLocale } from '@/contexts/locale-context';
import { useSpaceScope } from '@/contexts/space-context';
import { useCreditBalance } from '@/hooks/use-credit-balance';
import { useGoogleConnectionCheck } from '@/hooks/use-google-connection-check';
import { NotificationBell } from '@/components/notification-bell';
import { CommandMenu } from '@/components/layout/command-menu';
import { ThemeModeToggle } from '@/components/settings/theme-mode-toggle';
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
import { formatCredits } from '@/lib/credit-api';

const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');

const navigation = [
  { href: '/home', labelKey: 'nav.home', icon: LayoutDashboard },
  { href: '/agent', labelKey: 'nav.agent', icon: MessageCircle },
  { href: '/agenda', labelKey: 'nav.agenda', icon: ListTodo },
  { href: '/email', labelKey: 'nav.email', icon: Mail },
  { href: '/notes', labelKey: 'nav.notes', icon: NotebookPen },
] as const;

const moreNavigation: (typeof navigation[number])[] = [];

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
  balanceUsdMicros,
  compact,
  onSignOut,
}: {
  displayName: string;
  email: string;
  avatarUrl?: string;
  initials: string;
  balanceUsdMicros: number | null;
  compact: boolean;
  onSignOut: () => void;
}) {
  const { t, locale } = useLocale();
  const amount = balanceUsdMicros === null ? '—' : formatCredits(balanceUsdMicros);
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
                <span className={`truncate text-sm font-medium ${compact ? 'sr-only' : ''}`}>
                  <bdi dir="auto">{displayName}</bdi>
                </span>
                <span className={`truncate text-xs text-sidebar-foreground/70 ${compact ? 'sr-only' : ''}`}>
                  <bdi dir="ltr" lang="en">{email}</bdi>
                </span>
              </span>
              {!compact && <MoreHorizontal className="ms-auto size-4 opacity-60" aria-hidden="true" />}
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent side="top" align="end" className="app-border-subtle bg-popover/55 backdrop-blur-2xl">
            <DropdownMenuGroup>
              <DropdownMenuLabel>
                <span className="grid gap-0.5">
                  <bdi dir="auto" className="font-medium">{displayName}</bdi>
                  <bdi dir="ltr" lang="en" className="text-xs font-normal text-muted-foreground">{email}</bdi>
                </span>
              </DropdownMenuLabel>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem asChild data-testid="menu-profile">
                <Link href="/settings/account/profile">
                  <UserCircle aria-hidden="true" />
                  Settings
                </Link>
              </DropdownMenuItem>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild data-testid="menu-balance">
              <Link
                href="/settings/space/billing"
                aria-label={`${t('nav.aiCredits')}: ${amount}`}
                className="flex items-center"
              >
                <IconWallet aria-hidden="true" />
                <bdi dir="ltr" className="font-mono text-xs font-semibold">{amount}</bdi>
              </Link>
            </DropdownMenuItem>
            <div className="flex items-center justify-between gap-3 px-2 py-1.5">
              <div className="flex items-center gap-2 text-sm">
                <IconPalette className="size-4" aria-hidden="true" />
                <span>{locale === 'ar' ? 'المظهر' : 'Theme'}</span>
              </div>
              <ThemeModeToggle placement="menu" />
            </div>
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

function SpaceSelector({ compact }: { compact: boolean }) {
  const { locale } = useLocale();
  const { spaces, selectedSpaceId, setSelectedSpaceId, loading, error } = useSpaceScope();
  const { user } = useAppAuth();
  const allSpacesLabel = locale === 'ar' ? 'كل المساحات' : 'All Spaces';
  const accountName = [user?.firstName, user?.lastName].filter(Boolean).join(' ').trim()
    || user?.email?.split('@')[0]
    || 'Personal';
  const personalSpaceName = locale === 'ar' ? `مساحة ${accountName}` : `${accountName}'s space`;
  const getSpaceName = (space: (typeof spaces)[number]) => space.isPersonal
    ? personalSpaceName
    : space.name.replace(/\s*workspace\b/ig, '').trim();
  const selectedSpace = spaces.find((space) => space.id === selectedSpaceId);
  const selectedName = selectedSpace ? getSpaceName(selectedSpace) : allSpacesLabel;

  return (
    <div className={compact ? 'flex justify-center' : 'px-2'}>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            disabled={loading || Boolean(error)}
            aria-label={`${locale === 'ar' ? 'المساحة الحالية' : 'Current Space'}: ${selectedName}`}
            title={selectedName}
            className={`flex w-full items-center rounded-md border border-sidebar-border bg-sidebar text-start text-sm font-medium text-sidebar-foreground hover:bg-sidebar-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring disabled:cursor-wait disabled:opacity-60 ${
              compact ? 'size-8 min-h-8 justify-center p-0' : 'min-h-9 justify-between gap-2 px-3'
            }`}
            data-testid="space-selector"
          >
            {compact ? (
              <IconStack2 aria-hidden="true" className="size-4" />
            ) : (
              <>
                <span className="truncate">
                  {loading ? (locale === 'ar' ? 'جارٍ تحميل المساحات…' : 'Loading Spaces…') : selectedName}
                </span>
                <ChevronDown aria-hidden="true" className="size-4 shrink-0" />
              </>
            )}
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="start"
          className={compact ? 'w-56' : 'w-[var(--radix-dropdown-menu-trigger-width)]'}
        >
          <DropdownMenuItem onSelect={() => setSelectedSpaceId(null)}>{allSpacesLabel}</DropdownMenuItem>
          <DropdownMenuSeparator />
          {spaces.map((space) => (
            <DropdownMenuItem key={space.id} onSelect={() => setSelectedSpaceId(space.id)}>
              <span className="min-w-0 truncate">{getSpaceName(space)}</span>
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
      {error && <p className="mt-2 text-xs text-destructive" role="alert">{error}</p>}
    </div>
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
  const { t, locale } = useLocale();
  const { state, isMobile } = useSidebar();
  const compact = state === 'collapsed' && !isMobile;
  const balanceUsdMicros = useCreditBalance(location);
  const commands = [...navigation, ...moreNavigation].map((item) => ({
    href: item.href,
    label: t(item.labelKey),
    icon: item.icon,
  }));

  return (
    <div className="flex h-full min-h-0 flex-col bg-card text-sidebar-foreground">
      <SidebarHeader className="gap-3">
        <div className={`flex ${compact ? 'flex-col items-center gap-2' : 'items-center justify-between gap-2 px-2'}`}>
          <Link
            href="/home"
            className={`flex min-w-0 items-center gap-2 rounded-md text-sidebar-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring ${
              compact ? 'justify-center' : 'flex-1'
            }`}
            data-testid="link-brand-dashboard"
          >
            <img src={logoUrl} alt="Askolo" className="askolo-brand-logo h-7 w-auto object-contain" />
            {!compact && <span className="truncate font-display text-xl font-bold tracking-tight">Askolo</span>}
          </Link>
          <div className={`flex items-center ${compact ? 'flex-col' : 'gap-1'}`}>
            <NotificationBell className={compact ? 'size-8' : 'size-9'} />
            <SidebarTrigger
              type="button"
              aria-label={locale === 'ar' ? 'تبديل الشريط الجانبي' : 'Toggle sidebar'}
              data-testid="button-toggle-sidebar"
              className={`hidden md:flex ${compact ? 'size-8' : 'size-9'}`}
            />
          </div>
        </div>
        <div className={compact ? 'flex justify-center' : 'px-2'}>
          <CommandMenu commands={commands} collapsed={compact} />
        </div>
        <SpaceSelector compact={compact} />
      </SidebarHeader>

      <SidebarContent role="navigation" aria-label={t('nav.main')}>
        <SidebarGroup>
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
        {moreNavigation.length > 0 && <SidebarGroup>
          <SidebarGroupLabel>{t('nav.more')}</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {moreNavigation.map((item) => {
                const active = isCurrentRoute(location, item.href);
                const Icon = item.icon;
                return (
                  <SidebarMenuItem key={item.href}>
                    <SidebarMenuButton
                      asChild
                      isActive={active}
                      tooltip={t(item.labelKey)}
                      className="rtl:text-right"
                      data-testid={`link-nav-${item.href.slice(1)}`}
                    >
                      <Link href={item.href} aria-current={active ? 'page' : undefined}>
                        <Icon aria-hidden="true" />
                        <span>{t(item.labelKey)}</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                );
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>}
      </SidebarContent>

      <SidebarFooter>
        <SidebarSeparator className="mx-0" />
        <AccountMenu
          displayName={displayName}
          email={email}
          avatarUrl={avatarUrl}
          initials={initials}
          balanceUsdMicros={balanceUsdMicros}
          compact={compact}
          onSignOut={onSignOut}
        />
      </SidebarFooter>
    </div>
  );
}

function ShellFrame({ children }: { children: ReactNode }) {
  const { confirm } = useConfirmDialog();
  const { user, signOut } = useAppAuth();
  const { direction, t, locale } = useLocale();
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
  const handleSignOut = async () => {
    if (!(await confirm({
      title: 'Sign out?',
      description: 'You will need to sign in again to access your account.',
      confirmLabel: 'Sign out',
      variant: 'logout',
    }))) return;
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
        collapsible="icon"
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
          <div className="flex shrink-0 items-center gap-2 border-b border-sidebar/60 p-2 md:hidden">
            <SidebarTrigger
              type="button"
              aria-label={locale === 'ar' ? 'فتح قائمة التنقل' : 'Open navigation menu'}
              data-testid="button-open-navigation"
              className="size-9 [&>svg]:rtl:-scale-x-100"
            />
          </div>
          <div className="relative min-h-0 flex-1 overflow-y-auto p-4 pt-1 md:p-8 md:pt-3 text-[12px] font-light">
            {children}
          </div>
        </div>
      </SidebarInset>
    </div>
  );
}

export function SidebarAppLayout({ children }: { children: ReactNode }) {
  return (
    <NotificationProvider>
      <AssistantProvider>
        <ConfirmDialogProvider>
          <KeyboardShortcutProvider>
            <SidebarProvider keyboardShortcut={false} className="relative h-dvh min-h-0 w-full overflow-hidden">
              <ShellFrame>{children}</ShellFrame>
            </SidebarProvider>
          </KeyboardShortcutProvider>
        </ConfirmDialogProvider>
      </AssistantProvider>
    </NotificationProvider>
  );
}