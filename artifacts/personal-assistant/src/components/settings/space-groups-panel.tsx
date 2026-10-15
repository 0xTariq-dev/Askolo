import { useEffect, useState, type FormEvent } from 'react';
import { LoaderCircle, Plus, Save, Trash2, UsersRound } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  deleteV2WorkspaceGroup,
  getV2WorkspaceGroups,
  saveV2WorkspaceGroup,
  type V2SpaceMember,
  type V2WorkspaceGroup,
} from '@/lib/agenda-v2-api';

const permissionOptions = [
  ['resource.update.any', 'Update tasks created by other members'],
  ['resource.delete.any', 'Archive tasks created by other members'],
] as const;

type Draft = { id?: string; name: string; description: string; members: string[]; permissions: string[] };
const emptyDraft = (): Draft => ({ name: '', description: '', members: [], permissions: [] });

export function SpaceGroupsPanel({ workspaceId, members, canManage }: {
  workspaceId: string;
  members: V2SpaceMember[];
  canManage: boolean;
}) {
  const [groups, setGroups] = useState<V2WorkspaceGroup[]>([]);
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [reload, setReload] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    getV2WorkspaceGroups(workspaceId, controller.signal)
      .then(setGroups)
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : 'Groups could not be loaded.');
      });
    return () => controller.abort();
  }, [workspaceId, reload]);

  const selectGroup = (group: V2WorkspaceGroup) => setDraft({
    id: group.id, name: group.name, description: group.description ?? '',
    members: group.memberUserIds, permissions: group.permissions,
  });

  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (!draft.name.trim() || busy || !canManage) return;
    setBusy(true); setError(''); setNotice('');
    try {
      await saveV2WorkspaceGroup(workspaceId, {
        name: draft.name.trim(),
        description: draft.description.trim() || null,
        memberUserIds: draft.members,
        permissions: draft.permissions,
      }, draft.id);
      setDraft(emptyDraft());
      setNotice('Group saved.');
      setReload((value) => value + 1);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Group could not be saved.');
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!draft.id || !window.confirm(`Delete ${draft.name}? Tasks assigned to this group may prevent deletion.`)) return;
    setBusy(true); setError(''); setNotice('');
    try {
      await deleteV2WorkspaceGroup(workspaceId, draft.id);
      setDraft(emptyDraft()); setNotice('Group deleted.'); setReload((value) => value + 1);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Group could not be deleted.');
    } finally {
      setBusy(false);
    }
  };

  const toggle = (key: 'members' | 'permissions', value: string) => {
    setDraft((current) => ({
      ...current,
      [key]: current[key].includes(value)
        ? current[key].filter((item) => item !== value)
        : [...current[key], value],
    }));
  };

  return (
    <Card>
      <CardHeader className="border-b border-border/60 bg-muted/20 px-5 py-5 sm:px-7">
        <div className="flex items-start gap-3">
          <span className="mt-0.5 grid size-9 shrink-0 place-items-center rounded-xl bg-background text-primary shadow-sm"><UsersRound aria-hidden="true" className="size-4" /></span>
          <div>
            <CardTitle className="text-lg">Groups and permissions</CardTitle>
            <CardDescription className="mt-1">Group permissions add to each member’s existing direct access. Removing a member takes effect immediately.</CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="grid gap-6 px-5 py-5 sm:px-7 md:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)]">
        <div className="space-y-2">
          {groups.map((group) => (
            <button key={group.id} type="button" onClick={() => selectGroup(group)}
              className={`min-h-11 w-full rounded-xl border px-3 py-2 text-start focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${draft.id === group.id ? 'border-primary bg-primary/5' : 'border-border hover:bg-muted/40'}`}>
              <span className="block font-medium">{group.name}</span>
              <span className="text-xs text-muted-foreground">{group.memberUserIds.length} members · {group.permissions.length} permissions</span>
            </button>
          ))}
          {groups.length === 0 && <p className="py-4 text-sm text-muted-foreground">No groups yet.</p>}
          {canManage && <Button type="button" variant="outline" className="w-full" onClick={() => setDraft(emptyDraft())}><Plus aria-hidden="true" className="me-2 size-4" />New group</Button>}
        </div>
        {canManage ? (
          <form onSubmit={(event) => void save(event)} className="space-y-5">
            <div className="space-y-2"><Label htmlFor="space-group-name">Group name</Label><Input id="space-group-name" required maxLength={80} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} /></div>
            <div className="space-y-2"><Label htmlFor="space-group-description">Description</Label><Input id="space-group-description" maxLength={500} value={draft.description} onChange={(event) => setDraft({ ...draft, description: event.target.value })} /></div>
            <fieldset className="space-y-2">
              <legend className="text-sm font-medium">Active members</legend>
              <div className="grid max-h-48 gap-2 overflow-y-auto rounded-xl border border-border p-3 sm:grid-cols-2">
                {members.map((member) => (
                  <label key={member.userId} className="flex min-h-9 items-center gap-2 text-sm">
                    <input type="checkbox" checked={draft.members.includes(member.userId)} onChange={() => toggle('members', member.userId)} />
                    <span className="truncate">{member.name || member.email || 'Space member'}</span>
                  </label>
                ))}
              </div>
            </fieldset>
            <fieldset className="space-y-2">
              <legend className="text-sm font-medium">Additional permissions</legend>
              <div className="grid gap-2 sm:grid-cols-2">
                {permissionOptions.map(([key, label]) => (
                  <label key={key} className="flex min-h-9 items-center gap-2 text-sm">
                    <input type="checkbox" checked={draft.permissions.includes(key)} onChange={() => toggle('permissions', key)} />
                    {label}
                  </label>
                ))}
              </div>
            </fieldset>
            {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
            {notice && <p role="status" className="text-sm text-muted-foreground">{notice}</p>}
            <div className="flex flex-wrap gap-2">
              <Button type="submit" disabled={busy || !draft.name.trim()}><Save aria-hidden="true" className="me-2 size-4" />{busy ? <LoaderCircle aria-hidden="true" className="size-4 animate-spin" /> : 'Save group'}</Button>
              {draft.id && <Button type="button" variant="destructive" disabled={busy} onClick={() => void remove()}><Trash2 aria-hidden="true" className="me-2 size-4" />Delete</Button>}
            </div>
          </form>
        ) : <p className="text-sm text-muted-foreground">Only Space owners and admins can edit groups.</p>}
      </CardContent>
    </Card>
  );
}
