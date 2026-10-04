import { useEffect, useState } from 'react';
import { Link } from 'wouter';
import { Loader2, RefreshCw, ShieldCheck, WalletCards } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { PageTransition } from '@/components/ui/page-transition';
import { useToast } from '@/hooks/use-toast';
import { CInputGroup9CurrencyInput } from '@/components/examples/c-input-group-9';
import { creditApi, creditErrorMessage, formatUsdMicros, formatUsdMicrosInput, newCreditIdempotencyKey, tryParseUsdMicros, type CreditPolicy, type CreditUsageResponse } from '@/lib/credit-api';

const voiceAgentRateCardKey = 'assemblyai:voice_agent:managed-voice-agent';

type PolicyDrafts = {
  version: number;
  monthlyGrant: string;
  rolloverCap: string;
  rateCards: Record<string, string>;
  rolloverExpiryDays: number;
  overrunMarginPercent: number;
};

function createPolicyDrafts(policy: CreditPolicy, current: PolicyDrafts | null): PolicyDrafts {
  if (current?.version === policy.version) {
    return {
      ...current,
      rateCards: Object.fromEntries(
        Object.entries(policy.rateCards).map(([key, card]) => [
          key,
          current.rateCards[key] ?? (
            card.usdMicrosPerHour == null
              ? ''
              : formatUsdMicrosInput(card.usdMicrosPerHour)
          ),
        ]),
      ),
    };
  }

  return {
    version: policy.version,
    monthlyGrant: formatUsdMicrosInput(policy.monthlyGrantUsdMicros),
    rolloverCap: formatUsdMicrosInput(policy.rolloverCapUsdMicros),
    rateCards: Object.fromEntries(
      Object.entries(policy.rateCards).map(([key, card]) => [
        key,
        card.usdMicrosPerHour == null
          ? ''
          : formatUsdMicrosInput(card.usdMicrosPerHour),
      ]),
    ),
    rolloverExpiryDays: policy.rolloverExpiryDays,
    overrunMarginPercent: policy.overrunMarginPercent,
  };
}

export function AdminCreditsPage() {
  const { toast } = useToast(); const [policy, setPolicy] = useState<CreditPolicy | null>(null); const [usage, setUsage] = useState<CreditUsageResponse | null>(null);
  const [policyDrafts, setPolicyDrafts] = useState<PolicyDrafts | null>(null);
  const [userId, setUserId] = useState(''); const [amount, setAmount] = useState(''); const [reason, setReason] = useState(''); const [changeReason, setChangeReason] = useState('');
  const [saving, setSaving] = useState(false); const [loading, setLoading] = useState(true);
  const load = async () => { setLoading(true); try { setPolicy(await creditApi.adminPolicy()); } catch (e) { toast({ title: creditErrorMessage(e, 'USD tariff policy is unavailable'), variant: 'destructive' }); } finally { setLoading(false); } };
  useEffect(() => { void load(); }, []);
  useEffect(() => {
    if (!policy || policy.rateCards[voiceAgentRateCardKey]) return;
    setPolicy((current) => current && !current.rateCards[voiceAgentRateCardKey] ? {
      ...current,
      rateCards: {
        ...current.rateCards,
        [voiceAgentRateCardKey]: {
          provider: 'assemblyai', mode: 'voice_agent', model: 'managed-voice-agent',
          meter: 'hour', usdMicrosPerHour: null,
          inputUsdMicrosPerMillion: null, outputUsdMicrosPerMillion: null,
        },
      },
    } : current);
  }, [policy]);
  useEffect(() => {
    if (!policy) {
      setPolicyDrafts(null);
      return;
    }
    setPolicyDrafts((current) => createPolicyDrafts(policy, current));
  }, [policy]);
  const lookup = async () => {
    if (!userId.trim()) return;
    try {
      setUsage(await creditApi.adminUsage(userId.trim()));
    } catch (e) {
      toast({ title: creditErrorMessage(e, 'Account usage is unavailable'), variant: 'destructive' });
    }
  };
  const adjust = async () => {
    const micros = tryParseUsdMicros(amount);
    if (!userId.trim() || micros === null || micros === 0 || !reason.trim()) return;
    setSaving(true);
    try {
      await creditApi.adminAdjustment({
        userId: userId.trim(),
        amountUsdMicros: micros,
        reason: reason.trim(),
        idempotencyKey: newCreditIdempotencyKey(),
      });
      toast({ title: 'USD adjustment posted', description: `${formatUsdMicros(micros)} posted to ${userId.trim()}.` });
      setAmount('');
      setReason('');
      await lookup();
    } catch (e) {
      toast({ title: creditErrorMessage(e, 'Adjustment could not be posted'), variant: 'destructive' });
    } finally {
      setSaving(false);
    }
  };
  const updatePolicyDraft = (patch: Partial<Omit<PolicyDrafts, 'version' | 'rateCards'>>) =>
    setPolicyDrafts((current) => current ? { ...current, ...patch } : current);
  const updateRateDraft = (key: string, value: string) =>
    setPolicyDrafts((current) => current ? {
      ...current,
      rateCards: { ...current.rateCards, [key]: value },
    } : current);
  const monthlyGrantMicros = policyDrafts
    ? tryParseUsdMicros(policyDrafts.monthlyGrant)
    : null;
  const rolloverCapMicros = policyDrafts
    ? tryParseUsdMicros(policyDrafts.rolloverCap)
    : null;
  const rateDraftsAreValid = Boolean(policy && policyDrafts) &&
    Object.entries(policy?.rateCards ?? {}).every(([key]) => {
      const draft = policyDrafts?.rateCards[key] ?? '';
      if (!draft.trim()) return true;
      const micros = tryParseUsdMicros(draft);
      return micros !== null && micros >= 0;
    });
  const policyCurrenciesAreValid =
    monthlyGrantMicros !== null &&
    monthlyGrantMicros >= 0 &&
    rolloverCapMicros !== null &&
    rolloverCapMicros >= 0 &&
    rateDraftsAreValid;
  const adjustmentMicros = tryParseUsdMicros(amount);
  const savePolicy = async () => {
    if (!policy || !policyDrafts || !changeReason.trim()) return;
    const monthlyGrant = tryParseUsdMicros(policyDrafts.monthlyGrant);
    const rolloverCap = tryParseUsdMicros(policyDrafts.rolloverCap);
    if (
      monthlyGrant === null ||
      monthlyGrant < 0 ||
      rolloverCap === null ||
      rolloverCap < 0
    ) {
      toast({
        title: 'Check the USD amounts',
        description: 'Enter non-negative values with no more than six decimal places.',
        variant: 'destructive',
      });
      return;
    }
    const rateCards: CreditPolicy['rateCards'] = {};
    for (const [key, card] of Object.entries(policy.rateCards)) {
      const draft = policyDrafts.rateCards[key] ?? '';
      if (!draft.trim()) {
        rateCards[key] = { ...card, usdMicrosPerHour: null };
        continue;
      }
      const micros = tryParseUsdMicros(draft);
      if (micros === null || micros < 0) {
        toast({
          title: 'Check the USD amounts',
          description: 'Enter non-negative values with no more than six decimal places.',
          variant: 'destructive',
        });
        return;
      }
      rateCards[key] = { ...card, usdMicrosPerHour: micros };
    }

    setSaving(true);
    try {
      const next = await creditApi.updateAdminPolicy({
        rateCards,
        monthlyGrantUsdMicros: monthlyGrant,
        rolloverCapUsdMicros: rolloverCap,
        rolloverExpiryDays: policyDrafts.rolloverExpiryDays,
        overrunMarginPercent: policyDrafts.overrunMarginPercent,
      }, policy.version, changeReason.trim());
      setPolicy(next);
      setPolicyDrafts(createPolicyDrafts(next, null));
      setChangeReason('');
      toast({ title: `Tariff policy v${next.version} published` });
    } catch (e) {
      toast({ title: creditErrorMessage(e, 'Policy could not be published'), variant: 'destructive' });
    } finally {
      setSaving(false);
    }
  };
  if (loading || !policy || !policyDrafts) {
    return (
      <PageTransition surface={false} className="mx-auto max-w-5xl space-y-6">
        <div className="h-12 w-64 animate-pulse rounded-lg bg-muted" />
        <div className="h-64 animate-pulse rounded-2xl bg-muted" />
        <Card role="alert">
          <CardHeader>
            <CardTitle>USD tariff policy is unavailable</CardTitle>
            <CardDescription>
              Only authorized administrators can manage this policy.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button onClick={() => void load()}>
              <RefreshCw className="me-2 h-4 w-4" />
              Try again
            </Button>
          </CardContent>
        </Card>
      </PageTransition>
    );
  }

  return (
    <PageTransition surface={false} className="mx-auto max-w-6xl space-y-6">
      <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.22em] text-primary">
            <ShieldCheck className="h-3.5 w-3.5" />
            Admin control room
          </p>
          <h1 className="text-3xl font-display font-bold tracking-tight md:text-4xl">
            USD tariff policy
          </h1>
          <p className="mt-2 max-w-2xl text-muted-foreground">
            Publish auditable provider rates and post adjustments or refunds in USD microunits.
          </p>
        </div>
        <Link
          href="/credits"
          className="inline-flex min-h-10 items-center rounded-md border border-border px-4 text-sm font-medium"
        >
          <WalletCards className="me-2 h-4 w-4" />
          Balance
        </Link>
      </header>

      <div className="grid gap-6 xl:grid-cols-[1.2fr_0.8fr]">
        <Card>
          <CardHeader>
            <CardTitle>
              Active policy <Badge className="ms-2">v{policy.version}</Badge>
            </CardTitle>
            <CardDescription>
              All rates are fixed-point USD values. OpenAI rates remain unconfigured until entered.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="admin-monthly-grant">Monthly grant (USD)</Label>
                <CInputGroup9CurrencyInput
                  id="admin-monthly-grant"
                  value={policyDrafts.monthlyGrant}
                  onValueChange={(value) => updatePolicyDraft({ monthlyGrant: value })}
                  ariaLabel="Monthly grant in US dollars"
                  ariaInvalid={monthlyGrantMicros === null || monthlyGrantMicros < 0}
                  disabled={saving}
                  testId="input-admin-monthly-grant"
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="admin-rollover-cap">Rollover cap (USD)</Label>
                <CInputGroup9CurrencyInput
                  id="admin-rollover-cap"
                  value={policyDrafts.rolloverCap}
                  onValueChange={(value) => updatePolicyDraft({ rolloverCap: value })}
                  ariaLabel="Rollover cap in US dollars"
                  ariaInvalid={rolloverCapMicros === null || rolloverCapMicros < 0}
                  disabled={saving}
                  testId="input-admin-rollover-cap"
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="admin-rollover-expiry-days">Rollover expiry days</Label>
                <Input
                  id="admin-rollover-expiry-days"
                  type="number"
                  min={1}
                  step={1}
                  value={policyDrafts.rolloverExpiryDays}
                  onChange={(event) =>
                    updatePolicyDraft({
                      rolloverExpiryDays: Math.max(
                        1,
                        Math.trunc(Number(event.target.value) || 1),
                      ),
                    })
                  }
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="admin-overrun-margin">Overrun margin percent</Label>
                <Input
                  id="admin-overrun-margin"
                  type="number"
                  min={0}
                  step={1}
                  value={policyDrafts.overrunMarginPercent}
                  onChange={(event) =>
                    updatePolicyDraft({
                      overrunMarginPercent: Math.max(
                        0,
                        Math.trunc(Number(event.target.value) || 0),
                      ),
                    })
                  }
                />
              </div>
            </div>

            <div className="space-y-2">
              {Object.entries(policy.rateCards).map(([key, card], index) => {
                const draft = policyDrafts.rateCards[key] ?? '';
                const rateMicros = draft.trim()
                  ? tryParseUsdMicros(draft)
                  : null;
                const invalidRate =
                  draft.trim() !== '' &&
                  (rateMicros === null || rateMicros < 0);
                const rateId = `admin-rate-${index}`;

                return (
                  <div
                    key={key}
                    className="space-y-1.5 rounded-lg border border-border/70 p-3"
                  >
                    <p className="font-medium">
                      {card.provider} · {card.mode} · {card.model}
                    </p>
                    <p className="text-sm text-muted-foreground">{card.meter}</p>
                    <Label htmlFor={rateId}>Rate per hour (USD)</Label>
                    <CInputGroup9CurrencyInput
                      id={rateId}
                      value={draft}
                      onValueChange={(value) => updateRateDraft(key, value)}
                      placeholder="Not configured"
                      ariaLabel={`${key} USD per hour`}
                      ariaInvalid={invalidRate}
                      disabled={saving}
                      testId={`input-admin-rate-${index}`}
                    />
                  </div>
                );
              })}
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="policy-reason">Reason for next version</Label>
              <Input
                id="policy-reason"
                value={changeReason}
                onChange={(event) => setChangeReason(event.target.value)}
                placeholder="Explain the tariff change"
              />
            </div>
            <Button
              onClick={() => void savePolicy()}
              disabled={saving || !changeReason.trim() || !policyCurrenciesAreValid}
            >
              {saving && <Loader2 className="me-2 h-4 w-4 animate-spin" />}
              Publish policy v{policy.version + 1}
            </Button>
          </CardContent>
        </Card>

        <Card className="h-fit">
          <CardHeader>
            <CardTitle>Post USD adjustment</CardTitle>
            <CardDescription>
              Use a signed decimal amount such as $5.00 or -$0.25. The API receives integer microunits.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="admin-user">User ID</Label>
              <Input
                id="admin-user"
                value={userId}
                onChange={(event) => setUserId(event.target.value)}
                placeholder="usr_…"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="admin-amount">Amount (USD)</Label>
              <CInputGroup9CurrencyInput
                id="admin-amount"
                value={amount}
                onValueChange={setAmount}
                placeholder="5.00 or -0.25"
                ariaLabel="Signed adjustment amount in US dollars"
                allowNegative
                disabled={saving}
                testId="input-admin-adjustment-amount"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="admin-reason">Reason</Label>
              <Input
                id="admin-reason"
                value={reason}
                onChange={(event) => setReason(event.target.value)}
                placeholder="Customer support adjustment"
              />
            </div>
            <Button
              className="w-full"
              onClick={() => void adjust()}
              disabled={
                saving ||
                !userId.trim() ||
                !reason.trim() ||
                adjustmentMicros === null ||
                adjustmentMicros === 0
              }
            >
              {saving && <Loader2 className="me-2 h-4 w-4 animate-spin" />}
              Post adjustment
            </Button>
            <Button
              variant="outline"
              className="w-full"
              onClick={() => void lookup()}
              disabled={!userId.trim() || saving}
            >
              Inspect account
            </Button>
          </CardContent>
        </Card>
      </div>

      {usage && (
        <Card>
          <CardHeader>
            <CardTitle>Account ledger: {userId}</CardTitle>
            <CardDescription>
              Current account counters and immutable entries.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-6">
              {Object.entries(usage.usage)
                .filter(([key]) => key.endsWith('Micros'))
                .map(([key, value]) => (
                  <div key={key}>
                    <p className="text-xs text-muted-foreground">
                      {key.replace('UsdMicros', '')}
                    </p>
                    <p className="font-mono text-lg">
                      {formatUsdMicros(value as number)}
                    </p>
                  </div>
                ))}
            </div>
            <div className="space-y-2">
              {usage.events.map((event) => (
                <div
                  key={event.id}
                  className="rounded-lg border border-border/60 p-3 text-sm"
                >
                  <p className="font-medium">
                    {event.eventType} · {formatUsdMicros(event.amountUsdMicros)}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {event.createdAt} · reservation {event.reservationId}
                  </p>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}
    </PageTransition>
  );
}