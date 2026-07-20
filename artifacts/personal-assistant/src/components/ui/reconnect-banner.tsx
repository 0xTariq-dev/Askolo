import { RefreshCw, WifiOff } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

interface ReconnectBannerProps {
  service: 'calendar' | 'gmail';
  onRefresh?: () => void;
  isRefreshing?: boolean;
  className?: string;
}

const SERVICE_LABELS = {
  calendar: 'Google Calendar',
  gmail: 'Gmail',
};

const SERVICE_DESCRIPTIONS = {
  calendar: 'Calendar sync and event management require an active Google Calendar connection.',
  gmail: 'Email triage and reply drafting require an active Gmail connection.',
};

export function ReconnectBanner({
  service,
  onRefresh,
  isRefreshing,
  className,
}: ReconnectBannerProps) {
  const label = SERVICE_LABELS[service];
  const description = SERVICE_DESCRIPTIONS[service];

  return (
    <div
      className={cn(
        'flex flex-col sm:flex-row sm:items-center justify-between gap-3 rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-3',
        className,
      )}
    >
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-amber-500/30 bg-amber-500/20 text-amber-500">
          <WifiOff className="h-4 w-4" />
        </div>
        <div>
          <p className="text-sm font-semibold text-amber-400">
            {label} needs reconnecting
          </p>
          <p className="mt-0.5 text-xs text-amber-400/80">{description}</p>
          <p className="mt-1 text-xs text-muted-foreground">
            To restore access, reconnect the{' '}
            <span className="font-medium">{label}</span> integration in your Replit
            workspace, then click&nbsp;<span className="font-medium">Check connection</span> below.
          </p>
        </div>
      </div>
      {onRefresh && (
        <Button
          variant="outline"
          size="sm"
          onClick={onRefresh}
          disabled={isRefreshing}
          className="shrink-0 border-amber-500/30 text-amber-400 hover:bg-amber-500/20 hover:text-amber-300"
        >
          <RefreshCw className={cn('h-3.5 w-3.5 mr-1.5', isRefreshing && 'animate-spin')} />
          {isRefreshing ? 'Checking…' : 'Check connection'}
        </Button>
      )}
    </div>
  );
}
