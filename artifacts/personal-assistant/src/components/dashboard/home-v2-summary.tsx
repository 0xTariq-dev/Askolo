import { useEffect, useState } from 'react';
import { AlertCircle, CalendarDays, CheckCircle2, Clock3, Mail } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { getV2Home, type V2HomeResponse } from '@/lib/home-v2-api';

const freshnessLabel: Record<V2HomeResponse['emailSummary']['freshness'], string> = {
  current: 'Updated',
  stale: 'Out of date',
  syncing: 'Updating',
  error: 'Sync failed',
  unavailable: 'Not available',
};

export function HomeV2Summary() {
  const [home, setHome] = useState<V2HomeResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const controller = new AbortController();
    let cancelled = false;

    getV2Home(controller.signal)
      .then((data) => {
        if (!cancelled) {
          setHome(data);
          setError(null);
        }
      })
      .catch((requestError: unknown) => {
        if (!cancelled && !controller.signal.aborted) {
          setError(requestError instanceof Error ? requestError.message : 'Home could not be loaded.');
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, []);

  if (loading) {
    return (
      <section aria-label="New Home summary" aria-busy="true" className="grid gap-4 md:grid-cols-3">
        {[0, 1, 2].map((item) => (
          <div key={item} className="h-36 animate-pulse rounded-xl border border-border bg-card" />
        ))}
      </section>
    );
  }

  if (error || !home) {
    return (
      <Card aria-label="New Home summary">
        <CardContent className="flex items-center gap-3 p-5 text-sm text-muted-foreground">
          <AlertCircle className="h-4 w-4 shrink-0" aria-hidden="true" />
          <span role="status">{error ?? 'Home is unavailable.'}</span>
        </CardContent>
      </Card>
    );
  }

  const email = home.emailSummary;
  const countsAvailable = email.freshness === 'current' || email.freshness === 'stale';

  return (
    <section aria-label="Home summary" className="grid gap-4 md:grid-cols-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="flex items-center gap-2 text-base">
            <CheckCircle2 className="h-4 w-4 text-primary" aria-hidden="true" />
            Focus
          </CardTitle>
        </CardHeader>
        <CardContent>
          {home.focusActions.length ? (
            <ul className="space-y-2">
              {home.focusActions.map((action) => (
                <li key={`${action.workspaceId}:${action.taskId}:${action.occurrenceOn ?? ''}`} className="text-sm">
                  <span className="font-medium">{action.title}</span>
                  {home.scope === 'all_spaces' && (
                    <span className="ms-2 text-xs text-muted-foreground">{action.workspaceName}</span>
                  )}
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-muted-foreground">No focus actions for today.</p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="flex items-center gap-2 text-base">
            <Mail className="h-4 w-4 text-primary" aria-hidden="true" />
            Email
          </CardTitle>
        </CardHeader>
        <CardContent>
          {countsAvailable ? (
            <>
              <div className="grid grid-cols-2 gap-3">
                <Count label="Unread inbox" count={email.unreadInboxCount} />
                <Count label="Action needed" count={email.actionNeededCount} />
              </div>
              <p className="mt-3 flex items-center gap-1.5 text-xs text-muted-foreground" role="status">
                <Clock3 className="h-3.5 w-3.5" aria-hidden="true" />
                {freshnessLabel[email.freshness]}
                {email.lastSyncedAt ? ` · ${new Date(email.lastSyncedAt).toLocaleString()}` : ''}
              </p>
            </>
          ) : (
            <p className="text-sm text-muted-foreground" role="status">
              {freshnessLabel[email.freshness]}. Counts will appear after a successful sync.
            </p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="flex items-center gap-2 text-base">
            <CalendarDays className="h-4 w-4 text-primary" aria-hidden="true" />
            Calendar
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-2xl font-semibold">{home.calendarSummary.eventCount}</p>
          <p className="text-sm text-muted-foreground">events today</p>
          {home.calendarSummary.nextEvent && (
            <p className="mt-3 text-sm">
              Next: {home.calendarSummary.nextEvent.title} ·{' '}
              {new Date(home.calendarSummary.nextEvent.startsAt).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}
            </p>
          )}
        </CardContent>
      </Card>
    </section>
  );
}

function Count({ label, count }: { label: string; count: number }) {
  return (
    <div>
      <p className="text-2xl font-semibold tabular-nums">{count}</p>
      <p className="text-xs text-muted-foreground">{label}</p>
    </div>
  );
}
