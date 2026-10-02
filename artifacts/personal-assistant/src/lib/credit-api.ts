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
  policyVersion: number; expiresAt: string | null; createdAt: string;
};
export type CreditEvent = {
  id: string; reservationId: string; eventType: string; amountUsdMicros: UsdMicros;
  usageUnit: string; usageUnits: number; details: unknown; createdAt: string;
};
export type CreditAdjustment = { id: number; userId: string; amountUsdMicros: UsdMicros; reason: string; actorUserId: string; reversalOfId: number | null; createdAt: string };
export type CreditGrant = { id: number; userId: string; amountUsdMicros: UsdMicros; reason: string; actorUserId: string; createdAt: string };
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
  provider: string; mode: string; model: string; meter: string;
  usdMicrosPerHour: UsdMicros | null; inputUsdMicrosPerMillion: UsdMicros | null;
  outputUsdMicrosPerMillion: UsdMicros | null;
};
export type CreditPolicy = {
  version: number; rateCards: Record<string, RateCard>;
  monthlyGrantUsdMicros: UsdMicros; rolloverCapUsdMicros: UsdMicros;
  rolloverExpiryDays: number; overrunMarginPercent: number;
  changeReason?: string; createdBy?: string; createdAt?: string;
};
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
  estimate: (pricingKey: 'voice.recorded' | 'voice.realtime' | 'assistant', units: number) => request<CreditEstimate>(`/api/ai/credits/estimate?pricingKey=${encodeURIComponent(pricingKey)}&units=${Math.max(0, Math.trunc(units))}`),
  adminPolicy: () => request<CreditPolicy>('/api/admin/ai-credit-policy'),
  updateAdminPolicy: (policy: Pick<CreditPolicy, 'rateCards' | 'monthlyGrantUsdMicros' | 'rolloverCapUsdMicros' | 'rolloverExpiryDays' | 'overrunMarginPercent'>, expectedVersion: number, changeReason: string) => request<CreditPolicy>('/api/admin/ai-credit-policy', { method: 'PATCH', body: JSON.stringify({ ...policy, expectedVersion, changeReason }) }),
  adminAdjustment: (input: AdminAdjustmentInput) => request<CreditUsage>('/api/admin/ai-credit-adjustments', { method: 'POST', body: JSON.stringify(input) }),
  adminReverseAdjustment: (id: number, input: Pick<AdminAdjustmentInput, 'userId' | 'amountUsdMicros' | 'reason' | 'idempotencyKey'>) => request<CreditUsage>(`/api/admin/ai-credit-adjustments/${id}/reverse`, { method: 'POST', body: JSON.stringify(input) }),
  adminReverseGrant: (id: number, input: Pick<AdminAdjustmentInput, 'userId' | 'amountUsdMicros' | 'reason' | 'idempotencyKey'>) => request<CreditUsage>(`/api/admin/ai-credit-grants/${id}/reverse`, { method: 'POST', body: JSON.stringify(input) }),
  adminRefundReservation: (id: string, input: Pick<AdminAdjustmentInput, 'userId' | 'amountUsdMicros' | 'reason' | 'idempotencyKey'>) => request<{ status: string; reason: string }>(`/api/admin/ai-credit-reservations/${encodeURIComponent(id)}/refund`, { method: 'POST', body: JSON.stringify(input) }),
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
export function parseUsdMicros(value: string): number {
  const normalized = value.trim().replace(/^\-\$/, '-').replace(/^\$/, '').replace(/,/g, '');
  if (!/^-?\d+(?:\.\d{1,6})?$/.test(normalized)) return 0;
  const [whole, fraction = ''] = normalized.split('.');
  const micros = Number(`${whole}${fraction.padEnd(6, '0')}`);
  return Number.isSafeInteger(micros) ? micros : 0;
}
export function creditErrorMessage(error: unknown, fallback: string): string { return error instanceof Error && error.message ? error.message : fallback; }
export function newCreditIdempotencyKey(): string {
  if (!globalThis.crypto?.randomUUID) throw new Error('Secure request identifiers are not available in this browser.');
  return globalThis.crypto.randomUUID();
}