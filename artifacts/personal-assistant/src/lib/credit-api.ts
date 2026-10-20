const basePath = import.meta.env?.BASE_URL?.replace(/\/$/, '') ?? '';

type CreditRequestOptions = RequestInit & { body?: BodyInit | null };
export type UsdMicros = number;

export type CreditUsage = {
  currency: 'USD';
  balanceUsdMicros: UsdMicros;
  grantedUsdMicros: UsdMicros;
  adjustmentsUsdMicros: UsdMicros;
  reservedUsdMicros: UsdMicros;
  spentUsdMicros: UsdMicros;
  refundedUsdMicros: UsdMicros;
};
export type CreditReservation = {
  id: string; operationType: string; provider: string; model: string; mode: string;
  status: string; reservedUsdMicros: UsdMicros; settledUsdMicros: UsdMicros;
  refundedUsdMicros: UsdMicros; usageUnit: string; usageUnits: number;
  inputTokens: number; cachedInputTokens?: number; outputTokens: number;
  usageStatus: 'measured' | 'estimated' | 'unavailable' | 'pending_review';
  rateCard: RateCard; policyVersion: number; expiresAt: string | null; createdAt: string;
};
export type CreditEvent = {
  id: string; reservationId: string; eventType: string; amountUsdMicros: UsdMicros;
  usageUnit: string; usageUnits: number; details: unknown; createdAt: string;
};
export type CreditAdjustment = { id: number; userId: string; amountUsdMicros: UsdMicros; reason: string; actorUserId: string; reversalOfId: number | null; createdAt: string };
export type CreditGrant = { id: number; userId: string; amountUsdMicros: UsdMicros; reason: string; actorUserId: string; createdAt: string; expiresAt: string | null };
export type CreditUsageResponse = {
  usage: CreditUsage;
  reservations: CreditReservation[];
  events: CreditEvent[];
  adjustments: CreditAdjustment[];
  grants: CreditGrant[];
};
export type CreditAccountResponse = CreditUsage & {
  policyVersion: number; canManage: boolean; enforcement: 'strict' | string;
};
export type CreditEstimate = {
  pricingKey: string; units: number; unit: string; estimatedUsdMicros: UsdMicros;
  hardCapUsdMicros: UsdMicros; availableUsdMicros: UsdMicros; policyVersion: number;
  canReserve: boolean; overrunMarginPercent: number; currency: 'USD';
};
export type RateCard = {
  provider: string; providerAccountId: string; mode: string; model: string; meter: string;
  usdMicrosPerHour: UsdMicros | null; inputUsdMicrosPerMillion: UsdMicros | null;
  cachedInputUsdMicrosPerMillion?: UsdMicros | null;
  outputUsdMicrosPerMillion: UsdMicros | null;
  usdMicrosPerMillionCharacters?: UsdMicros | null;
};
export type CreditPolicy = {
  version: number; rateCards: Record<string, RateCard>; providerAccountIds: Record<string, string>;
  grantTypePolicies: Record<string, { issuanceCapUsdMicros: UsdMicros; carryoverCapUsdMicros: UsdMicros; expiryDays: number }>;
  signupGrantUsdMicros: UsdMicros; monthlyGrantUsdMicros: UsdMicros; rolloverCapUsdMicros: UsdMicros;
  monthlyGrantsEnabled: boolean;
  rolloverExpiryDays: number; overrunMarginPercent: number;
  changeReason?: string; createdBy?: string; createdAt?: string;
};
export type CreditUserPolicyOverride = {
  userId: string;
  monthlyGrantsEnabled: boolean | null;
  monthlyGrantUsdMicros: UsdMicros | null;
  rolloverCapUsdMicros: UsdMicros | null;
  grantExpiryDays: number | null;
  updatedBy?: string;
  reason?: string;
  updatedAt?: string;
};
export type CreditAdminAlert = {
  alertKey: string;
  alertType: string;
  userId?: string | null;
  details: Record<string, unknown>;
  createdAt: string;
};
export type MonthlyGrantPreview = {
  windowDays: number;
  dueCount: number;
  issuableCount: number;
  deferredCount: number;
  skippedCount: number;
};
export type ManualGrantPreview = {
  userId: string; amountUsdMicros: UsdMicros; balanceUsdMicros: UsdMicros;
  projectedBalanceUsdMicros: UsdMicros; grantExpiryDays: number;
  expiresAt: string; policyVersion: number;
};
export type GrantBatchItem = { userId: string; amountUsdMicros: UsdMicros };
export type GrantBatchPreview = { items: ManualGrantPreview[]; totalAmountUsdMicros: UsdMicros };
export type AdminAdjustmentInput = { userId: string; amountUsdMicros: UsdMicros; reason: string; idempotencyKey: string };
export type CreditReceiptDetails = {
  reservationId: string; reservedUsdMicros: UsdMicros; settledUsdMicros: UsdMicros;
  refundedUsdMicros: UsdMicros; balanceUsdMicros: UsdMicros; policyVersion: number;
  usageUnit: string; usageUnits: number;
};

async function request<T>(path: string, options: CreditRequestOptions = {}): Promise<T> {
  const response = await fetch(`${basePath}${path}`, {
    credentials: 'include', ...options,
    headers: { Accept: 'application/json', ...(options.body ? { 'Content-Type': 'application/json' } : {}), ...options.headers },
  });
  const payload = await response.json().catch(() => null) as { error?: string; code?: string; requestId?: string } | T | null;
  if (!response.ok) {
    const message = payload && typeof payload === 'object' && 'error' in payload && typeof payload.error === 'string' ? payload.error : `Request failed (${response.status})`;
    const error = new Error(message);
    Object.assign(error, { status: response.status, code: payload && typeof payload === 'object' && 'code' in payload ? payload.code : undefined, requestId: payload && typeof payload === 'object' && 'requestId' in payload ? payload.requestId : undefined });
    throw error;
  }
  return payload as T;
}

export const creditApi = {
  account: () => request<CreditAccountResponse>('/api/ai/credits'),
  usage: () => request<CreditUsageResponse>('/api/ai/credits/usage'),
  estimate: (pricingKey: 'voice.recorded' | 'voice.realtime' | 'voice.agent' | 'assistant' | 'assistant.speech', units: number) => request<CreditEstimate>(`/api/ai/credits/estimate?pricingKey=${encodeURIComponent(pricingKey)}&units=${Math.max(0, Math.trunc(units))}`),
  adminPolicy: () => request<CreditPolicy>('/api/admin/ai-credit-policy'),
  adminUserPolicyOverride: (userId: string) => request<CreditUserPolicyOverride>(`/api/admin/ai-credit-user-overrides/${encodeURIComponent(userId)}`),
  updateAdminUserPolicyOverride: (userId: string, input: Omit<CreditUserPolicyOverride, 'userId' | 'updatedBy' | 'updatedAt'> & { reason: string }) => request<CreditUserPolicyOverride>(`/api/admin/ai-credit-user-overrides/${encodeURIComponent(userId)}`, { method: 'PUT', body: JSON.stringify(input) }),
  adminGrantAlerts: () => request<{ alerts: CreditAdminAlert[] }>('/api/admin/ai-credit-grant-alerts'),
  adminMonthlyGrantPreview: () => request<MonthlyGrantPreview>('/api/admin/ai-credit-monthly-grants/preview'),
  adminManualGrantPreview: (userId: string, amountUsdMicros: number) => request<ManualGrantPreview>(`/api/admin/ai-credit-grants/preview?userId=${encodeURIComponent(userId)}&amountUsdMicros=${amountUsdMicros}`),
  adminBulkGrantPreview: (items: GrantBatchItem[], reason: string) => request<GrantBatchPreview>('/api/admin/ai-credit-grants/bulk/preview', { method: 'POST', body: JSON.stringify({ items, reason }) }),
  adminBulkGrant: (items: GrantBatchItem[], reason: string, idempotencyKey: string) => request<{ status: string; recipientCount: number }>('/api/admin/ai-credit-grants/bulk', { method: 'POST', body: JSON.stringify({ items, reason, idempotencyKey }) }),
  acknowledgeAdminGrantAlert: (alertKey: string) => request<{ status: string }>(`/api/admin/ai-credit-grant-alerts/${encodeURIComponent(alertKey)}/acknowledge`, { method: 'POST' }),
  updateAdminPolicy: (policy: Pick<CreditPolicy, 'rateCards' | 'providerAccountIds' | 'grantTypePolicies' | 'signupGrantUsdMicros' | 'monthlyGrantUsdMicros' | 'monthlyGrantsEnabled' | 'rolloverCapUsdMicros' | 'rolloverExpiryDays' | 'overrunMarginPercent'>, expectedVersion: number, changeReason: string) => request<CreditPolicy>('/api/admin/ai-credit-policy', { method: 'PATCH', body: JSON.stringify({ ...policy, expectedVersion, changeReason }) }),
  adminAdjustment: (input: AdminAdjustmentInput) => request<CreditUsage>('/api/admin/ai-credit-adjustments', { method: 'POST', body: JSON.stringify(input) }),
  adminReverseAdjustment: (id: number, input: Pick<AdminAdjustmentInput, 'userId' | 'amountUsdMicros' | 'reason' | 'idempotencyKey'>) => request<CreditUsage>(`/api/admin/ai-credit-adjustments/${id}/reverse`, { method: 'POST', body: JSON.stringify(input) }),
  adminReverseGrant: (id: number, input: Pick<AdminAdjustmentInput, 'userId' | 'amountUsdMicros' | 'reason' | 'idempotencyKey'>) => request<CreditUsage>(`/api/admin/ai-credit-grants/${id}/reverse`, { method: 'POST', body: JSON.stringify(input) }),
  adminRefundReservation: (id: string, input: Pick<AdminAdjustmentInput, 'userId' | 'amountUsdMicros' | 'reason' | 'idempotencyKey'>) => request<{ status: string; reason: string }>(`/api/admin/ai-credit-reservations/${encodeURIComponent(id)}/refund`, { method: 'POST', body: JSON.stringify(input) }),
  resolveAdminPendingUsage: (id: string, input: { userId: string; decision: 'settle' | 'release' | 'reconcile'; reason: string; idempotencyKey: string; providerRequestId?: string; inputTokens?: number; cachedInputTokens?: number; outputTokens?: number; durationMs?: number }) => request<{ status: string; decision: string }>(`/api/admin/ai-credit-reservations/${encodeURIComponent(id)}/resolve`, { method: 'POST', body: JSON.stringify(input) }),
  adminUsage: (userId: string) => request<CreditUsageResponse>(`/api/admin/ai-credit-usage/${encodeURIComponent(userId)}`),
};

export function formatUsdMicros(micros: number, locale = 'en-US'): string {
  const safe = Number.isSafeInteger(micros) ? micros : 0;
  const sign = safe < 0 ? '-' : '';
  const absolute = Math.abs(safe);
  const dollars = Math.floor(absolute / 1_000_000);
  const fraction = String(absolute % 1_000_000).padStart(6, '0').replace(/0+$/, '').padEnd(2, '0');
  return `${sign}$${new Intl.NumberFormat(locale).format(dollars)}.${fraction}`;
}

export function formatUsdMicrosInput(micros: number): string {
  const safe = Number.isSafeInteger(micros) ? micros : 0;
  const sign = safe < 0 ? '-' : '';
  const absolute = Math.abs(safe);
  const dollars = Math.floor(absolute / 1_000_000);
  const fraction = String(absolute % 1_000_000).padStart(6, '0').replace(/0+$/, '').padEnd(2, '0');
  return `${sign}${dollars}.${fraction}`;
}

const usdMicrosPerCredit = 1_000;

export function formatCredits(usdMicros: number, locale = 'en-US'): string {
  const safe = Number.isSafeInteger(usdMicros) ? usdMicros : 0;
  const sign = safe < 0 ? '-' : '';
  const absolute = Math.abs(safe);
  const whole = Math.floor(absolute / usdMicrosPerCredit);
  const fraction = String(absolute % usdMicrosPerCredit).padStart(3, '0').replace(/0+$/, '');
  const amount = `${sign}${new Intl.NumberFormat(locale).format(whole)}${fraction ? `.${fraction}` : ''}`;
  return `${amount} ${absolute === usdMicrosPerCredit ? 'credit' : 'credits'}`;
}

export function formatCreditsInput(usdMicros: number): string {
  const safe = Number.isSafeInteger(usdMicros) ? usdMicros : 0;
  const sign = safe < 0 ? '-' : '';
  const absolute = Math.abs(safe);
  const whole = Math.floor(absolute / usdMicrosPerCredit);
  const fraction = String(absolute % usdMicrosPerCredit).padStart(3, '0').replace(/0+$/, '');
  return `${sign}${whole}${fraction ? `.${fraction}` : ''}`;
}

export function tryParseCreditsToUsdMicros(value: string): number | null {
  const normalized = value.trim().replace(/^-/, '');
  const sign = value.trim().startsWith('-') ? -1 : 1;
  const match = normalized.match(/^(\d{1,3}(?:,\d{3})+|\d+)(?:\.(\d{1,3}))?$/);
  if (!match) return null;
  const [, whole, fraction = ''] = match;
  const micros = Number(whole.replace(/,/g, '')) * usdMicrosPerCredit +
    Number(fraction.padEnd(3, '0'));
  return Number.isSafeInteger(micros) ? micros * sign : null;
}

export function tryParseUsdMicros(value: string): number | null {
  const normalized = value.trim().replace(/^\-\$/, '-').replace(/^\$/, '');
  const match = normalized.match(
    /^(-?)(\d{1,3}(?:,\d{3})+|\d+)(?:\.(\d{1,6}))?$/,
  );
  if (!match) return null;
  const [, sign, whole, fraction = ''] = match;
  const micros = Number(
    `${sign}${whole.replace(/,/g, '')}${fraction.padEnd(6, '0')}`,
  );
  return Number.isSafeInteger(micros) ? micros : null;
}

export function parseUsdMicros(value: string): number {
  return tryParseUsdMicros(value) ?? 0;
}
export function creditErrorMessage(error: unknown, fallback: string): string { return error instanceof Error && error.message ? error.message : fallback; }
export function newCreditIdempotencyKey(): string {
  if (!globalThis.crypto?.randomUUID) throw new Error('Secure request identifiers are not available in this browser.');
  return globalThis.crypto.randomUUID();
}