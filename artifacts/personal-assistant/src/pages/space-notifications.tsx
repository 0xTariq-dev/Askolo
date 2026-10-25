import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { AlertCircle, Bell, Check, LoaderCircle, Save, UsersRound } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { SettingsLayout } from '@/components/settings/settings-layout';
import { useSpaceScope } from '@/contexts/space-context';
import {
  getV2NotificationPreferences,
  updateV2NotificationPreferences,
  type V2NotificationPreference,
} from '@/lib/agenda-v2-api';

const descriptions: Record<V2NotificationPreference['eventKey'], { title: string; detail: string }> = {
  assignments: {
    title: 'Task assignments',
    detail: 'When someone assigns a task to you directly or through a group.',
  },
  reminders: {
    title: 'Task and habit reminders',
    detail: 'Your choices are saved, but scheduled reminder delivery is not active yet.',
  },
  membership: {
    title: 'Membership and role changes',
    detail: 'Space invitations, member changes, and role updates. Only owners and admins receive these notices.',
  },
};

export function SpaceNotificationsPage() {
  const { spaces, selectedSpaceId, loading: spacesLoading, error: spacesError } = useSpaceScope();
  const [preferences, setPreferences] = useState<V2NotificationPreference[]>([]);
  const [savedPreferences, setSavedPreferences] = useState<V2NotificationPreference[]>([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [loadedForSpaceId, setLoadedForSpaceId] = useState<string | null>(null);
  const [loadAttempt, setLoadAttempt] = useState(0);
  const selectedSpace = spaces.find((space) => space.id === selectedSpaceId);
  const selectedRole = selectedSpace?.role.trim().toLowerCase() ?? '';
  const canSeeMembership = ['owner', 'admin'].includes(selectedRole);
  const categories = useMemo(
    () => canSeeMembership ? ['assignments', 'reminders', 'membership'] as const : ['assignments', 'reminders'] as const,
    [canSeeMembership],
  );

  useEffect(() => {
    if (!selectedSpaceId || spacesLoading) {
      setPreferences([]);
      setSavedPreferences([]);
      return;
    }
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    setSaved(false);
    setLoadedForSpaceId(null);
    getV2NotificationPreferences(selectedSpaceId, controller.signal)
      .then((result) => {
        if (controller.signal.aborted) return;
        const visiblePreferences = canSeeMembership
          ? result
          : result.filter((preference) => preference.eventKey !== 'membership');
        setPreferences(visiblePreferences);
        setSavedPreferences(visiblePreferences);
        setLoadedForSpaceId(selectedSpaceId);
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) {
          setError(reason instanceof Error ? reason.message : 'Notification preferences could not be loaded.');
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [canSeeMembership, loadAttempt, selectedSpaceId, spacesLoading]);

  const dirty = JSON.stringify(preferences) !== JSON.stringify(savedPreferences);
  const updatePreference = (
    eventKey: V2NotificationPreference['eventKey'],
    channel: 'emailEnabled' | 'inAppEnabled',
    value: boolean,
  ) => {
    setError(null);
    setSaved(false);
    setPreferences((current) => current.map((preference) =>
      preference.eventKey === eventKey ? { ...preference, [channel]: value } : preference,
    ));
  };

  const save = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!selectedSpaceId || !dirty) return;
    setSaving(true);
    setError(null);
    setSaved(false);
    try {
      const result = await updateV2NotificationPreferences(
        selectedSpaceId,
        preferences.filter((preference) => preference.eventKey !== 'membership' || canSeeMembership),
      );
      const visibleResult = canSeeMembership
        ? result
        : result.filter((preference) => preference.eventKey !== 'membership');
      setPreferences(visibleResult);
      setSavedPreferences(visibleResult);
      setSaved(true);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Notification preferences could not be saved.');
    } finally {
      setSaving(false);
    }
  };

  return (
    <SettingsLayout>
      <header className="relative overflow-hidden rounded-3xl border border-border/70 bg-card/45 px-5 py-7 shadow-sm sm:px-8 sm:py-9">
        <div aria-hidden="true" className="absolute -end-10 -top-16 size-52 rounded-full border-[30px] border-primary/[0.06]" />
        <div className="relative">
          <div className="mb-5 flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.19em] text-primary">
            <span className="grid size-8 place-items-center rounded-xl bg-primary/10"><Bell aria-hidden="true" className="size-4" /></span>
            Shared Space
          </div>
          <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
            <div>
              <h1 className="font-display text-3xl font-bold tracking-tight sm:text-4xl">Notifications</h1>
              <p className="mt-2 max-w-xl text-sm leading-6 text-muted-foreground">
                Choose which Space updates you receive by email and in the app.
              </p>
            </div>
            {selectedSpace && (
              <div className="inline-flex max-w-full items-center gap-2 self-start rounded-xl border border-border/70 bg-background/70 px-3 py-2 sm:self-auto">
                <span className="grid size-7 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary"><UsersRound aria-hidden="true" className="size-3.5" /></span>
                <span className="max-w-56 truncate text-sm font-medium">{selectedSpace.name}</span>
                <span className="rounded-full bg-muted px-2 py-1 text-[11px] font-semibold text-muted-foreground">{selectedSpace.role}</span>
              </div>
            )}
          </div>
        </div>
      </header>

      {(spacesError || error) && (
        <div role="alert" className="flex items-start justify-between gap-3 rounded-xl border border-destructive/25 bg-destructive/5 p-4 text-sm">
          <AlertCircle aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-destructive" />
          <div className="min-w-0"><p className="font-medium">Notification settings unavailable</p><p className="mt-1 text-muted-foreground">{spacesError ?? error}</p></div>
          {!spacesError && !loading && (
            <Button type="button" variant="outline" size="sm" onClick={() => setLoadAttempt((attempt) => attempt + 1)}>
              Try again
            </Button>
          )}
        </div>
      )}

      {loading || spacesLoading ? (
        <div role="status" aria-label="Loading notification preferences" className="flex items-center gap-3 rounded-2xl border border-border bg-card p-6 text-sm text-muted-foreground">
          <LoaderCircle aria-hidden="true" className="size-4 animate-spin" />Loading your preferences…
        </div>
      ) : selectedSpaceId && loadedForSpaceId === selectedSpaceId && !error ? (
        <Card>
          <CardHeader className="border-b border-border/60 bg-muted/20 px-5 py-5 sm:px-7">
            <CardTitle className="text-lg">Notification channels</CardTitle>
            <CardDescription>Preferences apply to your account in the selected Space.</CardDescription>
          </CardHeader>
          <CardContent className="px-5 py-5 sm:px-7">
            <form onSubmit={save} className="space-y-5">
              <div className="hidden grid-cols-[minmax(0,1fr)_7rem_7rem] gap-3 px-4 text-xs font-semibold text-muted-foreground sm:grid">
                <span>Update</span><span className="text-center">Email</span><span className="text-center">In-app</span>
              </div>
              <div className="space-y-3">
                {categories.map((eventKey) => {
                  const preference = preferences.find((item) => item.eventKey === eventKey);
                  if (!preference) return null;
                  return (
                    <section key={eventKey} className="grid gap-3 rounded-2xl border border-border/70 bg-background/50 p-4 sm:grid-cols-[minmax(0,1fr)_7rem_7rem] sm:items-center">
                      <div className="min-w-0">
                        <h2 className="text-sm font-semibold">{descriptions[eventKey].title}</h2>
                        <p className="mt-1 text-xs leading-5 text-muted-foreground">{descriptions[eventKey].detail}</p>
                      </div>
                      {([
                        ['emailEnabled', 'Email'],
                        ['inAppEnabled', 'In-app'],
                      ] as const).map(([channel, label]) => (
                        <label key={channel} className="flex min-h-10 items-center gap-2 text-sm sm:justify-center">
                          <input
                            type="checkbox"
                            checked={preference[channel]}
                            onChange={(event) => updatePreference(eventKey, channel, event.target.checked)}
                            aria-label={`${label} notifications: ${descriptions[eventKey].title}`}
                            className="size-4 rounded border-input accent-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          />
                          <span className="sm:hidden">{label}</span>
                        </label>
                      ))}
                    </section>
                  );
                })}
              </div>
              <div className="flex flex-col-reverse gap-3 border-t border-border/60 pt-5 sm:flex-row sm:items-center sm:justify-between">
                <div aria-live="polite" className="min-h-5 text-sm">
                  {saved && <span className="inline-flex items-center gap-1.5 text-primary"><Check aria-hidden="true" className="size-4" />Preferences saved</span>}
                  {!saved && !canSeeMembership && <span className="text-muted-foreground">Membership and role notices are limited to owners and admins.</span>}
                </div>
                <Button type="submit" disabled={!dirty || saving || Boolean(error)}>
                  {saving ? <LoaderCircle aria-hidden="true" className="me-2 size-4 animate-spin" /> : <Save aria-hidden="true" className="me-2 size-4" />}
                  {saving ? 'Saving…' : 'Save preferences'}
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      ) : !spacesError && !error && !selectedSpaceId ? (
        <Card><CardContent className="p-8 text-center text-sm text-muted-foreground">Choose a Space to manage its notification preferences.</CardContent></Card>
      ) : null}
    </SettingsLayout>
  );
}
