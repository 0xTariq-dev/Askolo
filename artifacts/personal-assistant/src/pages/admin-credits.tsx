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
import { creditApi, creditErrorMessage, formatCredits, formatCreditsInput, formatUsdMicros, formatUsdMicrosInput, newCreditIdempotencyKey, tryParseCreditsToUsdMicros, tryParseUsdMicros, type CreditAdminAlert, type CreditPolicy, type CreditUsageResponse, type CreditUserPolicyOverride, type GrantBatchItem, type GrantBatchPreview, type MonthlyGrantPreview } from '@/lib/credit-api';

const voiceAgentRateCardKey = 'assemblyai:voice_agent:managed-voice-agent';
const openAIRateCardKey = 'openai:assistant:gpt-5.6-luna';
const azureSpeechRateCardKey = 'azure:assistant_speech:azure-neural-tts';

type PolicyDrafts = {
  version: number;
  providerAccountIds: Record<string, string>;
  grantTypePolicies: Record<string, { issuanceCap: string; carryoverCap: string; expiryDays: number }>;
  signupGrant: string;
  monthlyGrant: string;
  monthlyGrantsEnabled: boolean;
  rolloverCap: string;
  rateCards: Record<string, RateCardDraft>;
  rolloverExpiryDays: number;
  overrunMarginPercent: number;
};

type RateCardDraft = {
  usdMicrosPerHour: string;
  inputUsdMicrosPerMillion: string;
  cachedInputUsdMicrosPerMillion: string;
  outputUsdMicrosPerMillion: string;
  usdMicrosPerMillionCharacters: string;
};

type UserOverrideDraft = {
  monthlyGrantsEnabled: 'inherit' | 'enabled' | 'disabled';
  monthlyGrant: string;
  rolloverCap: string;
  grantExpiryDays: string;
};

function createUserOverrideDraft(override: CreditUserPolicyOverride): UserOverrideDraft {
  return {
    monthlyGrantsEnabled: override.monthlyGrantsEnabled === null
      ? 'inherit'
      : override.monthlyGrantsEnabled ? 'enabled' : 'disabled',
    monthlyGrant: override.monthlyGrantUsdMicros === null ? '' : formatCreditsInput(override.monthlyGrantUsdMicros),
    rolloverCap: override.rolloverCapUsdMicros === null ? '' : formatCreditsInput(override.rolloverCapUsdMicros),
    grantExpiryDays: override.grantExpiryDays === null ? '' : String(override.grantExpiryDays),
  };
}

function createRateCardDraft(card: CreditPolicy['rateCards'][string]): RateCardDraft {
  return {
    usdMicrosPerHour: card.usdMicrosPerHour == null ? '' : formatUsdMicrosInput(card.usdMicrosPerHour),
    inputUsdMicrosPerMillion: card.inputUsdMicrosPerMillion == null ? '' : formatUsdMicrosInput(card.inputUsdMicrosPerMillion),
    cachedInputUsdMicrosPerMillion: card.cachedInputUsdMicrosPerMillion == null ? '' : formatUsdMicrosInput(card.cachedInputUsdMicrosPerMillion),
    outputUsdMicrosPerMillion: card.outputUsdMicrosPerMillion == null ? '' : formatUsdMicrosInput(card.outputUsdMicrosPerMillion),
    usdMicrosPerMillionCharacters: card.usdMicrosPerMillionCharacters == null ? '' : formatUsdMicrosInput(card.usdMicrosPerMillionCharacters),
  };
}

function mergeRateCardDraft(
  card: CreditPolicy['rateCards'][string],
  draft?: Partial<RateCardDraft>,
): RateCardDraft {
  const defaults = createRateCardDraft(card);
  return {
    usdMicrosPerHour: typeof draft?.usdMicrosPerHour === 'string' ? draft.usdMicrosPerHour : defaults.usdMicrosPerHour,
    inputUsdMicrosPerMillion: typeof draft?.inputUsdMicrosPerMillion === 'string' ? draft.inputUsdMicrosPerMillion : defaults.inputUsdMicrosPerMillion,
    cachedInputUsdMicrosPerMillion: typeof draft?.cachedInputUsdMicrosPerMillion === 'string' ? draft.cachedInputUsdMicrosPerMillion : defaults.cachedInputUsdMicrosPerMillion,
    outputUsdMicrosPerMillion: typeof draft?.outputUsdMicrosPerMillion === 'string' ? draft.outputUsdMicrosPerMillion : defaults.outputUsdMicrosPerMillion,
    usdMicrosPerMillionCharacters: typeof draft?.usdMicrosPerMillionCharacters === 'string' ? draft.usdMicrosPerMillionCharacters : defaults.usdMicrosPerMillionCharacters,
  };
}

function createPolicyDrafts(policy: CreditPolicy, current: PolicyDrafts | null): PolicyDrafts {
  if (current?.version === policy.version) {
    return {
      ...current,
      providerAccountIds: Object.fromEntries(Object.entries(policy.providerAccountIds).map(([provider, accountId]) => [
        provider, current.providerAccountIds[provider] ?? accountId,
      ])),
      grantTypePolicies: Object.fromEntries(Object.entries(policy.grantTypePolicies).map(([kind, value]) => [
        kind, current.grantTypePolicies[kind] ?? {
          issuanceCap: formatCreditsInput(value.issuanceCapUsdMicros),
          carryoverCap: formatCreditsInput(value.carryoverCapUsdMicros),
          expiryDays: value.expiryDays,
        },
      ])),
      rateCards: Object.fromEntries(
        Object.entries(policy.rateCards).map(([key, card]) => [
          key,
          mergeRateCardDraft(card, current.rateCards[key]),
        ]),
      ),
    };
  }

  return {
    version: policy.version,
    providerAccountIds: { ...policy.providerAccountIds },
    grantTypePolicies: Object.fromEntries(Object.entries(policy.grantTypePolicies).map(([kind, value]) => [
      kind, {
        issuanceCap: formatCreditsInput(value.issuanceCapUsdMicros),
        carryoverCap: formatCreditsInput(value.carryoverCapUsdMicros),
        expiryDays: value.expiryDays,
      },
    ])),
    signupGrant: formatCreditsInput(policy.signupGrantUsdMicros),
    monthlyGrant: formatCreditsInput(policy.monthlyGrantUsdMicros),
    monthlyGrantsEnabled: policy.monthlyGrantsEnabled,
    rolloverCap: formatCreditsInput(policy.rolloverCapUsdMicros),
    rateCards: Object.fromEntries(
      Object.entries(policy.rateCards).map(([key, card]) => [
        key,
        createRateCardDraft(card),
      ]),
    ),
    rolloverExpiryDays: policy.rolloverExpiryDays,
    overrunMarginPercent: policy.overrunMarginPercent,
  };
}

export function AdminCreditsPage() {
  const { toast } = useToast(); const [policy, setPolicy] = useState<CreditPolicy | null>(null); const [usage, setUsage] = useState<CreditUsageResponse | null>(null);
  const [alerts, setAlerts] = useState<CreditAdminAlert[]>([]);
  const [grantPreview, setGrantPreview] = useState<MonthlyGrantPreview | null>(null);
  const [userOverride, setUserOverride] = useState<UserOverrideDraft | null>(null);
  const [manualGrantPreview, setManualGrantPreview] = useState<{
    userId: string; amountMicros: number; reason: string; balanceMicros: number;
    expiryDays: number; expiresAt: string; idempotencyKey: string;
  } | null>(null);
  const [bulkRecipientIds, setBulkRecipientIds] = useState('');
  const [bulkAmount, setBulkAmount] = useState('');
  const [bulkReason, setBulkReason] = useState('');
  const [bulkGrantPreview, setBulkGrantPreview] = useState<{
    result: GrantBatchPreview; ids: string; amount: string; reason: string; key: string;
  } | null>(null);
  const [overrideReason, setOverrideReason] = useState('');
  const [reviewReasons, setReviewReasons] = useState<Record<string, string>>({});
  const [reviewKeys, setReviewKeys] = useState<Record<string, string>>({});
  const [reconciliationTokens, setReconciliationTokens] = useState<Record<string, { input: string; cached: string; output: string; requestId: string }>>({});
  const [policyDrafts, setPolicyDrafts] = useState<PolicyDrafts | null>(null);
  const [userId, setUserId] = useState(''); const [amount, setAmount] = useState(''); const [reason, setReason] = useState(''); const [changeReason, setChangeReason] = useState('');
  const [saving, setSaving] = useState(false); const [loading, setLoading] = useState(true);
  const refreshGrantPreview = async () => {
    try { setGrantPreview(await creditApi.adminMonthlyGrantPreview()); } catch (e) {
      toast({ title: creditErrorMessage(e, 'Grant preview is unavailable'), variant: 'destructive' });
    }
  };
  const load = async () => {
    setLoading(true);
    try {
      try { setPolicy(await creditApi.adminPolicy()); } catch (e) { toast({ title: creditErrorMessage(e, 'USD tariff policy is unavailable'), variant: 'destructive' }); }
      try { setAlerts((await creditApi.adminGrantAlerts()).alerts); } catch (e) { toast({ title: creditErrorMessage(e, 'Grant alerts are unavailable'), variant: 'destructive' }); }
      await refreshGrantPreview();
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => { void load(); }, []);
  useEffect(() => {
    if (!policy) return;
    const missingVoiceAgent = !policy.rateCards[voiceAgentRateCardKey];
    const missingOpenAI = !policy.rateCards[openAIRateCardKey];
    const missingAzureSpeech = !policy.rateCards[azureSpeechRateCardKey];
    if (!missingVoiceAgent && !missingOpenAI && !missingAzureSpeech) return;
    setPolicy((current) => current ? {
      ...current,
      rateCards: {
        ...current.rateCards,
        ...(missingVoiceAgent ? { [voiceAgentRateCardKey]: {
          provider: 'assemblyai', providerAccountId: current.providerAccountIds.assemblyai ?? 'default', mode: 'voice_agent', model: 'managed-voice-agent',
          meter: 'hour', usdMicrosPerHour: null,
          inputUsdMicrosPerMillion: null, outputUsdMicrosPerMillion: null,
        } } : {}),
        ...(missingOpenAI ? { [openAIRateCardKey]: {
          provider: 'openai', providerAccountId: current.providerAccountIds.openai ?? 'default', mode: 'assistant', model: 'gpt-5.6-luna', meter: 'tokens',
          usdMicrosPerHour: null, inputUsdMicrosPerMillion: 200_000,
          cachedInputUsdMicrosPerMillion: 20_000, outputUsdMicrosPerMillion: 1_200_000,
        } } : {}),
        ...(missingAzureSpeech ? { [azureSpeechRateCardKey]: {
          provider: 'azure', providerAccountId: current.providerAccountIds.azure ?? 'default', mode: 'assistant_speech', model: 'azure-neural-tts', meter: 'characters',
          usdMicrosPerHour: null, inputUsdMicrosPerMillion: null, outputUsdMicrosPerMillion: null,
          usdMicrosPerMillionCharacters: null,
        } } : {}),
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
      const [nextUsage, nextOverride] = await Promise.all([
        creditApi.adminUsage(userId.trim()),
        creditApi.adminUserPolicyOverride(userId.trim()),
      ]);
      setUsage(nextUsage);
      setUserOverride(createUserOverrideDraft(nextOverride));
      setOverrideReason('');
    } catch (e) {
      toast({ title: creditErrorMessage(e, 'Account usage is unavailable'), variant: 'destructive' });
    }
  };
  const saveUserOverride = async () => {
    if (!userId.trim() || !userOverride || !overrideReason.trim()) return;
    const monthlyGrantUsdMicros = userOverride.monthlyGrant.trim() === ''
      ? null : tryParseCreditsToUsdMicros(userOverride.monthlyGrant);
    const rolloverCapUsdMicros = userOverride.rolloverCap.trim() === ''
      ? null : tryParseCreditsToUsdMicros(userOverride.rolloverCap);
    const grantExpiryDays = userOverride.grantExpiryDays.trim() === ''
      ? null : Number(userOverride.grantExpiryDays);
    if ((userOverride.monthlyGrant.trim() !== '' && (monthlyGrantUsdMicros === null || monthlyGrantUsdMicros < 0)) ||
        (userOverride.rolloverCap.trim() !== '' && (rolloverCapUsdMicros === null || rolloverCapUsdMicros < 0)) ||
        (grantExpiryDays !== null && (!Number.isInteger(grantExpiryDays) || grantExpiryDays < 1 || grantExpiryDays > 3650))) {
      toast({ title: 'Enter valid non-negative credit amounts for the override.', variant: 'destructive' });
      return;
    }
    setSaving(true);
    try {
      const updated = await creditApi.updateAdminUserPolicyOverride(userId.trim(), {
        monthlyGrantsEnabled: userOverride.monthlyGrantsEnabled === 'inherit'
          ? null : userOverride.monthlyGrantsEnabled === 'enabled',
        monthlyGrantUsdMicros,
        rolloverCapUsdMicros,
        grantExpiryDays,
        reason: overrideReason.trim(),
      });
      setUserOverride(createUserOverrideDraft(updated));
      setOverrideReason('');
      void refreshGrantPreview();
      toast({ title: 'User grant overrides saved' });
    } catch (e) {
      toast({ title: creditErrorMessage(e, 'User grant overrides could not be saved'), variant: 'destructive' });
    } finally {
      setSaving(false);
    }
  };
  const acknowledgeAlert = async (alert: CreditAdminAlert) => {
    try {
      await creditApi.acknowledgeAdminGrantAlert(alert.alertKey);
      setAlerts((current) => current.filter((item) => item.alertKey !== alert.alertKey));
    } catch (e) {
      toast({ title: creditErrorMessage(e, 'Alert could not be acknowledged'), variant: 'destructive' });
    }
  };
  const resolvePendingUsage = async (reservationId: string, decision: 'settle' | 'release') => {
    const reviewReason = reviewReasons[reservationId]?.trim() ?? '';
    if (!userId.trim() || !reviewReason) return;
    const key = `${reservationId}:${decision}`;
    const idempotencyKey = reviewKeys[key] ?? newCreditIdempotencyKey();
    setReviewKeys((current) => ({ ...current, [key]: idempotencyKey }));
    setSaving(true);
    try {
      await creditApi.resolveAdminPendingUsage(reservationId, {
        userId: userId.trim(), decision, reason: reviewReason, idempotencyKey,
      });
      setReviewKeys((current) => {
        const next = { ...current };
        delete next[key];
        return next;
      });
      setReviewReasons((current) => ({ ...current, [reservationId]: '' }));
      toast({ title: decision === 'settle' ? 'Pending usage settled' : 'Credit hold released' });
      await lookup();
    } catch (e) {
      toast({ title: creditErrorMessage(e, 'Pending usage could not be resolved'), variant: 'destructive' });
    } finally {
      setSaving(false);
    }
  };
  const reconcileAssistantUsage = async (reservation: CreditUsageResponse['reservations'][number]) => {
    const draft = reconciliationTokens[reservation.id] ?? {
      input: String(reservation.inputTokens), cached: String(reservation.cachedInputTokens ?? 0),
      output: String(reservation.outputTokens), requestId: '',
    };
    const reviewReason = reviewReasons[reservation.id]?.trim() ?? '';
    const inputTokens = Number(draft.input);
    const cachedInputTokens = Number(draft.cached);
    const outputTokens = Number(draft.output);
    const isVoice = reservation.provider === 'assemblyai';
    if (!userId.trim() || !reviewReason || (isVoice
      ? !Number.isInteger(inputTokens)
      : !Number.isInteger(inputTokens) || !Number.isInteger(cachedInputTokens) || !Number.isInteger(outputTokens))) return;
    const key = `${reservation.id}:reconcile`;
    const idempotencyKey = reviewKeys[key] ?? newCreditIdempotencyKey();
    setReviewKeys((current) => ({ ...current, [key]: idempotencyKey }));
    setSaving(true);
    try {
      await creditApi.resolveAdminPendingUsage(reservation.id, {
        userId: userId.trim(), decision: 'reconcile', reason: reviewReason, idempotencyKey,
        providerRequestId: draft.requestId.trim(),
        ...(isVoice ? { durationMs: inputTokens } : { inputTokens, cachedInputTokens, outputTokens }),
      });
      setReviewKeys((current) => { const next = { ...current }; delete next[key]; return next; });
      setReviewReasons((current) => ({ ...current, [reservation.id]: '' }));
      toast({ title: 'Assistant usage reconciled' });
      await lookup();
    } catch (e) {
      toast({ title: creditErrorMessage(e, 'Assistant usage could not be reconciled'), variant: 'destructive' });
    } finally {
      setSaving(false);
    }
  };
  const adjust = async () => {
    const micros = tryParseCreditsToUsdMicros(amount);
    if (!userId.trim() || micros === null || micros === 0 || !reason.trim()) return;
    if (micros > 0 && (!manualGrantPreview ||
        manualGrantPreview.userId !== userId.trim() ||
        manualGrantPreview.amountMicros !== micros ||
        manualGrantPreview.reason !== reason.trim())) {
      toast({ title: 'Preview this grant before posting it.', variant: 'destructive' });
      return;
    }
    const idempotencyKey = micros > 0 ? manualGrantPreview?.idempotencyKey : newCreditIdempotencyKey();
    if (!idempotencyKey) return;
    setSaving(true);
    try {
      await creditApi.adminAdjustment({
        userId: userId.trim(),
        amountUsdMicros: micros,
        reason: reason.trim(),
        idempotencyKey,
      });
      toast({ title: 'Credit adjustment posted', description: `${formatCredits(micros)} posted to ${userId.trim()}.` });
      setAmount('');
      setReason('');
      setManualGrantPreview(null);
      await lookup();
    } catch (e) {
      toast({ title: creditErrorMessage(e, 'Adjustment could not be posted'), variant: 'destructive' });
    } finally {
      setSaving(false);
    }
  };
  const previewManualGrant = async () => {
    const micros = tryParseCreditsToUsdMicros(amount);
    if (!userId.trim() || micros === null || micros <= 0 || !reason.trim() || !policy) return;
    setSaving(true);
    try {
      const preview = await creditApi.adminManualGrantPreview(userId.trim(), micros);
      setManualGrantPreview({
        userId: preview.userId, amountMicros: preview.amountUsdMicros, reason: reason.trim(),
        balanceMicros: preview.balanceUsdMicros, expiryDays: preview.grantExpiryDays,
        expiresAt: preview.expiresAt, idempotencyKey: newCreditIdempotencyKey(),
      });
    } catch (e) {
      toast({ title: creditErrorMessage(e, 'Grant preview is unavailable'), variant: 'destructive' });
    } finally {
      setSaving(false);
    }
  };
  const parseBulkGrantItems = (): GrantBatchItem[] | null => {
    const ids = bulkRecipientIds.split(/\r?\n/).map((value) => value.trim()).filter(Boolean);
    const amountMicros = tryParseCreditsToUsdMicros(bulkAmount);
    if (!ids.length || ids.length > 100 || new Set(ids).size !== ids.length || amountMicros === null || amountMicros <= 0) return null;
    return ids.map((recipientId) => ({ userId: recipientId, amountUsdMicros: amountMicros }));
  };
  const previewBulkGrant = async () => {
    const items = parseBulkGrantItems();
    if (!items || !bulkReason.trim()) return;
    setSaving(true);
    try {
      const result = await creditApi.adminBulkGrantPreview(items, bulkReason.trim());
      setBulkGrantPreview({
        result, ids: bulkRecipientIds.trim(), amount: bulkAmount.trim(),
        reason: bulkReason.trim(), key: newCreditIdempotencyKey(),
      });
    } catch (e) {
      toast({ title: creditErrorMessage(e, 'Bulk grant preview is unavailable'), variant: 'destructive' });
    } finally {
      setSaving(false);
    }
  };
  const issueBulkGrant = async () => {
    const items = parseBulkGrantItems();
    const preview = bulkGrantPreview;
    if (!items || !preview || preview.ids !== bulkRecipientIds.trim() ||
        preview.amount !== bulkAmount.trim() || preview.reason !== bulkReason.trim()) {
      toast({ title: 'Preview the current recipients and grant details before posting.', variant: 'destructive' });
      return;
    }
    setSaving(true);
    try {
      const result = await creditApi.adminBulkGrant(items, bulkReason.trim(), preview.key);
      setBulkGrantPreview(null);
      setBulkRecipientIds('');
      setBulkAmount('');
      setBulkReason('');
      toast({ title: 'Bulk grant issued', description: `Issued to ${result.recipientCount} recipients.` });
    } catch (e) {
      toast({ title: creditErrorMessage(e, 'Bulk grant could not be issued'), variant: 'destructive' });
    } finally {
      setSaving(false);
    }
  };
  const updatePolicyDraft = (patch: Partial<Omit<PolicyDrafts, 'version' | 'rateCards'>>) =>
    setPolicyDrafts((current) => current ? { ...current, ...patch } : current);
  const updateRateDraft = (key: string, value: string) =>
    setPolicyDrafts((current) => current ? {
      ...current,
      rateCards: {
        ...current.rateCards,
        [key]: { ...current.rateCards[key], usdMicrosPerHour: value },
      },
    } : current);
  const updateTokenRateDraft = (key: string, field: 'inputUsdMicrosPerMillion' | 'cachedInputUsdMicrosPerMillion' | 'outputUsdMicrosPerMillion', value: string) =>
    setPolicyDrafts((current) => current ? {
      ...current,
      rateCards: {
        ...current.rateCards,
        [key]: { ...current.rateCards[key], [field]: value },
      },
    } : current);
  const updateGrantTypeDraft = (kind: string, patch: Partial<PolicyDrafts['grantTypePolicies'][string]>) =>
    setPolicyDrafts((current) => current ? {
      ...current,
      grantTypePolicies: {
        ...current.grantTypePolicies,
        [kind]: { ...current.grantTypePolicies[kind], ...patch },
      },
    } : current);
  const monthlyGrantMicros = policyDrafts
    ? tryParseCreditsToUsdMicros(policyDrafts.monthlyGrant)
    : null;
  const signupGrantMicros = policyDrafts
    ? tryParseCreditsToUsdMicros(policyDrafts.signupGrant)
    : null;
  const rolloverCapMicros = policyDrafts
    ? tryParseCreditsToUsdMicros(policyDrafts.rolloverCap)
    : null;
  const rateDraftsAreValid = Boolean(policy && policyDrafts) &&
    Object.entries(policy?.rateCards ?? {}).every(([key, card]) => {
      const draft = mergeRateCardDraft(card, policyDrafts?.rateCards[key]);
      const relevant = card.meter === 'hour'
        ? [draft.usdMicrosPerHour]
        : card.meter === 'input_tokens'
          ? [draft.inputUsdMicrosPerMillion]
          : card.meter === 'output_tokens'
            ? [draft.outputUsdMicrosPerMillion]
            : card.meter === 'characters'
              ? [draft.usdMicrosPerMillionCharacters]
            : [draft.inputUsdMicrosPerMillion, draft.outputUsdMicrosPerMillion];
      const populated = relevant.filter((value) => value.trim() !== '');
      if (populated.length === 0) return true;
      if (populated.length !== relevant.length) return false;
      const cached = draft.cachedInputUsdMicrosPerMillion.trim();
      return relevant.every((value) => {
        const micros = tryParseUsdMicros(value);
        return micros !== null && micros > 0;
      }) && (!cached || ((tryParseUsdMicros(cached) ?? 0) > 0));
    });
  const grantTypeDraftsAreValid = Boolean(policyDrafts) &&
    Object.values(policyDrafts?.grantTypePolicies ?? {}).every((draft) => {
      const issuanceCap = tryParseCreditsToUsdMicros(draft.issuanceCap);
      const carryoverCap = tryParseCreditsToUsdMicros(draft.carryoverCap);
      return issuanceCap !== null && issuanceCap >= 0 && carryoverCap !== null && carryoverCap >= 0 &&
        Number.isInteger(draft.expiryDays) && draft.expiryDays >= 1 && draft.expiryDays <= 3650;
    });
  const policyCurrenciesAreValid =
    signupGrantMicros !== null &&
    signupGrantMicros >= 0 &&
    monthlyGrantMicros !== null &&
    monthlyGrantMicros >= 0 &&
    rolloverCapMicros !== null &&
    rolloverCapMicros >= 0 &&
    grantTypeDraftsAreValid &&
    rateDraftsAreValid;
  const adjustmentMicros = tryParseCreditsToUsdMicros(amount);
  const manualGrantPreviewIsCurrent = Boolean(
    manualGrantPreview &&
    manualGrantPreview.userId === userId.trim() &&
    manualGrantPreview.amountMicros === adjustmentMicros &&
    manualGrantPreview.reason === reason.trim(),
  );
  const savePolicy = async () => {
    if (!policy || !policyDrafts || !changeReason.trim()) return;
    const monthlyGrant = tryParseCreditsToUsdMicros(policyDrafts.monthlyGrant);
    const signupGrant = tryParseCreditsToUsdMicros(policyDrafts.signupGrant);
    const rolloverCap = tryParseCreditsToUsdMicros(policyDrafts.rolloverCap);
    if (
      signupGrant === null ||
      signupGrant < 0 ||
      monthlyGrant === null ||
      monthlyGrant < 0 ||
      rolloverCap === null ||
      rolloverCap < 0
    ) {
      toast({
        title: 'Check the credit amounts',
        description: 'Enter non-negative credit values with no more than three decimal places.',
        variant: 'destructive',
      });
      return;
    }
    const rateCards: CreditPolicy['rateCards'] = {};
    const grantTypePolicies: CreditPolicy['grantTypePolicies'] = {};
    for (const [kind, draft] of Object.entries(policyDrafts.grantTypePolicies)) {
      const issuanceCap = tryParseCreditsToUsdMicros(draft.issuanceCap);
      const carryoverCap = tryParseCreditsToUsdMicros(draft.carryoverCap);
      if (issuanceCap === null || issuanceCap < 0 || carryoverCap === null || carryoverCap < 0 ||
          !Number.isInteger(draft.expiryDays) || draft.expiryDays < 1 || draft.expiryDays > 3650) {
        toast({ title: `Check the ${kind} grant policy`, description: 'Caps must be non-negative and expiry must be 1–3650 days.', variant: 'destructive' });
        return;
      }
      grantTypePolicies[kind] = {
        issuanceCapUsdMicros: issuanceCap,
        carryoverCapUsdMicros: carryoverCap,
        expiryDays: draft.expiryDays,
      };
    }
    const providerAccountIds = Object.fromEntries(Object.entries(policyDrafts.providerAccountIds).map(([provider, accountId]) =>
      [provider, accountId.trim()])) as Record<string, string>;
    if (Object.values(providerAccountIds).some((accountId) => !/^[A-Za-z0-9._-]{1,120}$/.test(accountId))) {
      toast({ title: 'Enter a valid provider account ID for each configured credential.', variant: 'destructive' });
      return;
    }
    for (const [key, card] of Object.entries(policy.rateCards)) {
      const draft = mergeRateCardDraft(card, policyDrafts.rateCards[key]);
      const fields = card.meter === 'hour'
        ? [['usdMicrosPerHour', draft.usdMicrosPerHour]]
        : card.meter === 'characters'
          ? [['usdMicrosPerMillionCharacters', draft.usdMicrosPerMillionCharacters]]
        : card.meter === 'input_tokens'
          ? [['inputUsdMicrosPerMillion', draft.inputUsdMicrosPerMillion]]
          : card.meter === 'output_tokens'
            ? [['outputUsdMicrosPerMillion', draft.outputUsdMicrosPerMillion]]
            : [['inputUsdMicrosPerMillion', draft.inputUsdMicrosPerMillion], ['outputUsdMicrosPerMillion', draft.outputUsdMicrosPerMillion]];
      if (fields.every(([, value]) => !String(value).trim())) {
        continue;
      }
       const allFields = card.meter === 'tokens' && draft.cachedInputUsdMicrosPerMillion.trim()
         ? [...fields, ['cachedInputUsdMicrosPerMillion', draft.cachedInputUsdMicrosPerMillion]]
         : fields;
       const parsed = allFields.map(([field, value]) => [field, tryParseUsdMicros(String(value))] as const);
      if (parsed.some(([, micros]) => micros === null || micros <= 0)) {
        toast({
          title: 'Check the rate amount',
          description: 'Enter each required USD rate with no more than six decimal places.',
          variant: 'destructive',
        });
        return;
      }
      const updated = { ...card, providerAccountId: providerAccountIds[card.provider] ?? '' };
      if (card.meter === 'tokens' && !draft.cachedInputUsdMicrosPerMillion.trim()) {
        updated.cachedInputUsdMicrosPerMillion = null;
      }
      for (const [field, micros] of parsed) {
        if (field === 'usdMicrosPerHour') updated.usdMicrosPerHour = micros;
        if (field === 'inputUsdMicrosPerMillion') updated.inputUsdMicrosPerMillion = micros;
        if (field === 'cachedInputUsdMicrosPerMillion') updated.cachedInputUsdMicrosPerMillion = micros;
        if (field === 'outputUsdMicrosPerMillion') updated.outputUsdMicrosPerMillion = micros;
        if (field === 'usdMicrosPerMillionCharacters') updated.usdMicrosPerMillionCharacters = micros;
      }
      rateCards[key] = updated;
    }

    setSaving(true);
    try {
      const next = await creditApi.updateAdminPolicy({
        rateCards,
        providerAccountIds,
        grantTypePolicies,
        signupGrantUsdMicros: signupGrant,
        monthlyGrantUsdMicros: monthlyGrant,
        monthlyGrantsEnabled: policyDrafts.monthlyGrantsEnabled,
        rolloverCapUsdMicros: rolloverCap,
        rolloverExpiryDays: policyDrafts.rolloverExpiryDays,
        overrunMarginPercent: policyDrafts.overrunMarginPercent,
      }, policy.version, changeReason.trim());
      setPolicy(next);
      setPolicyDrafts(createPolicyDrafts(next, null));
      void refreshGrantPreview();
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
            Credit and tariff policy
          </h1>
          <p className="mt-2 max-w-2xl text-muted-foreground">
            Hourly provider rates stay in USD. Grants, adjustments, and balances use credits.
          </p>
        </div>
        <Link
          href="/settings/space/billing"
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
              Provider rates are fixed-point USD values; grant amounts and caps are credit values.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="admin-signup-grant">Verified signup grant (credits)</Label>
                <Input
                  id="admin-signup-grant"
                  type="text"
                  inputMode="decimal"
                  value={policyDrafts.signupGrant}
                  onChange={(event) => updatePolicyDraft({ signupGrant: event.target.value })}
                  aria-invalid={signupGrantMicros === null || signupGrantMicros < 0}
                  disabled={saving}
                  data-testid="input-admin-signup-grant"
                />
                <p className="text-xs text-muted-foreground">Issued once after email verification. Set to 0 to disable until configured.</p>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="admin-monthly-grant">Monthly grant (credits)</Label>
                <Input
                  id="admin-monthly-grant"
                  type="text"
                  inputMode="decimal"
                  value={policyDrafts.monthlyGrant}
                  onChange={(event) => updatePolicyDraft({ monthlyGrant: event.target.value })}
                  aria-invalid={monthlyGrantMicros === null || monthlyGrantMicros < 0}
                  disabled={saving}
                  data-testid="input-admin-monthly-grant"
                />
                <label className="flex items-start gap-2 text-sm">
                  <input
                    type="checkbox"
                    className="mt-0.5 size-4 accent-primary"
                    checked={policyDrafts.monthlyGrantsEnabled}
                    onChange={(event) => updatePolicyDraft({ monthlyGrantsEnabled: event.target.checked })}
                    disabled={saving}
                    data-testid="toggle-admin-monthly-grants"
                  />
                  <span>
                    Enable recurring grants
                    <span className="mt-0.5 block text-xs text-muted-foreground">
                      Grants run every 30 days from signup; a zero amount issues nothing.
                    </span>
                  </span>
                </label>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="admin-rollover-cap">Rollover cap (credits)</Label>
                <Input
                  id="admin-rollover-cap"
                  type="text"
                  inputMode="decimal"
                  value={policyDrafts.rolloverCap}
                  onChange={(event) => updatePolicyDraft({ rolloverCap: event.target.value })}
                  aria-invalid={rolloverCapMicros === null || rolloverCapMicros < 0}
                  disabled={saving}
                  data-testid="input-admin-rollover-cap"
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

            <section aria-labelledby="grant-type-policies-heading" className="space-y-3">
              <div>
                <h3 id="grant-type-policies-heading" className="font-medium">Grant policies by type</h3>
                <p className="text-xs text-muted-foreground">Caps are in USD-backed credits. A zero cap disables issuance or carryover for that grant type.</p>
              </div>
              <div className="grid gap-3 lg:grid-cols-3">
                {Object.entries(policyDrafts.grantTypePolicies).map(([kind, draft]) => (
                  <div key={kind} className="space-y-2 rounded-lg border border-border/70 p-3">
                    <h4 className="font-medium capitalize">{kind} grants</h4>
                    <div className="space-y-1.5">
                      <Label htmlFor={`grant-${kind}-issuance-cap`}>Maximum grant amount (credits)</Label>
                      <Input id={`grant-${kind}-issuance-cap`} inputMode="decimal" value={draft.issuanceCap}
                        onChange={(event) => updateGrantTypeDraft(kind, { issuanceCap: event.target.value })} disabled={saving} />
                    </div>
                    <div className="space-y-1.5">
                      <Label htmlFor={`grant-${kind}-carryover-cap`}>Maximum carried balance (credits)</Label>
                      <Input id={`grant-${kind}-carryover-cap`} inputMode="decimal" value={draft.carryoverCap}
                        onChange={(event) => updateGrantTypeDraft(kind, { carryoverCap: event.target.value })} disabled={saving} />
                    </div>
                    <div className="space-y-1.5">
                      <Label htmlFor={`grant-${kind}-expiry`}>Expiry (days)</Label>
                      <Input id={`grant-${kind}-expiry`} type="number" min={1} max={3650} step={1} value={draft.expiryDays}
                        onChange={(event) => updateGrantTypeDraft(kind, { expiryDays: Math.trunc(Number(event.target.value) || 0) })} disabled={saving} />
                    </div>
                  </div>
                ))}
              </div>
            </section>
            <div className="grid gap-4 sm:grid-cols-3">
              {Object.entries(policyDrafts.providerAccountIds).map(([provider, accountId]) => (
                <div key={provider} className="space-y-1.5">
                  <Label htmlFor={`provider-account-${provider}`}>{provider} account ID</Label>
                  <Input
                    id={`provider-account-${provider}`}
                    value={accountId}
                    onChange={(event) => updatePolicyDraft({
                      providerAccountIds: { ...policyDrafts.providerAccountIds, [provider]: event.target.value },
                    })}
                    placeholder="Configured account ID"
                    disabled={saving}
                    maxLength={120}
                  />
                  <p className="text-xs text-muted-foreground">Identifies the already-connected credential for rate selection; it is not a credential.</p>
                </div>
              ))}
            </div>
            <div className="space-y-2">
              {Object.entries(policy.rateCards).map(([key, card], index) => {
                const draft = mergeRateCardDraft(card, policyDrafts.rateCards[key]);
                const invalidRate = (value: string | undefined) =>
                  (value ?? '').trim() !== '' &&
                  ((tryParseUsdMicros(value ?? '') ?? 0) <= 0);
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
                    {(card.meter === 'hour') && (
                      <>
                        <Label htmlFor={rateId}>Rate per hour (USD)</Label>
                        <CInputGroup9CurrencyInput
                          id={rateId}
                          value={draft.usdMicrosPerHour}
                          onValueChange={(value) => updateRateDraft(key, value)}
                          placeholder="Not configured"
                          ariaLabel={`${key} USD per hour`}
                          ariaInvalid={invalidRate(draft.usdMicrosPerHour)}
                          disabled={saving}
                          testId={`input-admin-rate-${index}`}
                        />
                      </>
                    )}
                    {card.meter === 'characters' && (
                      <div className="space-y-1.5">
                        <Label htmlFor={`${rateId}-characters`}>Rate per million characters (USD)</Label>
                        <CInputGroup9CurrencyInput
                          id={`${rateId}-characters`}
                          value={draft.usdMicrosPerMillionCharacters}
                          onValueChange={(value) => setPolicyDrafts((current) => current ? ({
                            ...current,
                            rateCards: {
                              ...current.rateCards,
                              [key]: { ...current.rateCards[key], usdMicrosPerMillionCharacters: value },
                            },
                          }) : current)}
                          placeholder="Not configured"
                          ariaLabel={`${key} USD per million characters`}
                          ariaInvalid={invalidRate(draft.usdMicrosPerMillionCharacters)}
                          disabled={saving}
                          testId={`input-admin-rate-${index}-characters`}
                        />
                      </div>
                    )}
                    {(card.meter === 'input_tokens' || card.meter === 'tokens') && (
                      <div className="space-y-1.5">
                        <Label htmlFor={`${rateId}-input`}>Input rate per million tokens (USD)</Label>
                        <CInputGroup9CurrencyInput
                          id={`${rateId}-input`}
                          value={draft.inputUsdMicrosPerMillion}
                          onValueChange={(value) => updateTokenRateDraft(key, 'inputUsdMicrosPerMillion', value)}
                          placeholder="Not configured"
                          ariaLabel={`${key} input USD per million tokens`}
                          ariaInvalid={invalidRate(draft.inputUsdMicrosPerMillion)}
                          disabled={saving}
                          testId={`input-admin-rate-${index}-input`}
                        />
                      </div>
                    )}
                    {card.provider === 'openai' && (card.meter === 'input_tokens' || card.meter === 'tokens') && (
                      <div className="space-y-1.5">
                        <Label htmlFor={`${rateId}-cached-input`}>Cached input rate per million tokens (USD)</Label>
                        <CInputGroup9CurrencyInput
                          id={`${rateId}-cached-input`}
                          value={draft.cachedInputUsdMicrosPerMillion}
                          onValueChange={(value) => updateTokenRateDraft(key, 'cachedInputUsdMicrosPerMillion', value)}
                          placeholder="Optional; uses standard input rate when blank"
                          ariaLabel={`${key} cached input USD per million tokens`}
                          ariaInvalid={invalidRate(draft.cachedInputUsdMicrosPerMillion)}
                          disabled={saving}
                          testId={`input-admin-rate-${index}-cached-input`}
                        />
                      </div>
                    )}
                    {(card.meter === 'output_tokens' || card.meter === 'tokens') && (
                      <div className="space-y-1.5">
                        <Label htmlFor={`${rateId}-output`}>Output rate per million tokens (USD)</Label>
                        <CInputGroup9CurrencyInput
                          id={`${rateId}-output`}
                          value={draft.outputUsdMicrosPerMillion}
                          onValueChange={(value) => updateTokenRateDraft(key, 'outputUsdMicrosPerMillion', value)}
                          placeholder="Not configured"
                          ariaLabel={`${key} output USD per million tokens`}
                          ariaInvalid={invalidRate(draft.outputUsdMicrosPerMillion)}
                          disabled={saving}
                          testId={`input-admin-rate-${index}-output`}
                        />
                      </div>
                    )}
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
            <CardTitle>Adjust credits</CardTitle>
            <CardDescription>
              Enter a signed credit amount, such as 500 or -25.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="admin-user">User ID</Label>
              <Input
                id="admin-user"
                value={userId}
                onChange={(event) => {
                  setUserId(event.target.value);
                  setUsage(null);
                  setUserOverride(null);
                  setManualGrantPreview(null);
                }}
                placeholder="usr_…"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="admin-amount">Amount (credits)</Label>
              <Input
                id="admin-amount"
                type="text"
                inputMode="decimal"
                value={amount}
                onChange={(event) => {
                  setAmount(event.target.value);
                  setManualGrantPreview(null);
                }}
                placeholder="500 or -25"
                aria-label="Signed adjustment amount in credits"
                disabled={saving}
                data-testid="input-admin-adjustment-amount"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="admin-reason">Reason</Label>
              <Input
                id="admin-reason"
                value={reason}
                onChange={(event) => {
                  setReason(event.target.value);
                  setManualGrantPreview(null);
                }}
                placeholder="Customer support adjustment"
              />
            </div>
            {adjustmentMicros !== null && adjustmentMicros > 0 && (
              <>
                {manualGrantPreviewIsCurrent && manualGrantPreview && (
                  <div role="status" aria-live="polite" className="rounded-lg border border-primary/30 bg-primary/5 p-3 text-sm">
                    <p className="font-medium">Grant preview</p>
                    <p className="mt-1">Current balance: {formatCredits(manualGrantPreview.balanceMicros)}</p>
                    <p>After grant: {formatCredits(manualGrantPreview.balanceMicros + manualGrantPreview.amountMicros)}</p>
                    <p>Expires in {manualGrantPreview.expiryDays} days ({new Date(manualGrantPreview.expiresAt).toLocaleDateString()})</p>
                    <p className="mt-1 text-xs text-muted-foreground">Confirm the recipient, amount, and expiry before posting. Any edit requires a new preview.</p>
                  </div>
                )}
                <Button
                  type="button"
                  variant={manualGrantPreviewIsCurrent ? 'default' : 'outline'}
                  className="w-full"
                  onClick={() => manualGrantPreviewIsCurrent ? void adjust() : void previewManualGrant()}
                  disabled={saving || !userId.trim() || !reason.trim() || adjustmentMicros === null}
                >
                  {saving && <Loader2 className="me-2 h-4 w-4 animate-spin" />}
                  {manualGrantPreviewIsCurrent ? 'Confirm and post grant' : 'Preview grant'}
                </Button>
              </>
            )}
            <Button
              className="w-full"
              onClick={() => void adjust()}
              disabled={
                saving ||
                !userId.trim() ||
                !reason.trim() ||
                adjustmentMicros === null ||
                adjustmentMicros >= 0
              }
            >
              {saving && <Loader2 className="me-2 h-4 w-4 animate-spin" />}
              Post debit adjustment
            </Button>
            <Button
              variant="outline"
              className="w-full"
              onClick={() => void lookup()}
              disabled={!userId.trim() || saving}
            >
              Inspect account
            </Button>
            <div className="space-y-3 border-t border-border pt-4">
              <div>
                <h3 className="font-medium">Bulk grant</h3>
                <p className="text-xs text-muted-foreground">One user ID per line; up to 100 recipients. Review the full preview before issuing.</p>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="admin-bulk-user-ids">Recipient user IDs</Label>
                <textarea
                  id="admin-bulk-user-ids"
                  rows={4}
                  value={bulkRecipientIds}
                  onChange={(event) => { setBulkRecipientIds(event.target.value); setBulkGrantPreview(null); }}
                  className="flex min-h-24 w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  disabled={saving}
                  data-testid="input-admin-bulk-recipient-ids"
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="admin-bulk-amount">Amount per recipient (credits)</Label>
                <Input id="admin-bulk-amount" inputMode="decimal" value={bulkAmount}
                  onChange={(event) => { setBulkAmount(event.target.value); setBulkGrantPreview(null); }}
                  disabled={saving} data-testid="input-admin-bulk-amount" />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="admin-bulk-reason">Reason</Label>
                <Input id="admin-bulk-reason" value={bulkReason}
                  onChange={(event) => { setBulkReason(event.target.value); setBulkGrantPreview(null); }}
                  disabled={saving} />
              </div>
              {bulkGrantPreview && bulkGrantPreview.ids === bulkRecipientIds.trim() &&
                bulkGrantPreview.amount === bulkAmount.trim() && bulkGrantPreview.reason === bulkReason.trim() && (
                  <div role="status" aria-live="polite" className="max-h-64 space-y-1 overflow-auto rounded-lg border border-primary/30 bg-primary/5 p-3 text-sm">
                    <p className="font-medium">{bulkGrantPreview.result.items.length} recipients · total {formatCredits(bulkGrantPreview.result.totalAmountUsdMicros)}</p>
                    {bulkGrantPreview.result.items.map((item) => (
                      <p key={item.userId}>{item.userId}: {formatCredits(item.balanceUsdMicros)} → {formatCredits(item.projectedBalanceUsdMicros)} · expires {new Date(item.expiresAt).toLocaleDateString()}</p>
                    ))}
                  </div>
                )}
              <Button type="button" variant={bulkGrantPreview && bulkGrantPreview.ids === bulkRecipientIds.trim() &&
                bulkGrantPreview.amount === bulkAmount.trim() && bulkGrantPreview.reason === bulkReason.trim() ? 'default' : 'outline'}
                className="w-full" disabled={saving || !bulkReason.trim()}
                onClick={() => bulkGrantPreview && bulkGrantPreview.ids === bulkRecipientIds.trim() &&
                  bulkGrantPreview.amount === bulkAmount.trim() && bulkGrantPreview.reason === bulkReason.trim()
                  ? void issueBulkGrant() : void previewBulkGrant()}>
                {bulkGrantPreview && bulkGrantPreview.ids === bulkRecipientIds.trim() &&
                  bulkGrantPreview.amount === bulkAmount.trim() && bulkGrantPreview.reason === bulkReason.trim()
                  ? 'Confirm and issue bulk grant' : 'Preview bulk grant'}
              </Button>
            </div>
            {userOverride && (
              <div className="space-y-3 border-t border-border pt-4">
                <div>
                  <h3 className="font-medium">Recurring grant overrides</h3>
                  <p className="text-xs text-muted-foreground">Blank values inherit global settings. The global toggle remains the master switch; zero disables that grant or cap.</p>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="admin-user-monthly-enabled">Recurring grants</Label>
                  <select
                    id="admin-user-monthly-enabled"
                    className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                    value={userOverride.monthlyGrantsEnabled}
                    onChange={(event) => setUserOverride((current) => current ? ({
                      ...current,
                      monthlyGrantsEnabled: event.target.value as UserOverrideDraft['monthlyGrantsEnabled'],
                    }) : current)}
                    disabled={saving}
                  >
                    <option value="inherit">Use global setting</option>
                    <option value="enabled">Enabled for this user</option>
                    <option value="disabled">Disabled for this user</option>
                  </select>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="admin-user-monthly-grant">Monthly grant (credits)</Label>
                  <Input
                    id="admin-user-monthly-grant"
                    inputMode="decimal"
                    value={userOverride.monthlyGrant}
                    onChange={(event) => setUserOverride((current) => current ? ({ ...current, monthlyGrant: event.target.value }) : current)}
                    placeholder="Inherit global amount"
                    disabled={saving}
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="admin-user-rollover-cap">Rollover cap (credits)</Label>
                  <Input
                    id="admin-user-rollover-cap"
                    inputMode="decimal"
                    value={userOverride.rolloverCap}
                    onChange={(event) => setUserOverride((current) => current ? ({ ...current, rolloverCap: event.target.value }) : current)}
                    placeholder="Inherit global cap"
                    disabled={saving}
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="admin-user-grant-expiry-days">Grant expiry (days)</Label>
                  <Input
                    id="admin-user-grant-expiry-days"
                    type="number"
                    min={1}
                    max={3650}
                    step={1}
                    value={userOverride.grantExpiryDays}
                    onChange={(event) => setUserOverride((current) => current ? ({ ...current, grantExpiryDays: event.target.value }) : current)}
                    placeholder={`Inherit global (${policy.rolloverExpiryDays} days)`}
                    disabled={saving}
                    data-testid="input-admin-user-grant-expiry-days"
                  />
                  <p className="text-xs text-muted-foreground">Applies to future recurring and manual grants for this user. Blank inherits the global expiry.</p>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="admin-user-override-reason">Reason for override</Label>
                  <Input
                    id="admin-user-override-reason"
                    value={overrideReason}
                    onChange={(event) => setOverrideReason(event.target.value)}
                    maxLength={160}
                    placeholder="Reason for this user-specific change"
                    disabled={saving}
                  />
                </div>
                <Button
                  className="w-full"
                  onClick={() => void saveUserOverride()}
                  disabled={saving || !overrideReason.trim()}
                >
                  Save user overrides
                </Button>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {grantPreview && (
        <Card aria-labelledby="monthly-grant-preview-title">
          <CardHeader>
            <CardTitle id="monthly-grant-preview-title">Recurring grant preview</CardTitle>
            <CardDescription>
              Accounts with a cycle due now or within the next {grantPreview.windowDays} days, using the current global and user-specific settings.
            </CardDescription>
          </CardHeader>
          <CardContent className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {[
              ['Cycles due', grantPreview.dueCount],
              ['Would issue', grantPreview.issuableCount],
              ['Paused by cap', grantPreview.deferredCount],
              ['Skipped', grantPreview.skippedCount],
            ].map(([label, value]) => (
              <div key={label} className="rounded-lg border border-border/60 p-3">
                <p className="text-xs text-muted-foreground">{label}</p>
                <p className="font-mono text-xl">{value}</p>
              </div>
            ))}
          </CardContent>
        </Card>
      )}

      {alerts.length > 0 && (
        <Card aria-labelledby="credit-alerts-title">
          <CardHeader>
            <CardTitle id="credit-alerts-title">Paused recurring grants</CardTitle>
            <CardDescription>These grants are waiting for the account balance to fall within its rollover cap.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {alerts.map((alert) => (
              <div key={alert.alertKey} className="flex flex-col gap-3 rounded-lg border border-border/70 p-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="min-w-0">
                  <p className="font-medium">Grant deferred for <bdi dir="ltr">{alert.userId ?? 'unknown user'}</bdi></p>
                  <p className="text-xs text-muted-foreground">Scheduled {new Date(String(alert.details.scheduledAt ?? alert.createdAt)).toLocaleString()}</p>
                </div>
                <Button variant="outline" onClick={() => void acknowledgeAlert(alert)} disabled={saving}>Acknowledge</Button>
              </div>
            ))}
          </CardContent>
        </Card>
      )}

      {usage && (
        <Card>
          <CardHeader>
            <CardTitle>Account ledger: {userId}</CardTitle>
            <CardDescription>
              Current account counters and immutable entries.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {usage.reservations.some((reservation) => reservation.usageStatus === 'pending_review') && (
              <section aria-labelledby="pending-usage-title" className="space-y-3 rounded-lg border border-amber-500/40 bg-amber-500/5 p-3">
                <div>
                  <h3 id="pending-usage-title" className="font-medium">Pending Azure usage review</h3>
                  <p className="text-sm text-muted-foreground">
                    Settle using the saved character rate, or release the hold if the provider rejected the request.
                  </p>
                </div>
                {usage.reservations.filter((reservation) => reservation.usageStatus === 'pending_review').map((reservation) => (
                  <div key={reservation.id} className="space-y-2 rounded-md border border-border/60 bg-background p-3">
                    <p className="text-sm">
                      {reservation.usageUnit === 'characters' && reservation.usageUnits > 0
                        ? <><bdi dir="ltr">{reservation.usageUnits.toLocaleString()}</bdi> characters</>
                        : 'Character metering unavailable'} ·
                      {' '}hold {formatCredits(reservation.reservedUsdMicros)} credits · {reservation.createdAt}
                    </p>
                    <Label htmlFor={`review-reason-${reservation.id}`}>Review reason</Label>
                    <Input
                      id={`review-reason-${reservation.id}`}
                      value={reviewReasons[reservation.id] ?? ''}
                      onChange={(event) => setReviewReasons((current) => ({ ...current, [reservation.id]: event.target.value }))}
                      maxLength={160}
                      placeholder="What did you verify?"
                      disabled={saving}
                    />
                    <div className="flex flex-col gap-2 sm:flex-row">
                      <Button
                        onClick={() => void resolvePendingUsage(reservation.id, 'settle')}
                        disabled={saving || !(reviewReasons[reservation.id] ?? '').trim() || reservation.usageUnit !== 'characters' || reservation.usageUnits <= 0}
                      >
                        Settle using saved rate
                      </Button>
                      <Button
                        variant="outline"
                        onClick={() => void resolvePendingUsage(reservation.id, 'release')}
                        disabled={saving || !(reviewReasons[reservation.id] ?? '').trim()}
                      >
                        Release hold
                      </Button>
                    </div>
                  </div>
                ))}
              </section>
            )}
            {usage.reservations.some((reservation) => reservation.usageStatus === 'estimated') && (
              <section aria-labelledby="estimated-usage-title" className="space-y-3 rounded-lg border border-sky-500/40 bg-sky-500/5 p-3">
                <div>
                  <h3 id="estimated-usage-title" className="font-medium">Estimated assistant usage</h3>
                  <p className="text-sm text-muted-foreground">
                    Replace proxy estimates with provider-reported token counts. Reconciliation is limited to the original operation cap.
                  </p>
                </div>
                {usage.reservations.filter((reservation) => reservation.usageStatus === 'estimated').map((reservation) => {
                  const draft = reconciliationTokens[reservation.id] ?? {
                    input: String(reservation.provider === 'assemblyai' ? reservation.usageUnits : reservation.inputTokens),
                    cached: String(reservation.cachedInputTokens ?? 0),
                    output: String(reservation.outputTokens),
                    requestId: '',
                  };
                  const updateDraft = (key: keyof typeof draft, value: string) => setReconciliationTokens((current) => ({
                    ...current, [reservation.id]: { ...draft, [key]: value },
                  }));
                  return (
                    <div key={reservation.id} className="space-y-3 rounded-md border border-border/60 bg-background p-3">
                      <p className="text-sm">
                        {reservation.provider === 'assemblyai'
                          ? `Estimated session duration ${reservation.usageUnits.toLocaleString()} ms`
                          : `Estimated ${reservation.inputTokens.toLocaleString()} input / ${reservation.outputTokens.toLocaleString()} output tokens`} ·
                        {' '}settled {formatCredits(reservation.settledUsdMicros)} credits · {reservation.createdAt}
                      </p>
                      {reservation.provider === 'assemblyai' ? (
                        <div className="space-y-1">
                          <Label htmlFor={`reconcile-duration-${reservation.id}`}>Provider duration (milliseconds)</Label>
                          <Input id={`reconcile-duration-${reservation.id}`} type="number" min={1} max={180000} step={1}
                            value={draft.input} onChange={(event) => updateDraft('input', event.target.value)} disabled={saving} />
                        </div>
                      ) : <div className="grid gap-3 sm:grid-cols-3">
                        <div className="space-y-1">
                          <Label htmlFor={`reconcile-input-${reservation.id}`}>Provider input tokens</Label>
                          <Input id={`reconcile-input-${reservation.id}`} type="number" min={1} max={4096} step={1}
                            value={draft.input} onChange={(event) => updateDraft('input', event.target.value)} disabled={saving} />
                        </div>
                        <div className="space-y-1">
                          <Label htmlFor={`reconcile-cached-${reservation.id}`}>Cached input tokens</Label>
                          <Input id={`reconcile-cached-${reservation.id}`} type="number" min={0} max={4096} step={1}
                            value={draft.cached} onChange={(event) => updateDraft('cached', event.target.value)} disabled={saving} />
                        </div>
                        <div className="space-y-1">
                          <Label htmlFor={`reconcile-output-${reservation.id}`}>Provider output tokens</Label>
                          <Input id={`reconcile-output-${reservation.id}`} type="number" min={1} max={256} step={1}
                            value={draft.output} onChange={(event) => updateDraft('output', event.target.value)} disabled={saving} />
                        </div>
                      </div>}
                      <div className="space-y-1">
                        <Label htmlFor={`reconcile-request-${reservation.id}`}>Provider request ID (optional)</Label>
                        <Input id={`reconcile-request-${reservation.id}`} maxLength={200} value={draft.requestId}
                          onChange={(event) => updateDraft('requestId', event.target.value)} disabled={saving} />
                      </div>
                      <Label htmlFor={`review-reason-${reservation.id}`}>Reconciliation reason</Label>
                      <Input id={`review-reason-${reservation.id}`} value={reviewReasons[reservation.id] ?? ''}
                        onChange={(event) => setReviewReasons((current) => ({ ...current, [reservation.id]: event.target.value }))}
                        maxLength={160} placeholder="What provider usage did you verify?" disabled={saving} />
                      <Button onClick={() => void reconcileAssistantUsage(reservation)}
                        disabled={saving || !(reviewReasons[reservation.id] ?? '').trim()}>
                        Reconcile provider usage
                      </Button>
                    </div>
                  );
                })}
              </section>
            )}
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-6">
              {Object.entries(usage.usage)
                .filter(([key]) => key.endsWith('Micros'))
                .map(([key, value]) => (
                  <div key={key}>
                    <p className="text-xs text-muted-foreground">
                      {key.replace('UsdMicros', '')}
                    </p>
                    <p className="font-mono text-lg">
                      {formatCredits(value as number)}
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
              {usage.grants.length > 0 && (
                <section aria-labelledby="admin-grants-title" className="space-y-2">
                  <h3 id="admin-grants-title" className="font-medium">Grant history</h3>
                  {usage.grants.map((grant) => (
                    <div key={grant.id} className="rounded-lg border border-border/60 p-3 text-sm" data-testid={`grant-history-${grant.id}`}>
                      <p className="font-medium">{grant.reason} · {formatCredits(grant.amountUsdMicros)}</p>
                      <p className="text-xs text-muted-foreground">
                        {grant.createdAt} · {grant.expiresAt ? `Expires ${new Date(grant.expiresAt).toLocaleString()}` : 'Does not expire'}
                      </p>
                    </div>
                  ))}
                </section>
              )}
            </div>
          </CardContent>
        </Card>
      )}
    </PageTransition>
  );
}