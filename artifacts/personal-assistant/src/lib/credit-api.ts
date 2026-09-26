const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');

type CreditRequestOptions = RequestInit & { body?: BodyInit | null };

export type CreditUsage = {
  balance: number;
  granted: number;
  adjustments: number;
  reserved: number;
  spent: number;
  refunded: number;
};

export type CreditReceipt = {
  id: string | number;
  operationType?: string;
  provider?: string;
  mode?: string;
  status?: string;
  reservedCredits?: number;
  settledCredits?: number;
  refundedCredits?: number;
  expiresAt?: string | null;
  createdAt?: string | null;
  reservationId?: string;
  eventType?: string;
  credits?: number;
  durationMs?: number | null;
  policyVersion?: number;
  reason?: string;
  actorUserId?: string;
  reversalOfId?: number | null;
  reversalOfGrantId?: number | null;
};

export type CreditUsageResponse = {
  usage: CreditUsage;
  recent: {
    reservations: CreditReceipt[];
    events: CreditReceipt[];
  };
  ledgerEntries?: {
    adjustments: CreditReceipt[];
    grants: CreditReceipt[];
  };
};

export type CreditAccountResponse = CreditUsage & {
  policyVersion: number;
  canManage: boolean;
  enforcement: 'strict' | string;
};

export type CreditEstimate = {
  pricingKey: string;
  units: number;
  estimatedCredits: number;
  unit: string;
  creditsPerUnit: number;
  hardCapCredits: number;
  availableCredits: number;
  policyVersion: number;
  canReserve: boolean;
  overrunMarginPercent: number;
};

export type CreditPolicy = {
  version: number;
  operationWeights: Record<string, number>;
  monthlyGrantCredits: number;
  rolloverCapCredits: number;
  rolloverExpiryDays: number;
  overrunMarginPercent: number;
  changeReason?: string;
  createdBy?: string;
  createdAt?: string;
};

export type AdminAdjustmentInput = {
  userId: string;
  amountCredits: number;
  reason: string;
  idempotencyKey: string;
};

export type CreditReceiptDetails = {
  reservationId: string;
  reservedCredits: number;
  settledCredits: number;
  refundedCredits: number;
  balance: number;
  policyVersion: number;
};

async function request<T>(path: string, options: CreditRequestOptions = {}): Promise<T> {
  const response = await fetch(`${basePath}${path}`, {
    credentials: 'include',
    ...options,
    headers: {
      Accept: 'application/json',
      ...(options.body ? { 'Content-Type': 'application/json' } : {}),
      ...options.headers,
    },
  });

  const payload = (await response.json().catch(() => null)) as
    | { error?: string; code?: string; requestId?: string }
    | T
    | null;
  if (!response.ok) {
    const message =
      payload && typeof payload === 'object' && 'error' in payload && typeof payload.error === 'string'
        ? payload.error
        : `Request failed (${response.status})`;
    const error = new Error(message);
    Object.assign(error, {
      status: response.status,
      code: payload && typeof payload === 'object' && 'code' in payload ? payload.code : undefined,
      requestId: payload && typeof payload === 'object' && 'requestId' in payload ? payload.requestId : undefined,
    });
    throw error;
  }
  return payload as T;
}

export const creditApi = {
  account: () => request<CreditAccountResponse>('/api/ai/credits'),
  usage: () => request<CreditUsageResponse>('/api/ai/credits/usage'),
  estimate: (pricingKey: string, units: number) =>
    request<CreditEstimate>(`/api/ai/credits/estimate?pricingKey=${encodeURIComponent(pricingKey)}&units=${units}`),
  adminPolicy: () => request<CreditPolicy>('/api/admin/ai-credit-policy'),
  updateAdminPolicy: (
    policy: Omit<CreditPolicy, 'version' | 'createdBy' | 'createdAt'>,
    expectedVersion: number,
    changeReason: string,
  ) =>
    request<CreditPolicy>('/api/admin/ai-credit-policy', {
      method: 'PATCH',
      body: JSON.stringify({ ...policy, expectedVersion, changeReason }),
    }),
  adminAdjustment: (input: AdminAdjustmentInput) =>
    request<CreditUsage>('/api/admin/ai-credit-adjustments', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  adminReverseAdjustment: (id: number, input: Pick<AdminAdjustmentInput, 'userId' | 'reason' | 'idempotencyKey'>) =>
    request<CreditUsage>(`/api/admin/ai-credit-adjustments/${id}/reverse`, {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  adminReverseGrant: (id: number, input: Pick<AdminAdjustmentInput, 'userId' | 'reason' | 'idempotencyKey'>) =>
    request<CreditUsage>(`/api/admin/ai-credit-grants/${id}/reverse`, {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  adminRefundReservation: (
    id: string,
    input: Pick<AdminAdjustmentInput, 'userId' | 'amountCredits' | 'reason' | 'idempotencyKey'>,
  ) =>
    request<{ status: string; reason: string }>(`/api/admin/ai-credit-reservations/${encodeURIComponent(id)}/refund`, {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  adminUsage: (userId: string) =>
    request<CreditUsageResponse>(`/api/admin/ai-credit-usage/${encodeURIComponent(userId)}`),
};

export function creditErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

export function newCreditIdempotencyKey(): string {
  if (!globalThis.crypto?.randomUUID) {
    throw new Error('Secure request identifiers are not available in this browser.');
  }
  return globalThis.crypto.randomUUID();
}