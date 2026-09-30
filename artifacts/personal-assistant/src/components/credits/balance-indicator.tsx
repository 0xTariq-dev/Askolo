import { Link } from 'wouter';
import { WalletCards } from 'lucide-react';
import { cn } from '@/lib/utils';
import { formatUsdMicros } from '@/lib/credit-api';

type BalanceIndicatorProps = {
  balanceUsdMicros: number | null;
  label: string;
  compact?: boolean;
  className?: string;
};

export function BalanceIndicator({
  balanceUsdMicros,
  label,
  compact = false,
  className,
}: BalanceIndicatorProps) {
  const amount = balanceUsdMicros === null ? null : formatUsdMicros(balanceUsdMicros);
  const accessibleName = amount ? `${label}: ${amount}` : `${label} unavailable`;

  return (
    <Link
      href="/credits"
      aria-label={accessibleName}
      title={accessibleName}
      data-testid="balance-indicator"
      className={cn(
        'inline-flex min-h-9 shrink-0 items-center gap-1.5 rounded-full border border-border bg-card px-2 text-xs font-medium text-foreground transition-colors hover:border-primary/40 hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:gap-2 sm:px-3',
        className,
      )}
    >
      <WalletCards className="h-4 w-4 shrink-0 text-primary" aria-hidden="true" />
      <span className={compact ? 'sr-only sm:not-sr-only' : 'hidden xl:inline'}>
        {label}
      </span>
      <bdi dir="ltr" className="font-mono text-xs font-semibold">
        {amount ?? '—'}
      </bdi>
    </Link>
  );
}