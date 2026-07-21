import { ReactNode } from 'react';
import { Link, useLocation } from 'wouter';
import { useUser, useClerk } from '@clerk/react';
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
  UserCircle,
} from 'lucide-react';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { AssistantProvider } from '@/contexts/assistant-context';
import { AssistantSidebar } from '@/components/assistant-sidebar';
import logoUrl from '/logo.png';

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
];

const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');

export function AppLayout({ children }: { children: ReactNode }) {
  const { user } = useUser();
  const { signOut } = useClerk();
  const [location] = useLocation();

  const displayName = [user?.firstName, user?.lastName].filter(Boolean).join(' ') || 'User';
  const email = user?.primaryEmailAddress?.emailAddress || '';
  const avatarUrl = user?.imageUrl || undefined;
  const initials = user?.firstName?.[0] || 'U';

  return (
    <AssistantProvider>
      <div className="min-h-screen w-full flex bg-background text-foreground">
        {/* Sidebar */}
        <aside className="w-64 border-r border-border bg-sidebar flex flex-col hidden md:flex shrink-0 z-10 relative">
          <div className="h-16 flex items-center px-6 border-b border-border">
            <Link href="/dashboard" className="flex items-center gap-2 group">
              <img src={logoUrl} alt="Askolo" className="h-7 w-auto object-contain" />
              <span className="font-display font-bold text-xl tracking-tight text-sidebar-foreground">Askolo</span>
            </Link>
          </div>

          <nav className="flex-1 py-6 px-3 space-y-1 overflow-y-auto">
            {navItems.map((item) => {
              const isActive = location === item.href || (item.href !== '/' && location.startsWith(item.href));
              const Icon = item.icon;
              return (
                <Link key={item.href} href={item.href}>
                  <div
                    className={cn(
                      'flex items-center gap-3 px-3 py-2.5 rounded-md transition-all duration-200 cursor-pointer text-sm font-medium',
                      isActive
                        ? 'bg-sidebar-primary/10 text-sidebar-primary'
                        : 'text-sidebar-foreground/70 hover:bg-white/5 hover:text-sidebar-foreground',
                    )}
                    data-testid={`nav-${item.label.toLowerCase().replace(' ', '-')}`}
                  >
                    <Icon className={cn('h-4 w-4', isActive ? 'text-sidebar-primary' : 'text-sidebar-foreground/70')} />
                    {item.label}
                  </div>
                </Link>
              );
            })}
          </nav>

          <div className="p-4 border-t border-border bg-sidebar">
            <Link href="/profile" className="flex items-center gap-3 mb-4 px-2 rounded-md py-1.5 hover:bg-white/5 transition-colors group cursor-pointer">
              <Avatar className="h-9 w-9 border border-border">
                <AvatarImage src={avatarUrl} />
                <AvatarFallback className="bg-sidebar-primary/20 text-sidebar-primary">
                  {initials}
                </AvatarFallback>
              </Avatar>
              <div className="flex flex-col flex-1 overflow-hidden">
                <span className="text-sm font-medium truncate text-sidebar-foreground group-hover:text-sidebar-primary transition-colors">{displayName}</span>
                <span className="text-xs text-sidebar-foreground/60 truncate">{email}</span>
              </div>
              <UserCircle className="h-4 w-4 text-sidebar-foreground/40 group-hover:text-sidebar-primary transition-colors shrink-0" />
            </Link>
            <Button
              variant="outline"
              className="w-full justify-start text-sidebar-foreground/70 border-border hover:bg-white/5 hover:text-sidebar-foreground"
              onClick={() => signOut({ redirectUrl: basePath || '/' })}
              data-testid="button-logout"
            >
              <LogOut className="h-4 w-4 mr-2" />
              Sign Out
            </Button>
          </div>
        </aside>

        {/* Main Content */}
        <main className="flex-1 flex flex-col h-screen overflow-hidden relative">
          {/* Mobile Header */}
          <header className="h-16 flex items-center px-4 border-b border-border bg-card md:hidden shrink-0 z-10 relative">
            <Link href="/dashboard" className="flex items-center gap-2 group">
              <img src={logoUrl} alt="Askolo" className="h-7 w-auto object-contain" />
              <span className="font-display font-bold text-xl tracking-tight group-hover:text-primary transition-colors">Askolo</span>
            </Link>
          </header>

          <div className="flex-1 overflow-y-auto p-4 md:p-8 bg-background relative">
            {children}
            <footer className="mt-10 pt-6 border-t border-border/50 text-xs text-muted-foreground flex flex-col sm:flex-row gap-4 sm:gap-6 justify-center items-center">
              <span>© {new Date().getFullYear()} Askolo</span>
              <Link href="/privacy" className="hover:text-primary transition-colors">Privacy Policy</Link>
              <Link href="/terms" className="hover:text-primary transition-colors">Terms of Service</Link>
            </footer>
          </div>
        </main>

        {/* Persistent AI assistant sidebar */}
        <AssistantSidebar />
      </div>
    </AssistantProvider>
  );
}
