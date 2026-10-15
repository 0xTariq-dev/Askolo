import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { Link } from 'wouter';
import {
  AlertCircle,
  ArrowUpRight,
  Check,
  ChevronDown,
  Clock3,
  Globe2,
  LoaderCircle,
  LockKeyhole,
  RotateCw,
  Save,
  UsersRound,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { PageTransition } from '@/components/ui/page-transition';
import { SpaceSettingsNavigation } from '@/components/settings/space-settings-navigation';
import { useSpaceScope } from '@/contexts/space-context';
import {
  getV2WorkspaceSettings,
  updateV2WorkspaceSettings,
  type V2Workspace,
} from '@/lib/agenda-v2-api';

type Draft = {
  name: string;
  description: string;
  slug: string;
  timeZone: string;
};

const commonTimeZones = [
  'Etc/UTC',
  'America/Los_Angeles',
  'America/Denver',
  'America/Chicago',
  'America/New_York',
  'America/Toronto',
  'America/Sao_Paulo',
  'Europe/London',
  'Europe/Paris',
  'Europe/Berlin',
  'Europe/Istanbul',
  'Africa/Cairo',
  'Asia/Dubai',
  'Asia/Kolkata',
  'Asia/Singapore',
  'Asia/Tokyo',
  'Australia/Sydney',
];

function draftFrom(workspace: V2Workspace): Draft {
  return {
    name: workspace.name,
    description: workspace.description ?? '',
    slug: workspace.slug ?? '',
    timeZone: workspace.timeZone || 'Etc/UTC',
  };
}

function isEditorRole(role: string): boolean {
  return ['owner', 'admin'].includes(role.trim().toLowerCase());
}

function roleLabel(role: string): string {
  const normalized = role.trim().toLowerCase();
  if (normalized === 'owner') return 'Owner';
  if (normalized === 'admin') return 'Admin';
  if (normalized === 'member') return 'Member';
  return role ? role[0].toUpperCase() + role.slice(1) : 'Member';
}

export function SpaceDetailsPage() {
  const { spaces, selectedSpaceId, setSelectedSpaceId, loading: spacesLoading, error: spacesError } =
    useSpaceScope();
  const [workspace, setWorkspace] = useState<V2Workspace | null>(null);
  const [loadedForSpaceId, setLoadedForSpaceId] = useState<string | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [requestKey, setRequestKey] = useState(0);

  useEffect(() => {
    if (!selectedSpaceId || spacesLoading) {
      setWorkspace(null);
      setLoadedForSpaceId(null);
      setDraft(null);
      return;
    }
    const controller = new AbortController();
    setLoading(true);
    setLoadedForSpaceId(null);
    setLoadError(null);
    setSaveError(null);
    setSaved(false);
    getV2WorkspaceSettings(selectedSpaceId, controller.signal)
      .then((result) => {
        if (controller.signal.aborted) return;
        setWorkspace(result);
        setLoadedForSpaceId(selectedSpaceId);
        setDraft(draftFrom(result));
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        setLoadError(error instanceof Error ? error.message : 'Space details could not be loaded.');
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [selectedSpaceId, spacesLoading, requestKey]);

  const timeZones = useMemo(() => {
    const browserZone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    const choices = new Set([...commonTimeZones, browserZone, workspace?.timeZone || '']);
    return [...choices].filter(Boolean).sort((a, b) => {
      if (a === workspace?.timeZone) return -1;
      if (b === workspace?.timeZone) return 1;
      return a.localeCompare(b);
    });
  }, [workspace?.timeZone]);

  const canEdit = Boolean(workspace && isEditorRole(workspace.role));
  const dirty = Boolean(workspace && draft && (
    draft.name !== workspace.name
    || draft.description !== (workspace.description ?? '')
    || draft.slug !== (workspace.slug ?? '')
    || draft.timeZone !== workspace.timeZone
  ));

  const save = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!workspace || !draft || !canEdit || !draft.name.trim()) return;
    setSaving(true);
    setSaveError(null);
    setSaved(false);
    try {
      const updated = await updateV2WorkspaceSettings(workspace.id, {
        name: draft.name.trim(),
        description: draft.description.trim() || null,
        slug: draft.slug.trim() || null,
        timeZone: draft.timeZone,
        imageObjectPath: workspace.imageObjectPath,
      });
      setWorkspace(updated);
      setDraft(draftFrom(updated));
      setSaved(true);
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Space details could not be saved.');
    } finally {
      setSaving(false);
    }
  };

  const resetDraft = () => {
    if (workspace) setDraft(draftFrom(workspace));
    setSaveError(null);
    setSaved(false);
  };

  const hasSpaces = spaces.length > 0;

  return (
    <PageTransition surface={false} className="mx-auto max-w-5xl space-y-7 pb-10">
      <header className="relative overflow-hidden rounded-3xl border border-border/70 bg-card/45 px-5 py-7 shadow-sm sm:px-8 sm:py-9">
        <div aria-hidden="true" className="absolute -end-12 -top-20 size-64 rounded-full border-[36px] border-primary/[0.06]" />
        <div className="relative">
          <div className="mb-5 flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.19em] text-primary">
            <span className="grid size-8 place-items-center rounded-xl bg-primary/10"><UsersRound className="size-4" /></span>
            Shared Space
          </div>
          <div className="flex flex-col justify-between gap-5 sm:flex-row sm:items-end">
            <div>
              <h1 className="font-display text-3xl font-bold tracking-tight sm:text-4xl">Space details</h1>
              <p className="mt-2 max-w-xl text-sm leading-6 text-muted-foreground">
                Set the identity and local time used by everyone in this Space.
              </p>
            </div>
            <div className="w-full sm:w-64">
              <Label htmlFor="space-picker" className="mb-1.5 block text-xs text-muted-foreground">Selected Space</Label>
              <div className="relative">
                <select
                  id="space-picker"
                  value={selectedSpaceId ?? ''}
                  onChange={(event) => setSelectedSpaceId(event.target.value || null)}
                  disabled={spacesLoading || !hasSpaces}
                  className="h-11 w-full appearance-none rounded-xl border border-border bg-background/80 px-3 pe-10 text-sm font-medium outline-none transition focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-60"
                >
                  {!hasSpaces && <option value="">No Spaces available</option>}
                  {spaces.map((space) => (
                    <option key={space.id} value={space.id}>{space.name}{space.isPersonal ? ' · Personal' : ''}</option>
                  ))}
                </select>
                <ChevronDown aria-hidden="true" className="pointer-events-none absolute end-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              </div>
            </div>
          </div>
        </div>
      </header>

      <SpaceSettingsNavigation active="details" />

      {spacesError && (
        <div role="alert" className="flex items-start gap-3 rounded-xl border border-destructive/25 bg-destructive/5 p-4 text-sm">
          <AlertCircle className="mt-0.5 size-4 shrink-0 text-destructive" />
          <div><p className="font-medium">Spaces could not be loaded</p><p className="mt-1 text-muted-foreground">{spacesError}</p></div>
        </div>
      )}

      {!spacesLoading && !spacesError && !hasSpaces && (
        <Card className="border-dashed">
          <CardContent className="flex flex-col items-center px-6 py-14 text-center">
            <span className="mb-4 grid size-12 place-items-center rounded-2xl bg-muted text-muted-foreground"><UsersRound className="size-5" /></span>
            <h2 className="font-display text-lg font-semibold">No Space to manage yet</h2>
            <p className="mt-1 max-w-sm text-sm text-muted-foreground">Your available Spaces will appear here once they are ready.</p>
          </CardContent>
        </Card>
      )}

      {spacesLoading && (
        <div role="status" aria-label="Loading Spaces" className="space-y-4">
          <div className="h-28 animate-pulse rounded-2xl bg-muted/70" />
          <div className="h-72 animate-pulse rounded-2xl bg-muted/70" />
        </div>
      )}

      {selectedSpaceId && !spacesLoading && (
        loading ? (
          <div role="status" aria-label="Loading Space details" className="space-y-4">
            <div className="h-28 animate-pulse rounded-2xl bg-muted/70" />
            <div className="h-72 animate-pulse rounded-2xl bg-muted/70" />
          </div>
        ) : loadError ? (
          <Card role="alert">
            <CardHeader>
              <CardTitle className="flex items-center gap-2"><AlertCircle className="size-5 text-destructive" />Details unavailable</CardTitle>
              <CardDescription>{loadError}</CardDescription>
            </CardHeader>
            <CardContent><Button variant="outline" onClick={() => setRequestKey((key) => key + 1)}><RotateCw className="me-2 size-4" />Try again</Button></CardContent>
          </Card>
        ) : workspace && draft && loadedForSpaceId === selectedSpaceId ? (
          <>
            <section className="grid gap-4 sm:grid-cols-[1fr_auto]">
              <div className="flex min-w-0 items-center gap-4 rounded-2xl border border-border/70 bg-card/45 p-4 sm:p-5">
                <div className="grid size-12 shrink-0 place-items-center rounded-2xl bg-primary/10 text-primary">
                  {workspace.isPersonal ? <LockKeyhole className="size-5" /> : <UsersRound className="size-5" />}
                </div>
                <div className="min-w-0">
                  <p className="truncate font-display text-lg font-semibold">{workspace.name}</p>
                  <p className="mt-0.5 text-sm text-muted-foreground">
                    {workspace.isPersonal ? 'Personal Space' : 'Shared with your Space members'}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-3 rounded-2xl border border-border/70 bg-card/45 px-4 py-3 sm:min-w-40 sm:justify-center">
                <span className={`size-2 rounded-full ${canEdit ? 'bg-primary' : 'bg-muted-foreground/50'}`} />
                <div><p className="text-xs text-muted-foreground">Your role</p><p className="text-sm font-semibold">{roleLabel(workspace.role)}</p></div>
              </div>
            </section>

            <Card className="overflow-hidden">
              <CardHeader className="border-b border-border/60 bg-muted/20 px-5 py-5 sm:px-7">
                <div className="flex items-start gap-3">
                  <span className="mt-0.5 grid size-9 shrink-0 place-items-center rounded-xl bg-background text-primary shadow-sm"><Globe2 className="size-4" /></span>
                  <div>
                    <CardTitle className="text-lg">Identity &amp; locale</CardTitle>
                    <CardDescription className="mt-1">
                      {canEdit ? 'Changes are shared with everyone who belongs to this Space.' : 'You can view these details. Ask an owner or admin to make changes.'}
                    </CardDescription>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="px-5 py-6 sm:px-7">
                <form onSubmit={save} className="space-y-6">
                  <fieldset disabled={!canEdit || saving} className="space-y-6 disabled:opacity-80">
                    <div className="grid gap-5 md:grid-cols-2">
                      <div className="space-y-2">
                        <Label htmlFor="space-name">Space name <span className="text-destructive" aria-hidden="true">*</span></Label>
                        <Input id="space-name" required maxLength={100} value={draft.name} onChange={(event) => { setDraft({ ...draft, name: event.target.value }); setSaved(false); }} aria-describedby="space-name-help" />
                        <p id="space-name-help" className="text-xs text-muted-foreground">A clear name helps members recognize where they are.</p>
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="space-slug">Custom link</Label>
                        <div className="flex h-10 items-center overflow-hidden rounded-xl border border-input bg-background/70 focus-within:ring-2 focus-within:ring-ring">
                          <span className="hidden border-e border-border px-3 text-xs text-muted-foreground sm:block">/s/</span>
                          <Input id="space-slug" value={draft.slug} onChange={(event) => { setDraft({ ...draft, slug: event.target.value }); setSaved(false); }} placeholder="your-space" className="h-full rounded-none border-0 bg-transparent focus-visible:ring-0" aria-describedby="space-slug-help" />
                        </div>
                        <p id="space-slug-help" className="text-xs text-muted-foreground">Leave empty if this Space does not need a custom link.</p>
                      </div>
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="space-description">Description <span className="text-xs font-normal text-muted-foreground">(optional)</span></Label>
                      <textarea
                        id="space-description"
                        value={draft.description}
                        onChange={(event) => { setDraft({ ...draft, description: event.target.value }); setSaved(false); }}
                        rows={3}
                        maxLength={500}
                        placeholder="What is this Space for?"
                        className="flex w-full resize-y rounded-xl border border-input bg-background/70 px-3 py-2.5 text-sm outline-none transition placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed"
                        aria-describedby="space-description-help"
                      />
                      <p id="space-description-help" className="text-xs text-muted-foreground">A short note about what belongs here. Up to 500 characters.</p>
                    </div>

                    <div className="max-w-xl space-y-2">
                      <Label htmlFor="space-time-zone" className="flex items-center gap-2"><Clock3 className="size-4 text-muted-foreground" />Time zone</Label>
                      <div className="relative">
                        <select
                          id="space-time-zone"
                          value={draft.timeZone}
                          onChange={(event) => { setDraft({ ...draft, timeZone: event.target.value }); setSaved(false); }}
                          className="h-11 w-full appearance-none rounded-xl border border-input bg-background/70 px-3 pe-10 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        >
                          {timeZones.map((zone) => <option key={zone} value={zone}>{zone.replaceAll('_', ' ')}</option>)}
                        </select>
                        <ChevronDown aria-hidden="true" className="pointer-events-none absolute end-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                      </div>
                      <p className="text-xs text-muted-foreground">Dates and scheduled routines use this local time.</p>
                    </div>
                  </fieldset>

                  <div className="flex flex-col-reverse gap-3 border-t border-border/60 pt-5 sm:flex-row sm:items-center sm:justify-between">
                    <div aria-live="polite" className="min-h-5 text-sm">
                      {saveError && <span role="alert" className="text-destructive">{saveError}</span>}
                      {!saveError && saved && <span className="inline-flex items-center gap-1.5 text-primary"><Check className="size-4" />Changes saved</span>}
                      {!saveError && !saved && !canEdit && <span className="text-muted-foreground">View only · managed by Space owners and admins</span>}
                    </div>
                    {canEdit && (
                      <div className="flex gap-2 sm:justify-end">
                        <Button type="button" variant="ghost" onClick={resetDraft} disabled={!dirty || saving}>Discard</Button>
                        <Button type="submit" disabled={!dirty || saving || !draft.name.trim()} className="min-w-32">
                          {saving ? <LoaderCircle className="me-2 size-4 animate-spin" /> : <Save className="me-2 size-4" />}
                          {saving ? 'Saving…' : 'Save changes'}
                        </Button>
                      </div>
                    )}
                  </div>
                </form>
              </CardContent>
            </Card>

            <aside className="flex flex-col gap-3 rounded-2xl border border-border/60 bg-muted/25 p-4 sm:flex-row sm:items-center sm:justify-between sm:px-5">
              <div className="flex items-start gap-3">
                <span className="mt-0.5 grid size-8 shrink-0 place-items-center rounded-lg bg-background text-muted-foreground"><LockKeyhole className="size-4" /></span>
                <div><p className="text-sm font-medium">Separate from your account</p><p className="mt-0.5 text-xs leading-5 text-muted-foreground">Space details are shared settings. Your profile, security, usage and billing stay personal to your Askolo account.</p></div>
              </div>
              <Link href="/settings/account/profile" className="inline-flex min-h-10 shrink-0 items-center gap-2 rounded-lg px-3 text-sm font-medium text-primary hover:bg-primary/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                Account settings <ArrowUpRight className="size-4" />
              </Link>
            </aside>
          </>
        ) : null
      )}
    </PageTransition>
  );
}
