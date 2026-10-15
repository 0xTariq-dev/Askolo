import { useEffect, useState, type FormEvent } from 'react';
import { Archive, AlertCircle, Clock3, LoaderCircle, LogOut, Mail, Send, ShieldCheck, UserPlus, UsersRound } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { PageTransition } from '@/components/ui/page-transition';
import { SpaceSettingsNavigation } from '@/components/settings/space-settings-navigation';
import { useSpaceScope } from '@/contexts/space-context';
import {
  cancelV2WorkspaceInvitation,
  createV2WorkspaceInvitation,
  deleteV2Workspace,
  getV2SpaceMembers,
  getV2WorkspaceInvitations,
  leaveV2Workspace,
  updateV2SpaceMemberRole,
  type V2SpaceMember,
  type V2WorkspaceInvitation,
} from '@/lib/agenda-v2-api';

function roleLabel(role: string) {
  return role === 'owner' ? 'Owner' : role === 'admin' ? 'Admin' : 'Member';
}

export function SpaceMembersPage() {
  const {
    spaces,
    selectedSpaceId,
    setSelectedSpaceId,
    refreshSpaces,
    loading: spacesLoading,
    error: spacesError,
  } = useSpaceScope();
  const selectedSpace = spaces.find((space) => space.id === selectedSpaceId);
  const selectedRole = selectedSpace?.role.trim().toLowerCase() ?? '';
  const canManageRoles = selectedRole === 'owner';
  const canViewEmails = ['owner', 'admin'].includes(selectedRole);
  const canInvite = canViewEmails;
  const canInviteAdmins = selectedRole === 'owner';
  const [members, setMembers] = useState<V2SpaceMember[]>([]);
  const [loadedForSpaceId, setLoadedForSpaceId] = useState<string | null>(null);
  const [invitations, setInvitations] = useState<V2WorkspaceInvitation[]>([]);
  const [invitationsLoadedForSpaceId, setInvitationsLoadedForSpaceId] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [invitationsLoading, setInvitationsLoading] = useState(false);
  const [savingUserId, setSavingUserId] = useState<string | null>(null);
  const [cancellingInvitationId, setCancellingInvitationId] = useState<string | null>(null);
  const [creatingInvitation, setCreatingInvitation] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [invitationError, setInvitationError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [invitationNotice, setInvitationNotice] = useState<string | null>(null);
  const [spaceAction, setSpaceAction] = useState<'leave' | 'delete' | null>(null);
  const [spaceActionError, setSpaceActionError] = useState<string | null>(null);
  const [invitationEmail, setInvitationEmail] = useState('');
  const [invitationRole, setInvitationRole] = useState<'admin' | 'member'>('member');
  const [invitationDraftSpaceId, setInvitationDraftSpaceId] = useState<string | null>(null);
  const [loadAttempt, setLoadAttempt] = useState(0);
  const [invitationLoadAttempt, setInvitationLoadAttempt] = useState(0);

  useEffect(() => {
    setInvitationEmail('');
    setInvitationRole('member');
    setInvitationError(null);
    setInvitationNotice(null);
    setInvitationDraftSpaceId(selectedSpaceId);
  }, [selectedSpaceId]);

  useEffect(() => {
    if (!selectedSpaceId || spacesLoading) {
      setMembers([]);
      setLoadedForSpaceId(null);
      return;
    }
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    setLoadedForSpaceId(null);
    getV2SpaceMembers(selectedSpaceId, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) {
          setMembers(result);
          setLoadedForSpaceId(selectedSpaceId);
        }
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) {
          setError(reason instanceof Error ? reason.message : 'Space members could not be loaded.');
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [loadAttempt, selectedSpaceId, spacesLoading]);

  useEffect(() => {
    if (!selectedSpaceId || spacesLoading) {
      setInvitations([]);
      setInvitationsLoadedForSpaceId(null);
      return;
    }
    const controller = new AbortController();
    setInvitationsLoading(true);
    setInvitationError(null);
    setInvitationsLoadedForSpaceId(null);
    getV2WorkspaceInvitations(selectedSpaceId, controller.signal)
      .then((result) => {
        if (controller.signal.aborted) return;
        setInvitations(result);
        setInvitationsLoadedForSpaceId(selectedSpaceId);
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) {
          setInvitationError(reason instanceof Error ? reason.message : 'Space invitations could not be loaded.');
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setInvitationsLoading(false);
      });
    return () => controller.abort();
  }, [invitationLoadAttempt, selectedSpaceId, spacesLoading]);

  const changeRole = async (member: V2SpaceMember, role: 'admin' | 'member') => {
    if (!selectedSpaceId || !canManageRoles || role === member.role) return;
    setSavingUserId(member.userId);
    setError(null);
    setNotice(null);
    try {
      await updateV2SpaceMemberRole(selectedSpaceId, member.userId, role);
      setMembers((current) => current.map((candidate) =>
        candidate.userId === member.userId ? { ...candidate, role } : candidate,
      ));
      setNotice(`${member.name || member.email || 'Member'} is now a ${roleLabel(role).toLowerCase()}.`);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Member role could not be changed.');
    } finally {
      setSavingUserId(null);
    }
  };

  const leaveOrDeleteSpace = async () => {
    if (!selectedSpaceId || !selectedSpace || selectedSpace.isPersonal || spaceAction) return;
    const deleting = selectedRole === 'owner';
    const confirmed = window.confirm(
      deleting
        ? `Delete ${selectedSpace.name}? This archives the Space, revokes access for every member, cancels pending invitations, and retains its records.`
        : `Leave ${selectedSpace.name}? You will lose access to its tasks and shared data.`,
    );
    if (!confirmed) return;

    setSpaceAction(deleting ? 'delete' : 'leave');
    setSpaceActionError(null);
    let completed = false;
    try {
      if (deleting) await deleteV2Workspace(selectedSpaceId);
      else await leaveV2Workspace(selectedSpaceId);
      completed = true;
      setSelectedSpaceId(null);
      const remainingSpaces = await refreshSpaces();
      setSelectedSpaceId(remainingSpaces[0]?.id ?? null);
    } catch (reason) {
      setSpaceActionError(completed
        ? 'The action succeeded, but your Space list could not refresh. Reload Askolo to continue.'
        : reason instanceof Error ? reason.message : 'The Space action could not be completed.');
    } finally {
      setSpaceAction(null);
    }
  };

  const createInvitation = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const email = invitationEmail.trim();
    if (!selectedSpaceId || invitationDraftSpaceId !== selectedSpaceId || !canInvite || !email || (invitationRole === 'admin' && !canInviteAdmins)) return;
    setCreatingInvitation(true);
    setInvitationError(null);
    setInvitationNotice(null);
    try {
      const created = await createV2WorkspaceInvitation(selectedSpaceId, email, invitationRole);
      setInvitations((current) => [
        created,
        ...current.filter((invitation) => invitation.id !== created.id),
      ]);
      setInvitationEmail('');
      setInvitationRole('member');
      setInvitationNotice('Invitation sent. The pending invitation is listed below.');
    } catch (reason) {
      setInvitationError(reason instanceof Error ? reason.message : 'Invitation could not be sent.');
    } finally {
      setCreatingInvitation(false);
    }
  };

  const cancelInvitation = async (invitation: V2WorkspaceInvitation) => {
    if (!selectedSpaceId || !canInvite || cancellingInvitationId) return;
    if (!window.confirm('Cancel this pending invitation? The invite link will no longer work.')) return;
    setCancellingInvitationId(invitation.id);
    setInvitationError(null);
    setInvitationNotice(null);
    try {
      await cancelV2WorkspaceInvitation(selectedSpaceId, invitation.id);
      setInvitations((current) => current.filter((item) => item.id !== invitation.id));
      setInvitationNotice('Pending invitation cancelled.');
    } catch (reason) {
      setInvitationError(reason instanceof Error ? reason.message : 'Invitation could not be cancelled.');
    } finally {
      setCancellingInvitationId(null);
    }
  };

  return (
    <PageTransition surface={false} className="mx-auto max-w-5xl space-y-7 pb-10">
      <header className="relative overflow-hidden rounded-3xl border border-border/70 bg-card/45 px-5 py-7 shadow-sm sm:px-8 sm:py-9">
        <div aria-hidden="true" className="absolute -end-10 -top-16 size-52 rounded-full border-[30px] border-primary/[0.06]" />
        <div className="relative">
          <div className="mb-5 flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.19em] text-primary">
            <span className="grid size-8 place-items-center rounded-xl bg-primary/10"><UsersRound aria-hidden="true" className="size-4" /></span>
            Shared Space
          </div>
          <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
            <div>
              <h1 className="font-display text-3xl font-bold tracking-tight sm:text-4xl">People and access</h1>
              <p className="mt-2 max-w-xl text-sm leading-6 text-muted-foreground">
                Review who belongs here and keep role changes in the owner’s hands.
              </p>
            </div>
            {selectedSpace && (
              <div className="inline-flex max-w-full items-center gap-2 self-start rounded-xl border border-border/70 bg-background/70 px-3 py-2 sm:self-auto">
                <span className="grid size-7 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary"><UsersRound aria-hidden="true" className="size-3.5" /></span>
                <span className="max-w-56 truncate text-sm font-medium">{selectedSpace.name}</span>
                <span className="rounded-full bg-muted px-2 py-1 text-[11px] font-semibold text-muted-foreground">{roleLabel(selectedSpace.role)}</span>
              </div>
            )}
          </div>
        </div>
      </header>

      <SpaceSettingsNavigation active="members" />

      {(spacesError || error) && (
        <div role="alert" className="flex items-start justify-between gap-3 rounded-xl border border-destructive/25 bg-destructive/5 p-4 text-sm">
          <div className="flex min-w-0 items-start gap-3">
            <AlertCircle aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-destructive" />
            <div><p className="font-medium">Member list unavailable</p><p className="mt-1 text-muted-foreground">{spacesError ?? error}</p></div>
          </div>
          {!spacesError && !loading && <Button type="button" variant="outline" size="sm" onClick={() => setLoadAttempt((attempt) => attempt + 1)}>Try again</Button>}
        </div>
      )}

      {notice && <p role="status" aria-live="polite" className="text-sm text-primary">{notice}</p>}

      {selectedSpaceId && !spacesLoading && (
        <Card>
          <CardHeader className="border-b border-border/60 bg-muted/20 px-5 py-5 sm:px-7">
            <div className="flex items-start gap-3">
              <span className="mt-0.5 grid size-9 shrink-0 place-items-center rounded-xl bg-background text-primary shadow-sm">
                <Mail aria-hidden="true" className="size-4" />
              </span>
              <div>
                <CardTitle className="text-lg">Invitations</CardTitle>
                <CardDescription className="mt-1">
                  {selectedRole === 'owner'
                    ? 'Invite people as members or admins. Owners and admins can cancel pending invitations.'
                    : selectedRole === 'admin'
                      ? 'You can invite members and cancel pending invitations. Only owners can invite admins.'
                      : 'Review pending invitations. Recipient email addresses are visible only to owners and admins.'}
                </CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent className="space-y-5 px-5 py-5 sm:px-7">
            {canInvite && invitationDraftSpaceId === selectedSpaceId && (
              <form onSubmit={createInvitation} className="grid gap-4 rounded-2xl border border-border/70 bg-background/55 p-4 sm:grid-cols-[minmax(0,1fr)_10rem_auto] sm:items-end">
                <div className="space-y-2">
                  <Label htmlFor="invite-email">Email address</Label>
                  <Input
                    id="invite-email"
                    type="email"
                    autoComplete="email"
                    required
                    maxLength={254}
                    value={invitationEmail}
                    onChange={(event) => {
                      setInvitationEmail(event.target.value);
                      setInvitationError(null);
                      setInvitationNotice(null);
                    }}
                    placeholder="name@example.com"
                    aria-describedby="invite-email-help"
                  />
                  <p id="invite-email-help" className="text-xs text-muted-foreground">The invitation is tied to this email address.</p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="invite-role">Invite as</Label>
                  <select
                    id="invite-role"
                    value={canInviteAdmins ? invitationRole : 'member'}
                    disabled={!canInviteAdmins || creatingInvitation}
                    onChange={(event) => {
                      if (event.target.value === 'admin' || event.target.value === 'member') {
                        setInvitationRole(event.target.value);
                      }
                    }}
                    className="h-10 w-full rounded-xl border border-input bg-background px-3 text-sm font-medium text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-70"
                  >
                    <option value="member">Member</option>
                    {canInviteAdmins && <option value="admin">Admin</option>}
                  </select>
                  {!canInviteAdmins && <p className="text-xs text-muted-foreground">Admin invitations are owner-only.</p>}
                </div>
                <Button type="submit" disabled={creatingInvitation || !invitationEmail.trim()} className="min-h-10">
                  {creatingInvitation ? <LoaderCircle aria-hidden="true" className="me-2 size-4 animate-spin" /> : <Send aria-hidden="true" className="me-2 size-4" />}
                  {creatingInvitation ? 'Sending…' : 'Send invite'}
                </Button>
              </form>
            )}

            {invitationError && (
              <div role="alert" className="flex flex-col gap-3 rounded-xl border border-destructive/25 bg-destructive/5 p-3 text-sm sm:flex-row sm:items-center sm:justify-between">
                <span className="text-destructive">{invitationError}</span>
                {invitationsLoadedForSpaceId !== selectedSpaceId && !invitationsLoading && (
                  <Button type="button" variant="outline" size="sm" onClick={() => setInvitationLoadAttempt((attempt) => attempt + 1)}>
                    Try loading again
                  </Button>
                )}
              </div>
            )}
            {invitationNotice && <p role="status" aria-live="polite" className="text-sm text-primary">{invitationNotice}</p>}

            <div className="space-y-3">
              <div className="flex items-center justify-between gap-3">
                <h2 className="text-sm font-semibold">Pending invitations</h2>
                {invitationsLoadedForSpaceId === selectedSpaceId && (
                  <span className="rounded-full bg-muted px-2.5 py-1 text-xs font-medium text-muted-foreground">
                    {invitations.length}
                  </span>
                )}
              </div>
              {invitationsLoading ? (
                <div role="status" aria-label="Loading pending invitations" className="space-y-2">
                  <div className="h-16 animate-pulse rounded-xl bg-muted/60" />
                  <div className="h-16 animate-pulse rounded-xl bg-muted/60" />
                </div>
              ) : invitationsLoadedForSpaceId === selectedSpaceId && !invitationError ? (
                invitations.length ? (
                  <ul className="space-y-2">
                    {invitations.map((invitation) => (
                      <li key={invitation.id} className="flex flex-col gap-3 rounded-xl border border-border/70 bg-background/50 p-4 sm:flex-row sm:items-center sm:justify-between">
                        <div className="flex min-w-0 items-start gap-3">
                          <span className="mt-0.5 grid size-8 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary">
                            <UserPlus aria-hidden="true" className="size-4" />
                          </span>
                          <div className="min-w-0">
                            <p className="truncate text-sm font-semibold">
                              {canViewEmails ? invitation.email : 'Pending invitation'}
                            </p>
                            <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                              <span>Invited as {invitation.role}</span>
                              <span className="inline-flex items-center gap-1">
                                <Clock3 aria-hidden="true" className="size-3.5" />
                                Expires {new Date(invitation.expiresAt).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })}
                              </span>
                            </div>
                          </div>
                        </div>
                        {canInvite && (
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            disabled={Boolean(cancellingInvitationId)}
                            aria-label={`Cancel pending invitation${canViewEmails ? ` to ${invitation.email}` : ''}`}
                            onClick={() => void cancelInvitation(invitation)}
                          >
                            {cancellingInvitationId === invitation.id
                              ? <LoaderCircle aria-hidden="true" className="me-2 size-4 animate-spin" />
                              : <AlertCircle aria-hidden="true" className="me-2 size-4" />}
                            {cancellingInvitationId === invitation.id ? 'Cancelling…' : 'Cancel invite'}
                          </Button>
                        )}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <div className="rounded-xl border border-dashed border-border px-4 py-7 text-center">
                    <p className="text-sm font-medium">No pending invitations</p>
                    <p className="mt-1 text-xs text-muted-foreground">New invitations will appear here until they are accepted or expire.</p>
                  </div>
                )
              ) : !invitationError ? (
                <div role="status" aria-label="Loading pending invitations" className="h-16 animate-pulse rounded-xl bg-muted/60" />
              ) : null}
            </div>
          </CardContent>
        </Card>
      )}

      {loading || spacesLoading ? (
        <div role="status" aria-label="Loading Space members" className="flex items-center gap-3 rounded-2xl border border-border bg-card p-6 text-sm text-muted-foreground">
          <LoaderCircle aria-hidden="true" className="size-4 animate-spin" />Loading members…
        </div>
      ) : selectedSpaceId && loadedForSpaceId === selectedSpaceId && !error && (
        <Card>
          <CardHeader className="border-b border-border/60 bg-muted/20 px-5 py-5 sm:px-7">
            <div className="flex items-start gap-3">
              <span className="mt-0.5 grid size-9 shrink-0 place-items-center rounded-xl bg-background text-primary shadow-sm"><ShieldCheck aria-hidden="true" className="size-4" /></span>
              <div>
                <CardTitle className="text-lg">Space members</CardTitle>
                <CardDescription className="mt-1">
                  {canManageRoles
                    ? 'Only the owner can change roles. Email addresses are visible to owners and admins.'
                    : canViewEmails
                      ? 'You can review members and their contact emails. Role changes are reserved for the owner.'
                      : 'You can review member names and roles. Email addresses are visible only to owners and admins.'}
                </CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent className="space-y-3 px-5 py-5 sm:px-7">
            {members.length === 0 ? (
              <p className="py-8 text-center text-sm text-muted-foreground">No active members are listed for this Space.</p>
            ) : members.map((member) => (
                <div key={member.userId} className="flex flex-col gap-3 rounded-2xl border border-border/70 bg-background/50 p-4 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex min-w-0 items-center gap-3">
                  <span aria-hidden="true" className="grid size-10 shrink-0 place-items-center rounded-xl bg-primary/10 text-sm font-semibold text-primary">
                    {(member.name?.trim().charAt(0) || 'S').toUpperCase()}
                  </span>
                  <div className="min-w-0">
                    <p className="truncate text-sm font-semibold">{member.name?.trim() || 'Space member'}</p>
                    {canViewEmails && member.email && <p className="mt-1 truncate text-xs text-muted-foreground">{member.email}</p>}
                    {member.joinedAt && (
                      <p className="mt-1 text-xs text-muted-foreground">
                        Joined {new Date(member.joinedAt).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })}
                      </p>
                    )}
                  </div>
                </div>
                {canManageRoles && member.role !== 'owner' ? (
                  <label className="flex items-center gap-3 text-xs font-medium text-muted-foreground">
                    Role
                    <select
                      aria-label={`Role for ${member.name || member.email || 'Space member'}`}
                      value={member.role}
                      disabled={savingUserId === member.userId}
                      onChange={(event) => {
                        if (event.target.value === 'admin' || event.target.value === 'member') {
                          void changeRole(member, event.target.value);
                        }
                      }}
                      className="h-10 min-w-32 rounded-xl border border-input bg-background px-3 text-sm font-medium text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-60"
                    >
                      <option value="member">Member</option>
                      <option value="admin">Admin</option>
                    </select>
                    {savingUserId === member.userId && <LoaderCircle aria-hidden="true" className="size-4 animate-spin" />}
                  </label>
                ) : (
                  <span className="inline-flex min-h-9 items-center rounded-full bg-muted px-3 text-xs font-semibold">{roleLabel(member.role)}</span>
                )}
              </div>
            ))}
          </CardContent>
        </Card>
      )}
      {selectedSpace && !selectedSpace.isPersonal && (
        <Card className="border-destructive/20">
          <CardHeader>
            <CardTitle className="text-base">Space access</CardTitle>
            <CardDescription>
              {selectedRole === 'owner'
                ? 'Deleting a shared Space archives its records, cancels pending invitations, and revokes member access.'
                : 'Leaving a shared Space removes your access. You can be invited back later.'}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {spaceActionError && <p role="alert" className="text-sm text-destructive">{spaceActionError}</p>}
            <Button
              type="button"
              variant={selectedRole === 'owner' ? 'destructive' : 'outline'}
              onClick={() => void leaveOrDeleteSpace()}
              disabled={spaceAction !== null}
              className="min-h-11"
            >
              {spaceAction
                ? <LoaderCircle aria-hidden="true" className="me-2 size-4 animate-spin" />
                : selectedRole === 'owner'
                  ? <Archive aria-hidden="true" className="me-2 size-4" />
                  : <LogOut aria-hidden="true" className="me-2 size-4" />}
              {spaceAction
                ? spaceAction === 'delete' ? 'Deleting Space…' : 'Leaving Space…'
                : selectedRole === 'owner' ? 'Delete shared Space' : 'Leave Space'}
            </Button>
          </CardContent>
        </Card>
      )}
    </PageTransition>
  );
}
