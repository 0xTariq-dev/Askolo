import { useState } from 'react';
import { Link, useRoute } from 'wouter';
import {
  AlertCircle,
  ArrowRight,
  CheckCircle2,
  LoaderCircle,
  MailCheck,
  RotateCw,
  ShieldCheck,
  UsersRound,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { PageTransition } from '@/components/ui/page-transition';
import { useSpaceScope } from '@/contexts/space-context';
import { acceptV2WorkspaceInvitation, declineV2WorkspaceInvitation } from '@/lib/agenda-v2-api';

type AcceptedInvitation = {
  workspaceId: string;
  role: string;
};

type InvitationProblem = 'email' | 'expired' | 'already-member' | 'other';

function invitationProblem(error: unknown): { kind: InvitationProblem; message: string } {
  const message = error instanceof Error ? error.message : 'The invitation could not be accepted.';
  const normalized = message.toLowerCase();
  if (normalized.includes('sign in with the email') || normalized.includes('email address')) {
    return {
      kind: 'email',
      message: 'This invitation is for a different email address. Sign in to the Askolo account that received the invitation, then open this link again.',
    };
  }
  if (normalized.includes('already belong') || normalized.includes('already a member')) {
    return {
      kind: 'already-member',
      message: 'This account already belongs to the Space. Go to Home and choose it from your Space switcher.',
    };
  }
  if (
    normalized.includes('expired')
    || normalized.includes('no longer available')
    || normalized.includes('(400)')
    || normalized.includes('(404)')
    || normalized.includes('(410)')
  ) {
    return {
      kind: 'expired',
      message: 'This invitation has expired or is no longer available. Ask a Space owner or admin to send a new invitation.',
    };
  }
  return { kind: 'other', message };
}

export function InviteAcceptancePage() {
  const [, params] = useRoute('/invite/:token');
  const { refreshSpaces, setSelectedSpaceId, loading: spacesLoading } = useSpaceScope();
  const token = params?.token ?? '';
  const [accepting, setAccepting] = useState(false);
  const [declining, setDeclining] = useState(false);
  const [selectingSpace, setSelectingSpace] = useState(false);
  const [accepted, setAccepted] = useState<AcceptedInvitation | null>(null);
  const [declined, setDeclined] = useState(false);
  const [selected, setSelected] = useState(false);
  const [problem, setProblem] = useState<{ kind: InvitationProblem; message: string } | null>(null);
  const [selectionError, setSelectionError] = useState<string | null>(null);

  const selectAcceptedSpace = async (workspaceId: string) => {
    setSelectingSpace(true);
    setSelectionError(null);
    try {
      const refreshedSpaces = await refreshSpaces();
      if (!refreshedSpaces.some((space) => space.id === workspaceId)) {
        throw new Error('The Space is not available in your Space list yet. Try again in a moment.');
      }
      setSelectedSpaceId(workspaceId);
      setSelected(true);
    } catch (error) {
      setSelectionError(error instanceof Error
        ? `Your invitation was accepted, but Askolo could not refresh your Space list. ${error.message}`
        : 'Your invitation was accepted, but Askolo could not refresh your Space list.');
    } finally {
      setSelectingSpace(false);
    }
  };

  const acceptInvitation = async () => {
    if (!token || accepting || declining) return;
    setAccepting(true);
    setProblem(null);
    setSelectionError(null);
    try {
      const result = await acceptV2WorkspaceInvitation(token);
      setAccepted(result);
      await selectAcceptedSpace(result.workspaceId);
    } catch (error) {
      setProblem(invitationProblem(error));
    } finally {
      setAccepting(false);
    }
  };

  const declineInvitation = async () => {
    if (!token || accepting || declining) return;
    setDeclining(true);
    setProblem(null);
    try {
      await declineV2WorkspaceInvitation(token);
      setDeclined(true);
    } catch (error) {
      setProblem(invitationProblem(error));
    } finally {
      setDeclining(false);
    }
  };

  const retrySpaceSelection = () => {
    if (accepted) void selectAcceptedSpace(accepted.workspaceId);
  };

  return (
    <PageTransition surface={false} className="mx-auto grid min-h-[60vh] max-w-2xl content-center py-8">
      <Card className="overflow-hidden">
        <div className="h-1.5 bg-primary" aria-hidden="true" />
        <CardHeader className="px-5 pb-4 pt-7 sm:px-8 sm:pt-9">
          <div className="mb-4 grid size-12 place-items-center rounded-2xl bg-primary/10 text-primary">
            {accepted ? <CheckCircle2 aria-hidden="true" className="size-6" /> : <MailCheck aria-hidden="true" className="size-6" />}
          </div>
          <CardTitle className="font-display text-2xl sm:text-3xl">
            {accepted ? 'You’re in' : declined ? 'Invitation declined' : token ? 'Join a Space' : 'Invitation unavailable'}
          </CardTitle>
          <CardDescription className="mt-2 max-w-lg text-sm leading-6">
            {accepted
              ? `Your invitation was accepted${accepted.role ? ` as ${accepted.role}` : ''}. This Space is now selected in Askolo.`
              : declined
                ? 'You declined this invitation. No Space membership was created.'
                : 'This invitation is tied to the email address it was sent to. You must be signed in to the matching Askolo account to accept or decline it.'}
          </CardDescription>
        </CardHeader>

        <CardContent className="space-y-5 px-5 pb-7 sm:px-8 sm:pb-9">
          {!accepted && !declined && (
            <div className="flex items-start gap-3 rounded-xl border border-border/70 bg-muted/25 p-4">
              <ShieldCheck aria-hidden="true" className="mt-0.5 size-5 shrink-0 text-primary" />
              <p className="text-sm leading-6 text-muted-foreground">
                Your Space membership is connected to your account. If you use a different email to sign in, switch accounts before accepting.
              </p>
            </div>
          )}

          {problem && (
            <div role="alert" className="flex items-start gap-3 rounded-xl border border-destructive/25 bg-destructive/5 p-4">
              <AlertCircle aria-hidden="true" className="mt-0.5 size-5 shrink-0 text-destructive" />
              <div>
                <p className="font-semibold">
                  {problem.kind === 'email'
                    ? 'Email does not match'
                    : problem.kind === 'expired'
                      ? 'Invitation expired or invalid'
                      : problem.kind === 'already-member'
                        ? 'Already a Space member'
                        : 'Could not process invitation'}
                </p>
                <p className="mt-1 text-sm leading-5 text-muted-foreground">{problem.message}</p>
              </div>
            </div>
          )}

          {accepted && selectionError && (
            <div role="alert" className="flex flex-col gap-3 rounded-xl border border-warning/30 bg-warning/10 p-4 sm:flex-row sm:items-center sm:justify-between">
              <p className="text-sm leading-5 text-foreground">{selectionError}</p>
              <Button type="button" variant="outline" onClick={retrySpaceSelection} disabled={selectingSpace} className="shrink-0">
                {selectingSpace ? <LoaderCircle aria-hidden="true" className="me-2 size-4 animate-spin" /> : <RotateCw aria-hidden="true" className="me-2 size-4" />}
                Refresh Spaces
              </Button>
            </div>
          )}

          <div className="flex flex-col gap-3 border-t border-border/60 pt-5 sm:flex-row sm:items-center">
            {accepted ? (
              selected ? (
                <Button asChild className="min-h-11 w-full sm:w-auto">
                  <Link href="/home">
                    Open Space <ArrowRight aria-hidden="true" className="ms-2 size-4" />
                  </Link>
                </Button>
              ) : (
                <Button type="button" variant="outline" onClick={retrySpaceSelection} disabled={selectingSpace} className="min-h-11">
                  {selectingSpace ? <LoaderCircle aria-hidden="true" className="me-2 size-4 animate-spin" /> : <RotateCw aria-hidden="true" className="me-2 size-4" />}
                  Select Space
                </Button>
              )
            ) : declined ? (
              <Button asChild variant="outline" className="min-h-11 w-full sm:w-auto">
                <Link href="/home">Return to Askolo</Link>
              </Button>
            ) : (
              <>
                <Button
                  type="button"
                  onClick={acceptInvitation}
                  disabled={!token || accepting || declining || spacesLoading}
                  className="min-h-11 w-full sm:w-auto"
                >
                  {accepting ? <LoaderCircle aria-hidden="true" className="me-2 size-4 animate-spin" /> : <UsersRound aria-hidden="true" className="me-2 size-4" />}
                  {accepting ? 'Accepting…' : spacesLoading ? 'Loading Spaces…' : 'Accept invitation'}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={declineInvitation}
                  disabled={!token || accepting || declining}
                  className="min-h-11 w-full sm:w-auto"
                >
                  {declining && <LoaderCircle aria-hidden="true" className="me-2 size-4 animate-spin" />}
                  {declining ? 'Declining…' : 'Decline invitation'}
                </Button>
              </>
            )}
            {problem?.kind === 'already-member' && (
              <Button asChild type="button" variant="ghost" className="min-h-11">
                <Link href="/home">Go to Home</Link>
              </Button>
            )}
            {!accepted && !declined && !problem && (
              <p className="text-xs leading-5 text-muted-foreground">
                The invitation can only be accepted once by its intended account.
              </p>
            )}
          </div>
        </CardContent>
      </Card>
      <div className="mt-5 text-center text-xs text-muted-foreground">
        <Link href="/home" className="rounded-sm underline-offset-4 hover:text-foreground hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
          Return to Askolo
        </Link>
      </div>
    </PageTransition>
  );
}
