import { Link, useLocation } from 'wouter';
import { ArrowUpRight, CreditCard, UserRound } from 'lucide-react';

type SpaceSettingsNavigationProps = {
  active: 'details' | 'members' | 'notifications';
};

const spaceTabs = [
  { id: 'details', label: 'Details', href: '/settings/space/details' },
  { id: 'members', label: 'People', href: '/settings/space/members' },
  { id: 'notifications', label: 'Notifications', href: '/settings/space/notifications' },
] as const;

export function SpaceSettingsNavigation({ active }: SpaceSettingsNavigationProps) {
  const [location] = useLocation();
  const accountSelected = location.startsWith('/settings/account/');

  return (
    <div className="space-y-4">
      <nav aria-label="Settings sections" className="flex flex-wrap gap-1 border-b border-border">
        <Link
          href="/settings/account/profile"
          aria-current={accountSelected ? 'page' : undefined}
          className={`min-h-11 border-b-2 px-3 py-2 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
            accountSelected
              ? 'border-primary text-foreground'
              : 'border-transparent text-muted-foreground hover:text-foreground'
          }`}
        >
          Account
        </Link>
        <Link
          href="/settings/space/details"
          aria-current={!accountSelected ? 'page' : undefined}
          className={`min-h-11 border-b-2 px-3 py-2 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
            !accountSelected
              ? 'border-primary text-foreground'
              : 'border-transparent text-muted-foreground hover:text-foreground'
          }`}
        >
          Space
        </Link>
      </nav>

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <nav aria-label="Space settings" className="flex flex-wrap gap-1">
          {spaceTabs.map((tab) => (
            <Link
              key={tab.id}
              href={tab.href}
              aria-current={active === tab.id ? 'page' : undefined}
              className={`inline-flex min-h-10 items-center rounded-lg px-3 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                active === tab.id
                  ? 'bg-primary/10 text-primary'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground'
              }`}
            >
              {tab.label}
            </Link>
          ))}
        </nav>

        <nav
          aria-label="Personal wallet settings"
          className="flex flex-wrap items-center gap-1 rounded-xl border border-border/60 bg-muted/25 p-1"
        >
          <span className="flex items-center gap-1.5 px-2 text-xs font-medium text-muted-foreground">
            <UserRound aria-hidden="true" className="size-3.5" />
            Personal wallet
          </span>
          <Link
            href="/settings/space/usage"
            className="inline-flex min-h-9 items-center gap-1 rounded-lg px-2.5 text-xs font-medium text-muted-foreground hover:bg-background hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            Usage <ArrowUpRight aria-hidden="true" className="size-3.5" />
          </Link>
          <Link
            href="/settings/space/billing"
            className="inline-flex min-h-9 items-center gap-1 rounded-lg px-2.5 text-xs font-medium text-muted-foreground hover:bg-background hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <CreditCard aria-hidden="true" className="size-3.5" />
            Billing
          </Link>
        </nav>
      </div>
    </div>
  );
}
