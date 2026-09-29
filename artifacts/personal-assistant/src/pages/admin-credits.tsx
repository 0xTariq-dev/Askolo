import { useEffect, useState } from 'react';
import { ArrowLeftRight, ChevronDown, Coins, FilePenLine, Loader2, RotateCcw, Search, ShieldCheck, SlidersHorizontal, UserRound, WalletCards } from 'lucide-react';
import { useLocation } from 'wouter';
import { Badge } from '@workspace/askolo-design-system/components/ui/badge';
import { Button } from '@workspace/askolo-design-system/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@workspace/askolo-design-system/components/ui/card';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@workspace/askolo-design-system/components/ui/dialog';
import { Input } from '@workspace/askolo-design-system/components/ui/input';
import { Label } from '@workspace/askolo-design-system/components/ui/label';
import { PageTransition } from '@/components/ui/page-transition';
import { Separator } from '@workspace/askolo-design-system/components/ui/separator';
import { useToast } from '@workspace/askolo-design-system/hooks/use-toast';
import { creditApi, creditErrorMessage, newCreditIdempotencyKey, type CreditPolicy, type CreditReceipt, type CreditUsageResponse } from '@/lib/credit-api';

type LedgerAction = {
  kind: 'adjustment' | 'grant' | 'reservation';
  id: string | number;
  idempotencyKey: string;
  label: string;
  maxAmount?: number;
};

function numberValue(value: string) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

export function AdminCreditsPage() {
  const [, setLocation] = useLocation();
  const { toast } = useToast();
  const [policy, setPolicy] = useState<CreditPolicy | null>(null);
  const [policyForm, setPolicyForm] = useState<CreditPolicy | null>(null);
  const [loading, setLoading] = useState(true);
  const [policyLoadError, setPolicyLoadError] = useState('');
  const [savingPolicy, setSavingPolicy] = useState(false);
  const [targetUserId, setTargetUserId] = useState('');
  const [targetUsage, setTargetUsage] = useState<CreditUsageResponse | null>(null);
  const [lookupBusy, setLookupBusy] = useState(false);
  const [adjustment, setAdjustment] = useState({ amountCredits: '', reason: '', idempotencyKey: '' });
  const [adjustBusy, setAdjustBusy] = useState(false);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [ledgerAction, setLedgerAction] = useState<LedgerAction | null>(null);
  const [ledgerReason, setLedgerReason] = useState('');
  const [ledgerAmount, setLedgerAmount] = useState('');
  const [ledgerActionBusy, setLedgerActionBusy] = useState(false);

  const loadPolicy = async () => {
    setLoading(true);
    setPolicyLoadError('');
    try {
      const next = await creditApi.adminPolicy();
      setPolicy(next);
      setPolicyForm({ ...next, changeReason: '' });
    } catch (error) {
      setPolicyLoadError(creditErrorMessage(error, 'Admin credit controls are unavailable.'));
      toast({ title: creditErrorMessage(error, 'Admin credit controls are unavailable'), variant: 'destructive' });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void loadPolicy(); }, []);

  const lookupUser = async () => {
    if (!targetUserId.trim()) return;
    setLookupBusy(true);
    try {
      setTargetUsage(await creditApi.adminUsage(targetUserId.trim()));
    } catch (error) {
      toast({ title: creditErrorMessage(error, 'Could not load that account'), variant: 'destructive' });
    } finally {
      setLookupBusy(false);
    }
  };

  const applyAdjustment = async () => {
    if (!targetUserId.trim() || !adjustment.reason.trim() || !adjustment.idempotencyKey.trim() || !numberValue(adjustment.amountCredits.toString())) return;
    setAdjustBusy(true);
    try {
      await creditApi.adminAdjustment({
        userId: targetUserId.trim(),
        amountCredits: numberValue(adjustment.amountCredits),
        reason: adjustment.reason.trim(),
        idempotencyKey: adjustment.idempotencyKey.trim(),
      });
      toast({ title: 'Ledger adjustment applied', description: `${adjustment.amountCredits} credits posted to ${targetUserId.trim()}.` });
      setAdjustment({ amountCredits: '', reason: '', idempotencyKey: '' });
      await lookupUser();
    } catch (error) {
      toast({ title: creditErrorMessage(error, 'Adjustment could not be applied'), variant: 'destructive' });
    } finally {
      setAdjustBusy(false);
    }
  };

  const beginLedgerAction = (action: Omit<LedgerAction, 'idempotencyKey'>) => {
    try {
      setLedgerAction({ ...action, idempotencyKey: newCreditIdempotencyKey() });
      setLedgerReason('');
      setLedgerAmount(action.kind === 'reservation' ? String(action.maxAmount ?? '') : '');
    } catch (error) {
      toast({ title: creditErrorMessage(error, 'Secure request identifiers are unavailable'), variant: 'destructive' });
    }
  };

  const applyLedgerAction = async () => {
    if (!ledgerAction || !targetUserId.trim() || !ledgerReason.trim() || ledgerReason.length > 160) return;
    const amount = Number(ledgerAmount);
    if (ledgerAction.kind === 'reservation' &&
      (!Number.isInteger(amount) || amount < 1 || amount > (ledgerAction.maxAmount ?? 0))) {
      toast({ title: 'Enter a valid refund amount', description: `Choose from 1 to ${ledgerAction.maxAmount ?? 0} credits.`, variant: 'destructive' });
      return;
    }

    setLedgerActionBusy(true);
    try {
      const input = {
        userId: targetUserId.trim(),
        reason: ledgerReason.trim(),
        idempotencyKey: ledgerAction.idempotencyKey,
      };
      if (ledgerAction.kind === 'adjustment') {
        await creditApi.adminReverseAdjustment(Number(ledgerAction.id), input);
      } else if (ledgerAction.kind === 'grant') {
        await creditApi.adminReverseGrant(Number(ledgerAction.id), input);
      } else {
        await creditApi.adminRefundReservation(String(ledgerAction.id), { ...input, amountCredits: amount });
      }
      toast({ title: 'Ledger action completed', description: ledgerAction.label });
      setLedgerAction(null);
      await lookupUser();
    } catch (error) {
      toast({ title: creditErrorMessage(error, 'The ledger action could not be completed'), variant: 'destructive' });
    } finally {
      setLedgerActionBusy(false);
    }
  };

  const savePolicy = async () => {
    if (!policyForm || !policy) return;
    setSavingPolicy(true);
    try {
      const next = await creditApi.updateAdminPolicy({
        operationWeights: policyForm.operationWeights,
        monthlyGrantCredits: policyForm.monthlyGrantCredits,
        rolloverCapCredits: policyForm.rolloverCapCredits,
        rolloverExpiryDays: policyForm.rolloverExpiryDays,
        overrunMarginPercent: policyForm.overrunMarginPercent,
      }, policy.version, policyForm.changeReason?.trim() ?? '');
      setPolicy(next);
      setPolicyForm({ ...next, changeReason: '' });
      toast({ title: 'Credit policy published', description: `Policy v${next.version} is now active.` });
    } catch (error) {
      toast({ title: creditErrorMessage(error, 'Policy could not be published'), variant: 'destructive' });
      void loadPolicy();
    } finally {
      setSavingPolicy(false);
    }
  };

  if (loading || !policyForm || !policy) {
    if (loading) {
      return <PageTransition className="mx-auto max-w-5xl space-y-6" aria-busy="true"><div className="h-12 w-64 animate-pulse rounded-lg bg-muted" /><div className="h-64 animate-pulse rounded-2xl bg-muted" /><div className="h-96 animate-pulse rounded-2xl bg-muted" /></PageTransition>;
    }
    return (
      <PageTransition className="mx-auto max-w-3xl space-y-5">
        <Card role="alert">
          <CardHeader><CardTitle>Admin credit controls are unavailable</CardTitle><CardDescription>{policyLoadError || 'This account is not authorized to manage credit policy.'}</CardDescription></CardHeader>
          <CardContent><Button variant="outline" onClick={() => setLocation('/credits')}><WalletCards className="mr-2 h-4 w-4" />Return to your wallet</Button></CardContent>
        </Card>
      </PageTransition>
    );
  }

  const setPolicyField = <K extends keyof CreditPolicy>(key: K, value: CreditPolicy[K]) => setPolicyForm((current) => current ? { ...current, [key]: value } : current);
  const weightEntries = Object.entries(policyForm.operationWeights);
  const ledgerAdjustments = targetUsage?.ledgerEntries?.adjustments ?? [];
  const ledgerGrants = targetUsage?.ledgerEntries?.grants ?? [];
  const reversedAdjustmentIds = new Set(ledgerAdjustments.map((entry) => entry.reversalOfId).filter((id): id is number => typeof id === 'number'));
  const reversedGrantIds = new Set(ledgerAdjustments.map((entry) => entry.reversalOfGrantId).filter((id): id is number => typeof id === 'number'));
  const recentReceipts = targetUsage
    ? [
        ...(targetUsage.recent.reservations ?? []).map((item) => ({ item, key: `reservation-${item.id}` })),
        ...(targetUsage.recent.events ?? []).map((item) => ({ item, key: `event-${item.id}` })),
      ]
    : [];

  return (
    <PageTransition className="mx-auto max-w-6xl space-y-6">
      <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.22em] text-primary"><ShieldCheck className="h-3.5 w-3.5" /> Admin control room</p>
          <h1 className="text-3xl font-display font-bold tracking-tight md:text-4xl">AI credit operations</h1>
          <p className="mt-2 max-w-2xl text-muted-foreground">Publish policy changes, inspect a user’s receipts, and make auditable adjustments without touching the ledger directly.</p>
        </div>
        <Button variant="outline" onClick={() => setLocation('/credits')} data-testid="button-back-to-credits"><WalletCards className="mr-2 h-4 w-4" /> User wallet</Button>
      </header>

      <div className="grid gap-6 xl:grid-cols-[1.2fr_0.8fr]">
        <Card>
          <CardHeader className="flex flex-row items-start justify-between gap-3"><div><CardTitle className="flex items-center gap-2"><SlidersHorizontal className="h-4 w-4 text-primary" /> Active credit policy</CardTitle><CardDescription>Version {policy.version}. Publishing creates the next immutable policy version.</CardDescription></div><Badge className="border-primary/20 bg-primary/10 text-primary">v{policy.version}</Badge></CardHeader>
          <CardContent className="space-y-5">
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2"><Label htmlFor="monthly-grant">Monthly grant credits</Label><Input id="monthly-grant" type="number" min="0" value={policyForm.monthlyGrantCredits} onChange={(event) => setPolicyField('monthlyGrantCredits', numberValue(event.target.value))} data-testid="input-monthly-grant" /></div>
              <div className="space-y-2"><Label htmlFor="rollover-cap">Rollover cap credits</Label><Input id="rollover-cap" type="number" min="0" value={policyForm.rolloverCapCredits} onChange={(event) => setPolicyField('rolloverCapCredits', numberValue(event.target.value))} data-testid="input-rollover-cap" /></div>
              <div className="space-y-2"><Label htmlFor="rollover-expiry">Rollover expiry days</Label><Input id="rollover-expiry" type="number" min="1" value={policyForm.rolloverExpiryDays} onChange={(event) => setPolicyField('rolloverExpiryDays', numberValue(event.target.value))} data-testid="input-rollover-expiry" /></div>
              <div className="space-y-2"><Label htmlFor="overrun-margin">Overrun margin percent</Label><Input id="overrun-margin" type="number" min="0" max="100" value={policyForm.overrunMarginPercent} onChange={(event) => setPolicyField('overrunMarginPercent', numberValue(event.target.value))} data-testid="input-overrun-margin" /></div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="policy-change-reason">Reason for this policy version</Label>
              <Input id="policy-change-reason" maxLength={160} value={policyForm.changeReason ?? ''} onChange={(event) => setPolicyField('changeReason', event.target.value)} placeholder="Explain what changed" data-testid="input-policy-change-reason" />
            </div>
            <Separator />
            <div><div className="mb-3 flex items-center justify-between"><div><p className="text-sm font-medium">Operation weights</p><p className="text-xs text-muted-foreground">Credits reserved per billable unit.</p></div><Coins className="h-4 w-4 text-primary" /></div><div className="space-y-2">{weightEntries.map(([key, value]) => <div key={key} className="flex items-center gap-3"><Label className="min-w-28 font-mono text-xs text-muted-foreground">{key}</Label><Input type="number" min="1" value={value} onChange={(event) => setPolicyField('operationWeights', { ...policyForm.operationWeights, [key]: numberValue(event.target.value) })} data-testid={`input-operation-weight-${key}`} /><span className="text-xs text-muted-foreground">credits / unit</span></div>)}</div></div>
            <Button onClick={() => void savePolicy()} disabled={savingPolicy || !policyForm.changeReason?.trim()} data-testid="button-publish-credit-policy">{savingPolicy && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}Publish policy v{policy.version + 1}</Button>
          </CardContent>
        </Card>

        <Card className="h-fit">
          <CardHeader><CardTitle className="flex items-center gap-2"><ArrowLeftRight className="h-4 w-4 text-primary" /> Make an adjustment</CardTitle><CardDescription>Positive grants add credits. Negative adjustments are allowed only when the account can cover them.</CardDescription></CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2"><Label htmlFor="adjust-user">User ID</Label><Input id="adjust-user" value={targetUserId} onChange={(event) => setTargetUserId(event.target.value)} placeholder="usr_…" data-testid="input-adjust-user" /></div>
            <div className="space-y-2"><Label htmlFor="adjust-amount">Amount credits</Label><Input id="adjust-amount" type="number" value={adjustment.amountCredits} onChange={(event) => setAdjustment((current) => ({ ...current, amountCredits: event.target.value }))} placeholder="e.g. 25 or -5" data-testid="input-adjust-amount" /></div>
            <div className="space-y-2"><Label htmlFor="adjust-reason">Reason</Label><Input id="adjust-reason" maxLength={160} value={adjustment.reason} onChange={(event) => setAdjustment((current) => ({ ...current, reason: event.target.value }))} placeholder="Launch cohort grant" data-testid="input-adjust-reason" /></div>
            <div className="space-y-2"><Label htmlFor="adjust-key">Idempotency key</Label><Input id="adjust-key" value={adjustment.idempotencyKey} onChange={(event) => setAdjustment((current) => ({ ...current, idempotencyKey: event.target.value }))} placeholder="grant-2026-09-24-001" data-testid="input-adjust-key" /></div>
            <Button className="w-full" onClick={() => void applyAdjustment()} disabled={adjustBusy || !targetUserId || !adjustment.reason || !adjustment.idempotencyKey || !numberValue(adjustment.amountCredits)} data-testid="button-apply-adjustment">{adjustBusy && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}Post adjustment</Button>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader><CardTitle className="flex items-center gap-2"><Search className="h-4 w-4 text-primary" /> Account lookup</CardTitle><CardDescription>Review balances and receipt trails before or after an adjustment.</CardDescription></CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-col gap-2 sm:flex-row"><Input value={targetUserId} onChange={(event) => setTargetUserId(event.target.value)} placeholder="Enter a user ID" data-testid="input-lookup-user" /><Button onClick={() => void lookupUser()} disabled={lookupBusy || !targetUserId.trim()} data-testid="button-lookup-user">{lookupBusy && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}Look up account</Button></div>
          {targetUsage && (
            <div className="space-y-5 rounded-xl border border-border/70 bg-background/30 p-4">
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-6">
                {Object.entries(targetUsage.usage).map(([key, value]) => (
                  <div key={key}><p className="text-xs capitalize text-muted-foreground">{key}</p><p className="font-mono text-lg font-semibold">{value}</p></div>
                ))}
              </div>
              <Separator />
              <section aria-labelledby="admin-ledger-entries-title" className="space-y-3">
                <div><h3 id="admin-ledger-entries-title" className="font-medium">Grants and adjustments</h3><p className="text-xs text-muted-foreground">Reversals append a separate, attributed ledger entry.</p></div>
                {[...ledgerAdjustments.map((entry) => ({ entry, kind: 'adjustment' as const })), ...ledgerGrants.map((entry) => ({ entry, kind: 'grant' as const }))]
                  .map(({ entry, kind }) => {
                    const entryId = Number(entry.id);
                    const reversed = kind === 'adjustment'
                      ? reversedAdjustmentIds.has(entryId) || entry.reversalOfId != null
                      : reversedGrantIds.has(entryId);
                    const reversible = kind === 'adjustment'
                      ? entry.reversalOfId == null && entry.reversalOfGrantId == null && !reversed
                      : !reversed;
                    return (
                      <div key={`${kind}-${entry.id}`} className="flex flex-col gap-3 rounded-lg border border-border/60 p-3 sm:flex-row sm:items-center sm:justify-between">
                        <div className="min-w-0">
                          <p className="font-mono text-sm font-semibold">{entry.amountCredits ?? 0} credits · {kind}</p>
                          <p className="break-words text-xs text-muted-foreground">{entry.reason || 'No reason recorded'} · {entry.actorUserId || 'system'}</p>
                          <p className="text-[11px] text-muted-foreground">Entry {entry.id}{reversed ? ' · reversed' : ''}</p>
                        </div>
                        {reversible && (
                          <Button type="button" variant="outline" size="sm" onClick={() => beginLedgerAction({ kind, id: entry.id, label: `Reverse ${kind} entry ${entry.id}` })}>
                            <RotateCcw className="mr-2 h-3.5 w-3.5" />Reverse
                          </Button>
                        )}
                      </div>
                    );
                  })}
                {ledgerAdjustments.length === 0 && ledgerGrants.length === 0 && (
                  <p className="rounded-lg border border-dashed border-border p-4 text-sm text-muted-foreground">No grants or adjustments yet.</p>
                )}
              </section>
              <Separator />
              <section aria-labelledby="admin-receipts-title" className="space-y-3">
                <div><h3 id="admin-receipts-title" className="font-medium">Reservations and events</h3><p className="text-xs text-muted-foreground">Refund settled amounts when needed; reservation expiry is recorded as an event.</p></div>
                {recentReceipts.map(({ item, key }) => {
                  const itemId = String(item.id);
                  const remaining = Math.max(0, (item.settledCredits ?? 0) - (item.refundedCredits ?? 0));
                  return (
                    <div key={key} className="rounded-lg border border-border/60">
                      <button type="button" className="flex w-full items-center justify-between gap-3 p-3 text-left" onClick={() => setExpanded(expanded === key ? null : key)} data-testid={`button-toggle-admin-receipt-${key}`}>
                        <span className="flex min-w-0 items-center gap-2"><FilePenLine className="h-4 w-4 shrink-0 text-primary" /><span className="truncate text-sm">{item.eventType ?? item.operationType ?? 'Reservation'} · {itemId}</span></span>
                        <ChevronDown className={`h-4 w-4 transition-transform ${expanded === key ? 'rotate-180' : ''}`} aria-hidden="true" />
                      </button>
                      {expanded === key && (
                        <div className="border-t border-border/60 p-3 text-xs text-muted-foreground">
                          <pre className="overflow-auto whitespace-pre-wrap font-mono">{JSON.stringify(item, null, 2)}</pre>
                          <div className="mt-2 flex flex-wrap gap-2">
                            <Button type="button" variant="ghost" size="sm" onClick={() => {
                              void navigator.clipboard?.writeText(JSON.stringify(item, null, 2));
                              toast({ title: 'Receipt reference copied' });
                            }}><FilePenLine className="mr-2 h-3.5 w-3.5" />Copy reference</Button>
                            {!item.eventType && item.status === 'settled' && remaining > 0 && (
                              <Button type="button" variant="outline" size="sm" onClick={() => beginLedgerAction({ kind: 'reservation', id: item.id, label: `Refund reservation ${item.id}`, maxAmount: remaining })}>
                                <RotateCcw className="mr-2 h-3.5 w-3.5" />Refund up to {remaining}
                              </Button>
                            )}
                          </div>
                        </div>
                      )}
                    </div>
                  );
                })}
                {recentReceipts.length === 0 && <p className="rounded-lg border border-dashed border-border p-4 text-sm text-muted-foreground">No reservations or events yet.</p>}
              </section>
            </div>
          )}
          {!targetUsage && <div className="rounded-xl border border-dashed border-border p-8 text-center"><UserRound className="mx-auto mb-3 h-7 w-7 text-muted-foreground/50" /><p className="font-medium">No account selected</p><p className="mt-1 text-sm text-muted-foreground">Search by user ID to inspect ledger receipts.</p></div>}
        </CardContent>
      </Card>

      <Dialog open={Boolean(ledgerAction)} onOpenChange={(open) => !open && !ledgerActionBusy && setLedgerAction(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{ledgerAction?.kind === 'reservation' ? 'Refund settled credits' : 'Reverse ledger entry'}</DialogTitle>
            <DialogDescription>
              {ledgerAction?.label}. This appends a new ledger record; it does not delete the original.
            </DialogDescription>
          </DialogHeader>
          {ledgerAction?.kind === 'reservation' && (
            <div className="space-y-2">
              <Label htmlFor="ledger-refund-amount">Refund amount (up to {ledgerAction.maxAmount} credits)</Label>
              <Input id="ledger-refund-amount" type="number" min="1" max={ledgerAction.maxAmount} step="1" value={ledgerAmount} onChange={(event) => setLedgerAmount(event.target.value)} inputMode="numeric" />
            </div>
          )}
          <div className="space-y-2">
            <Label htmlFor="ledger-action-reason">Reason</Label>
            <Input id="ledger-action-reason" maxLength={160} value={ledgerReason} onChange={(event) => setLedgerReason(event.target.value)} placeholder="Explain this ledger action" />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setLedgerAction(null)} disabled={ledgerActionBusy}>Cancel</Button>
            <Button type="button" variant="destructive" onClick={() => void applyLedgerAction()} disabled={ledgerActionBusy || !ledgerReason.trim()}>
              {ledgerActionBusy && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}Confirm ledger action
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </PageTransition>
  );
}