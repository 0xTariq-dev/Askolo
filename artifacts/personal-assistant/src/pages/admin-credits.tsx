import { useEffect, useState } from 'react';
import { ArrowLeftRight, ChevronDown, Coins, FilePenLine, Loader2, RotateCcw, Search, ShieldCheck, SlidersHorizontal, UserRound, WalletCards } from 'lucide-react';
import { useLocation } from 'wouter';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { PageTransition } from '@/components/ui/page-transition';
import { Separator } from '@/components/ui/separator';
import { useToast } from '@/hooks/use-toast';
import { creditApi, creditErrorMessage, type CreditPolicy, type CreditUsageResponse } from '@/lib/credit-api';

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
  const [savingPolicy, setSavingPolicy] = useState(false);
  const [targetUserId, setTargetUserId] = useState('');
  const [targetUsage, setTargetUsage] = useState<CreditUsageResponse | null>(null);
  const [lookupBusy, setLookupBusy] = useState(false);
  const [adjustment, setAdjustment] = useState({ amountCredits: '', reason: '', idempotencyKey: '' });
  const [adjustBusy, setAdjustBusy] = useState(false);
  const [expanded, setExpanded] = useState<string | null>(null);

  const loadPolicy = async () => {
    setLoading(true);
    try {
      const next = await creditApi.adminPolicy();
      setPolicy(next);
      setPolicyForm(next);
    } catch (error) {
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

  const savePolicy = async () => {
    if (!policyForm || !policy) return;
    setSavingPolicy(true);
    try {
      const next = await creditApi.updateAdminPolicy({ ...policyForm, version: policy.version + 1 });
      setPolicy(next);
      setPolicyForm(next);
      toast({ title: 'Credit policy published', description: `Policy v${next.version} is now active.` });
    } catch (error) {
      toast({ title: creditErrorMessage(error, 'Policy could not be published'), variant: 'destructive' });
      void loadPolicy();
    } finally {
      setSavingPolicy(false);
    }
  };

  if (loading || !policyForm) {
    return <PageTransition className="mx-auto max-w-5xl space-y-6"><div className="h-12 w-64 animate-pulse rounded-lg bg-white/5" /><div className="h-64 animate-pulse rounded-2xl bg-white/5" /><div className="h-96 animate-pulse rounded-2xl bg-white/5" /></PageTransition>;
  }

  const setPolicyField = <K extends keyof CreditPolicy>(key: K, value: CreditPolicy[K]) => setPolicyForm((current) => current ? { ...current, [key]: value } : current);
  const weightEntries = Object.entries(policyForm.operationWeights);

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
          <CardHeader className="flex flex-row items-start justify-between gap-3"><div><CardTitle className="flex items-center gap-2"><SlidersHorizontal className="h-4 w-4 text-primary" /> Active pricing policy</CardTitle><CardDescription>Version {policy.version}. Publishing creates the next immutable policy version.</CardDescription></div><Badge className="border-primary/20 bg-primary/10 text-primary">v{policy.version}</Badge></CardHeader>
          <CardContent className="space-y-5">
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2"><Label htmlFor="monthly-grant">Monthly grant credits</Label><Input id="monthly-grant" type="number" min="0" value={policyForm.monthlyGrantCredits} onChange={(event) => setPolicyField('monthlyGrantCredits', numberValue(event.target.value))} data-testid="input-monthly-grant" /></div>
              <div className="space-y-2"><Label htmlFor="rollover-cap">Rollover cap credits</Label><Input id="rollover-cap" type="number" min="0" value={policyForm.rolloverCapCredits} onChange={(event) => setPolicyField('rolloverCapCredits', numberValue(event.target.value))} data-testid="input-rollover-cap" /></div>
              <div className="space-y-2"><Label htmlFor="rollover-expiry">Rollover expiry days</Label><Input id="rollover-expiry" type="number" min="1" value={policyForm.rolloverExpiryDays} onChange={(event) => setPolicyField('rolloverExpiryDays', numberValue(event.target.value))} data-testid="input-rollover-expiry" /></div>
              <div className="space-y-2"><Label htmlFor="overrun-margin">Overrun margin percent</Label><Input id="overrun-margin" type="number" min="0" max="100" value={policyForm.overrunMarginPercent} onChange={(event) => setPolicyField('overrunMarginPercent', numberValue(event.target.value))} data-testid="input-overrun-margin" /></div>
            </div>
            <Separator />
            <div><div className="mb-3 flex items-center justify-between"><div><p className="text-sm font-medium">Operation weights</p><p className="text-xs text-muted-foreground">Credits reserved per billable unit.</p></div><Coins className="h-4 w-4 text-primary" /></div><div className="space-y-2">{weightEntries.map(([key, value]) => <div key={key} className="flex items-center gap-3"><Label className="min-w-28 font-mono text-xs text-muted-foreground">{key}</Label><Input type="number" min="1" value={value} onChange={(event) => setPolicyField('operationWeights', { ...policyForm.operationWeights, [key]: numberValue(event.target.value) })} data-testid={`input-operation-weight-${key}`} /><span className="text-xs text-muted-foreground">credits / unit</span></div>)}</div></div>
            <Button onClick={() => void savePolicy()} disabled={savingPolicy} data-testid="button-publish-credit-policy">{savingPolicy && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}Publish policy v{policy.version + 1}</Button>
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
          {targetUsage && <div className="space-y-4 rounded-xl border border-border/70 bg-background/30 p-4"><div className="grid grid-cols-2 gap-3 sm:grid-cols-6">{Object.entries(targetUsage.usage).map(([key, value]) => <div key={key}><p className="text-xs capitalize text-muted-foreground">{key}</p><p className="font-mono text-lg font-semibold">{value}</p></div>)}</div><Separator /><div className="space-y-2">{[...(targetUsage.recent.reservations ?? []), ...(targetUsage.recent.events ?? [])].map((item, index) => { const itemId = String(item.id); return <div key={`${itemId}-${index}`} className="rounded-lg border border-border/60"><button type="button" className="flex w-full items-center justify-between gap-3 p-3 text-left" onClick={() => setExpanded(expanded === itemId ? null : itemId)} data-testid={`button-toggle-admin-receipt-${itemId}`}><span className="flex min-w-0 items-center gap-2"><FilePenLine className="h-4 w-4 shrink-0 text-primary" /><span className="truncate text-sm">{item.eventType ?? item.operationType ?? 'Reservation'} · {itemId}</span></span><ChevronDown className={`h-4 w-4 transition-transform ${expanded === itemId ? 'rotate-180' : ''}`} /></button>{expanded === itemId && <div className="border-t border-border/60 p-3 text-xs text-muted-foreground"><pre className="overflow-auto whitespace-pre-wrap font-mono">{JSON.stringify(item, null, 2)}</pre><Button variant="ghost" size="sm" className="mt-2" onClick={() => toast({ title: 'Receipt reference copied' })}><FilePenLine className="mr-2 h-3.5 w-3.5" /> Copy reference</Button></div>}</div>; })}</div></div>}
          {!targetUsage && <div className="rounded-xl border border-dashed border-border p-8 text-center"><UserRound className="mx-auto mb-3 h-7 w-7 text-muted-foreground/50" /><p className="font-medium">No account selected</p><p className="mt-1 text-sm text-muted-foreground">Search by user ID to inspect ledger receipts.</p></div>}
        </CardContent>
      </Card>
    </PageTransition>
  );
}