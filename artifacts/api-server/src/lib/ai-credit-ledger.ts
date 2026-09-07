import { createHash, randomUUID } from "node:crypto";
import { and, eq, gt, inArray, isNull, lt, ne } from "drizzle-orm";
import {
  aiCreditAccountsTable,
  aiCreditAdjustmentsTable,
  aiCreditGrantsTable,
  aiCreditReservationEventsTable,
  aiCreditReservationsTable,
  aiProviderUsageEvidenceTable,
  db,
  type AiCreditAccount,
  type AiCreditReservation,
} from "@workspace/db";

export type AiCreditOperation =
  | "model_call"
  | "recorded_transcription"
  | "realtime_transcript"
  | "managed_voice_session"
  | "tool_run"
  | "automation";

export type AiCreditReservationStatus =
  | "reserved"
  | "active"
  | "settled"
  | "released"
  | "expired"
  | "disconnected";

export interface ReserveCreditsInput {
  userId: string;
  operationType: AiCreditOperation;
  provider: string;
  mode: string;
  model?: string;
  estimatedCredits: number;
  maxCredits?: number;
  unitRate?: number;
  durationSeconds?: number;
  expiresInSeconds?: number;
  idempotencyKey: string;
  requestFingerprint?: string;
  providerTokenHash?: string;
  metadata?: Record<string, unknown>;
  /**
   * The initial product has no allowance policy yet. Record-only callers
   * can meter usage while policy callers set this to true.
   */
  enforceBalance?: boolean;
}

export type CreditPricingUnit = "operation" | "second" | "execution";

export interface CreditPrice {
  key: string;
  operationType: AiCreditOperation;
  unit: CreditPricingUnit;
  creditsPerUnit: number;
  minimumCredits: number;
  maximumCredits: number;
  provider: string;
  mode: string;
  model?: string;
}

const CREDIT_PRICES = {
  "model.generate-plan": {
    key: "model.generate-plan",
    operationType: "model_call",
    unit: "operation",
    creditsPerUnit: 1,
    minimumCredits: 1,
    maximumCredits: 1,
    provider: "openai",
    mode: "planning",
    model: "gpt-5.6-luna",
  },
  "model.coaching": {
    key: "model.coaching",
    operationType: "model_call",
    unit: "operation",
    creditsPerUnit: 1,
    minimumCredits: 1,
    maximumCredits: 1,
    provider: "openai",
    mode: "coaching",
    model: "gpt-5.6-luna",
  },
  "model.assistant": {
    key: "model.assistant",
    operationType: "model_call",
    unit: "operation",
    creditsPerUnit: 1,
    minimumCredits: 1,
    maximumCredits: 1,
    provider: "openai",
    mode: "chat",
    model: "gpt-5.6-luna",
  },
  "model.voice-to-plan": {
    key: "model.voice-to-plan",
    operationType: "model_call",
    unit: "operation",
    creditsPerUnit: 1,
    minimumCredits: 1,
    maximumCredits: 1,
    provider: "openai",
    mode: "planning",
    model: "gpt-5.6-luna",
  },
  "model.meeting-extract": {
    key: "model.meeting-extract",
    operationType: "model_call",
    unit: "operation",
    creditsPerUnit: 1,
    minimumCredits: 1,
    maximumCredits: 1,
    provider: "openai",
    mode: "meeting-extract",
    model: "gpt-5.6-luna",
  },
  "model.gmail-draft": {
    key: "model.gmail-draft",
    operationType: "model_call",
    unit: "operation",
    creditsPerUnit: 1,
    minimumCredits: 1,
    maximumCredits: 1,
    provider: "openai",
    mode: "gmail-draft",
    model: "gpt-5.6-luna",
  },
  "model.email-priority": {
    key: "model.email-priority",
    operationType: "model_call",
    unit: "operation",
    creditsPerUnit: 1,
    minimumCredits: 1,
    maximumCredits: 1,
    provider: "openai",
    mode: "email-priority",
    model: "gpt-5.6-luna",
  },
  "transcription.recorded": {
    key: "transcription.recorded",
    operationType: "recorded_transcription",
    unit: "second",
    creditsPerUnit: 1,
    minimumCredits: 1,
    maximumCredits: 120,
    provider: "assemblyai",
    mode: "recorded",
    model: "universal-3-5-pro",
  },
  "transcription.realtime": {
    key: "transcription.realtime",
    operationType: "realtime_transcript",
    unit: "second",
    creditsPerUnit: 1,
    minimumCredits: 1,
    maximumCredits: 3600,
    provider: "assemblyai",
    mode: "realtime",
  },
  "voice.managed-session": {
    key: "voice.managed-session",
    operationType: "managed_voice_session",
    unit: "second",
    creditsPerUnit: 2,
    minimumCredits: 2,
    maximumCredits: 7200,
    provider: "assemblyai",
    mode: "managed-agent",
  },
  "tool.read": {
    key: "tool.read",
    operationType: "tool_run",
    unit: "execution",
    creditsPerUnit: 1,
    minimumCredits: 1,
    maximumCredits: 100,
    provider: "askolo",
    mode: "tool-read",
  },
  "tool.write": {
    key: "tool.write",
    operationType: "tool_run",
    unit: "execution",
    creditsPerUnit: 2,
    minimumCredits: 2,
    maximumCredits: 200,
    provider: "askolo",
    mode: "tool-write",
  },
  "tool.external": {
    key: "tool.external",
    operationType: "tool_run",
    unit: "execution",
    creditsPerUnit: 3,
    minimumCredits: 3,
    maximumCredits: 300,
    provider: "external",
    mode: "tool-external",
  },
} as const satisfies Record<string, CreditPrice>;

export type CreditPricingKey = keyof typeof CREDIT_PRICES;

export interface CreditBalance {
  grantedCredits: number;
  adjustmentCredits: number;
  reservedCredits: number;
  spentCredits: number;
  refundedCredits: number;
  availableCredits: number;
}

export interface ReservationResult {
  reservation: AiCreditReservation;
  balance: CreditBalance;
  reused: boolean;
}

export class CreditLimitError extends Error {
  readonly code = "AI_CREDITS_EXHAUSTED";

  constructor(
    message = "There are not enough AI credits for this operation.",
    public readonly balance?: CreditBalance,
  ) {
    super(message);
    this.name = "CreditLimitError";
  }
}

export class CreditLedgerError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "CreditLedgerError";
  }
}

export function getCreditPrice(pricingKey: CreditPricingKey): CreditPrice {
  return CREDIT_PRICES[pricingKey];
}

export function estimateCredits(pricingKey: CreditPricingKey, units: number): {
  pricing: CreditPrice;
  units: number;
  estimatedCredits: number;
} {
  assertPositiveInteger(units, "units");
  const pricing = getCreditPrice(pricingKey);
  const raw = units * pricing.creditsPerUnit;
  const estimatedCredits = Math.max(pricing.minimumCredits, Math.min(pricing.maximumCredits, raw));
  if (raw > pricing.maximumCredits) {
    throw new CreditLimitError(`Requested usage exceeds the hard cap for ${pricingKey}`);
  }
  return { pricing, units, estimatedCredits };
}

export function estimateToolCredits(
  pricingKey: Extract<CreditPricingKey, `tool.${string}`>,
  executions = 1,
): ReturnType<typeof estimateCredits> {
  return estimateCredits(pricingKey, executions);
}

function assertPositiveInteger(value: number, field: string): void {
  if (!Number.isSafeInteger(value) || value <= 0) {
    throw new CreditLedgerError(`${field} must be a positive integer`);
  }
}

function assertNonNegativeInteger(value: number, field: string): void {
  if (!Number.isSafeInteger(value) || value < 0) {
    throw new CreditLedgerError(`${field} must be a non-negative integer`);
  }
}

function availableCredits(account: Pick<AiCreditAccount, "grantedCredits" | "adjustmentCredits" | "reservedCredits" | "spentCredits" | "refundedCredits">): number {
  return (
    account.grantedCredits +
    account.adjustmentCredits -
    account.reservedCredits -
    account.spentCredits +
    account.refundedCredits
  );
}

function toBalance(account: AiCreditAccount): CreditBalance {
  return {
    grantedCredits: account.grantedCredits,
    adjustmentCredits: account.adjustmentCredits,
    reservedCredits: account.reservedCredits,
    spentCredits: account.spentCredits,
    refundedCredits: account.refundedCredits,
    availableCredits: availableCredits(account),
  };
}

async function lockAccount(tx: any, userId: string): Promise<AiCreditAccount> {
  await tx
    .insert(aiCreditAccountsTable)
    .values({ userId })
    .onConflictDoNothing();

  const [account] = await tx
    .select()
    .from(aiCreditAccountsTable)
    .where(eq(aiCreditAccountsTable.userId, userId))
    .for("update");

  if (!account) {
    throw new CreditLedgerError("Unable to initialize the AI credit account");
  }
  return account;
}

async function findReservation(tx: any, reservationId: string): Promise<AiCreditReservation | undefined> {
  const [reservation] = await tx
    .select()
    .from(aiCreditReservationsTable)
    .where(eq(aiCreditReservationsTable.id, reservationId))
    .for("update");
  return reservation;
}

function assertReservationOwner(reservation: AiCreditReservation | undefined, userId: string): AiCreditReservation {
  if (!reservation || reservation.userId !== userId) {
    throw new CreditLedgerError("AI credit reservation was not found");
  }
  return reservation;
}

function assertReservationMatches(reservation: AiCreditReservation, input: ReserveCreditsInput): void {
  if (
    reservation.operationType !== input.operationType ||
    reservation.provider !== input.provider ||
    reservation.mode !== input.mode ||
    reservation.model !== (input.model ?? null) ||
    reservation.requestFingerprint !== (input.requestFingerprint ?? null)
  ) {
    throw new CreditLedgerError("Idempotency key was reused for a different AI operation");
  }
}

function reservationEventKey(idempotencyKey: string): string {
  return idempotencyKey.slice(0, 240);
}

const SAFE_TELEMETRY_KEYS = new Set([
  "operation",
  "route",
  "audioFormat",
  "pricingKey",
  "pricingUnit",
  "estimatedUnits",
  "billedUnits",
  "terminationReason",
  "toolName",
  "toolClass",
  "outcome",
]);

function safeTelemetry(input?: Record<string, unknown>): Record<string, string | number | boolean | null> {
  const output: Record<string, string | number | boolean | null> = {};
  for (const [key, value] of Object.entries(input ?? {})) {
    if (!SAFE_TELEMETRY_KEYS.has(key)) continue;
    if (typeof value === "string") output[key] = value.slice(0, 160);
    else if (typeof value === "number" && Number.isFinite(value)) output[key] = value;
    else if (typeof value === "boolean" || value === null) output[key] = value;
  }
  return output;
}

export function hashProviderToken(token: string): string {
  return createHash("sha256").update(token).digest("hex");
}

export function deriveScopedIdempotencyKey(
  parentKey: string,
  operation: string,
  discriminator: string,
): string {
  const parentHash = createHash("sha256").update(parentKey).digest("hex").slice(0, 24);
  const discriminatorHash = createHash("sha256").update(discriminator).digest("hex").slice(0, 24);
  return `${operation}:${parentHash}:${discriminatorHash}`.slice(0, 200);
}

export async function getCreditBalance(userId: string): Promise<CreditBalance> {
  const [account] = await db
    .select()
    .from(aiCreditAccountsTable)
    .where(eq(aiCreditAccountsTable.userId, userId))
    .limit(1);
  if (!account) {
    return {
      grantedCredits: 0,
      adjustmentCredits: 0,
      reservedCredits: 0,
      spentCredits: 0,
      refundedCredits: 0,
      availableCredits: 0,
    };
  }
  return toBalance(account);
}

export async function grantCredits(input: {
  userId: string;
  sourceType: string;
  amountCredits: number;
  idempotencyKey: string;
  entitlementKey?: string;
  metadata?: Record<string, unknown>;
}): Promise<{ grant: typeof aiCreditGrantsTable.$inferSelect; balance: CreditBalance; reused: boolean }> {
  assertPositiveInteger(input.amountCredits, "amountCredits");
  if (!input.idempotencyKey) throw new CreditLedgerError("idempotencyKey is required");

  return db.transaction(async (tx: any) => {
    const [existing] = await tx
      .select()
      .from(aiCreditGrantsTable)
      .where(and(
        eq(aiCreditGrantsTable.userId, input.userId),
        eq(aiCreditGrantsTable.idempotencyKey, input.idempotencyKey),
      ))
      .limit(1);
    if (existing) {
      if (existing.userId !== input.userId) throw new CreditLedgerError("Grant idempotency key belongs to another user");
      const account = await lockAccount(tx, input.userId);
      return { grant: existing, balance: toBalance(account), reused: true };
    }

    const account = await lockAccount(tx, input.userId);
    const [racedGrant] = await tx
      .select()
      .from(aiCreditGrantsTable)
      .where(and(
        eq(aiCreditGrantsTable.userId, input.userId),
        eq(aiCreditGrantsTable.idempotencyKey, input.idempotencyKey),
      ))
      .limit(1);
    if (racedGrant) {
      if (racedGrant.userId !== input.userId) {
        throw new CreditLedgerError("Grant idempotency key belongs to another user");
      }
      return { grant: racedGrant, balance: toBalance(account), reused: true };
    }
    const [grant] = await tx
      .insert(aiCreditGrantsTable)
      .values({
        userId: input.userId,
        sourceType: input.sourceType,
        amountCredits: input.amountCredits,
        entitlementKey: input.entitlementKey,
        idempotencyKey: input.idempotencyKey,
        metadata: safeTelemetry(input.metadata),
      })
      .returning();
    await tx
      .update(aiCreditAccountsTable)
      .set({ grantedCredits: account.grantedCredits + input.amountCredits })
      .where(eq(aiCreditAccountsTable.userId, input.userId));
    const updated = await lockAccount(tx, input.userId);
    return { grant, balance: toBalance(updated), reused: false };
  });
}

export async function adjustCredits(input: {
  userId: string;
  amountCredits: number;
  reason: string;
  idempotencyKey: string;
  metadata?: Record<string, unknown>;
}): Promise<{ adjustment: typeof aiCreditAdjustmentsTable.$inferSelect; balance: CreditBalance; reused: boolean }> {
  if (!Number.isSafeInteger(input.amountCredits) || input.amountCredits === 0) {
    throw new CreditLedgerError("amountCredits must be a non-zero integer");
  }
  if (!input.reason.trim()) throw new CreditLedgerError("reason is required");
  if (!input.idempotencyKey) throw new CreditLedgerError("idempotencyKey is required");

  return db.transaction(async (tx: any) => {
    const account = await lockAccount(tx, input.userId);
    const [existing] = await tx
      .select()
      .from(aiCreditAdjustmentsTable)
      .where(and(
        eq(aiCreditAdjustmentsTable.userId, input.userId),
        eq(aiCreditAdjustmentsTable.idempotencyKey, input.idempotencyKey),
      ))
      .limit(1);
    if (existing) {
      if (existing.userId !== input.userId) throw new CreditLedgerError("Adjustment idempotency key belongs to another user");
      return { adjustment: existing, balance: toBalance(account), reused: true };
    }
    if (availableCredits(account) + input.amountCredits < 0) {
      throw new CreditLimitError("Adjustment would make the available AI credit balance negative", toBalance(account));
    }
    const [adjustment] = await tx
      .insert(aiCreditAdjustmentsTable)
      .values({
        userId: input.userId,
        amountCredits: input.amountCredits,
        reason: input.reason.trim(),
        idempotencyKey: input.idempotencyKey,
        metadata: safeTelemetry(input.metadata),
      })
      .returning();
    await tx
      .update(aiCreditAccountsTable)
      .set({ adjustmentCredits: account.adjustmentCredits + input.amountCredits })
      .where(eq(aiCreditAccountsTable.userId, input.userId));
    const updated = await lockAccount(tx, input.userId);
    return { adjustment, balance: toBalance(updated), reused: false };
  });
}

export async function rolloverCredits(input: {
  userId: string;
  amountCredits: number;
  entitlementKey: string;
  periodKey: string;
  metadata?: Record<string, unknown>;
}): ReturnType<typeof grantCredits> {
  return grantCredits({
    userId: input.userId,
    sourceType: "rollover",
    amountCredits: input.amountCredits,
    entitlementKey: input.entitlementKey,
    idempotencyKey: `rollover:${input.entitlementKey}:${input.periodKey}`.slice(0, 200),
    metadata: input.metadata,
  });
}

export async function reserveCredits(input: ReserveCreditsInput): Promise<ReservationResult> {
  assertPositiveInteger(input.estimatedCredits, "estimatedCredits");
  if (input.maxCredits !== undefined) {
    assertPositiveInteger(input.maxCredits, "maxCredits");
    if (input.maxCredits < input.estimatedCredits) {
      throw new CreditLedgerError("maxCredits cannot be less than estimatedCredits");
    }
  }
  if (input.unitRate !== undefined) assertPositiveInteger(input.unitRate, "unitRate");
  if (input.durationSeconds !== undefined) assertPositiveInteger(input.durationSeconds, "durationSeconds");
  if (input.expiresInSeconds !== undefined) assertPositiveInteger(input.expiresInSeconds, "expiresInSeconds");
  if (!input.idempotencyKey) throw new CreditLedgerError("idempotencyKey is required");

  return db.transaction(async (tx: any) => {
    const initialExisting = await tx
      .select()
      .from(aiCreditReservationsTable)
      .where(and(
        eq(aiCreditReservationsTable.userId, input.userId),
        eq(aiCreditReservationsTable.idempotencyKey, input.idempotencyKey),
      ))
      .limit(1);
    if (initialExisting[0]) {
      if (initialExisting[0].userId !== input.userId) {
        throw new CreditLedgerError("Reservation idempotency key belongs to another user");
      }
      assertReservationMatches(initialExisting[0], input);
      const account = await lockAccount(tx, input.userId);
      return { reservation: initialExisting[0], balance: toBalance(account), reused: true };
    }

    const account = await lockAccount(tx, input.userId);
    const existing = await tx
      .select()
      .from(aiCreditReservationsTable)
      .where(and(
        eq(aiCreditReservationsTable.userId, input.userId),
        eq(aiCreditReservationsTable.idempotencyKey, input.idempotencyKey),
      ))
      .limit(1);
    if (existing[0]) {
      if (existing[0].userId !== input.userId) {
        throw new CreditLedgerError("Reservation idempotency key belongs to another user");
      }
      assertReservationMatches(existing[0], input);
      return { reservation: existing[0], balance: toBalance(account), reused: true };
    }

    const balance = toBalance(account);
    if (input.enforceBalance && balance.availableCredits < input.estimatedCredits) {
      throw new CreditLimitError(undefined, balance);
    }

    const now = new Date();
    const expiresAt = input.expiresInSeconds
      ? new Date(now.getTime() + input.expiresInSeconds * 1000)
      : null;
    const reservationId = randomUUID();
    const isConnectedSession =
      input.operationType === "realtime_transcript" ||
      input.operationType === "managed_voice_session";
    const status = isConnectedSession ? "active" : "reserved";
    const [reservation] = await tx
      .insert(aiCreditReservationsTable)
      .values({
        id: reservationId,
        userId: input.userId,
        operationType: input.operationType,
        provider: input.provider,
        mode: input.mode,
        model: input.model,
        status,
        idempotencyKey: input.idempotencyKey,
        requestFingerprint: input.requestFingerprint,
        reservedCredits: input.estimatedCredits,
        maxCredits: input.maxCredits ?? input.estimatedCredits,
        unitRate: input.unitRate ?? 1,
        durationSeconds: input.durationSeconds,
        expiresAt,
        providerTokenHash: input.providerTokenHash,
        metadata: safeTelemetry(input.metadata),
      })
      .returning();

    await tx
      .update(aiCreditAccountsTable)
      .set({ reservedCredits: account.reservedCredits + input.estimatedCredits })
      .where(eq(aiCreditAccountsTable.userId, input.userId));
    await tx.insert(aiCreditReservationEventsTable).values({
      reservationId,
      userId: input.userId,
      eventType: "reserve",
      credits: input.estimatedCredits,
      idempotencyKey: reservationEventKey(`${input.idempotencyKey}:reserve`),
      details: { operationType: input.operationType, provider: input.provider, mode: input.mode },
    });
    const updated = await lockAccount(tx, input.userId);
    return { reservation, balance: toBalance(updated), reused: false };
  });
}

export async function extendReservation(input: {
  reservationId: string;
  userId: string;
  additionalCredits: number;
  additionalDurationSeconds?: number;
  idempotencyKey: string;
  enforceBalance?: boolean;
}): Promise<ReservationResult> {
  assertPositiveInteger(input.additionalCredits, "additionalCredits");
  if (input.additionalDurationSeconds !== undefined) {
    assertPositiveInteger(input.additionalDurationSeconds, "additionalDurationSeconds");
  }

  return db.transaction(async (tx: any) => {
    const reservation = assertReservationOwner(await findReservation(tx, input.reservationId), input.userId);
    const [event] = await tx
      .select()
      .from(aiCreditReservationEventsTable)
      .where(and(
        eq(aiCreditReservationEventsTable.reservationId, input.reservationId),
        eq(aiCreditReservationEventsTable.idempotencyKey, reservationEventKey(input.idempotencyKey)),
      ))
      .limit(1);
    if (event) {
      const account = await lockAccount(tx, input.userId);
      return { reservation, balance: toBalance(account), reused: true };
    }
    if (reservation.status !== "active" && reservation.status !== "reserved") {
      throw new CreditLedgerError(`Cannot extend a ${reservation.status} reservation`);
    }
    if (reservation.maxCredits !== null && reservation.reservedCredits + input.additionalCredits > reservation.maxCredits) {
      throw new CreditLedgerError("Reservation maximum would be exceeded");
    }

    const account = await lockAccount(tx, input.userId);
    const balance = toBalance(account);
    if (input.enforceBalance && balance.availableCredits < input.additionalCredits) {
      throw new CreditLimitError(undefined, balance);
    }
    const [updatedReservation] = await tx
      .update(aiCreditReservationsTable)
      .set({
        reservedCredits: reservation.reservedCredits + input.additionalCredits,
        durationSeconds: reservation.durationSeconds === null
          ? input.additionalDurationSeconds ?? null
          : reservation.durationSeconds + (input.additionalDurationSeconds ?? 0),
      })
      .where(eq(aiCreditReservationsTable.id, input.reservationId))
      .returning();
    await tx
      .update(aiCreditAccountsTable)
      .set({ reservedCredits: account.reservedCredits + input.additionalCredits })
      .where(eq(aiCreditAccountsTable.userId, input.userId));
    await tx.insert(aiCreditReservationEventsTable).values({
      reservationId: input.reservationId,
      userId: input.userId,
      eventType: "extend",
      credits: input.additionalCredits,
      idempotencyKey: reservationEventKey(input.idempotencyKey),
      durationMs: input.additionalDurationSeconds ? input.additionalDurationSeconds * 1000 : null,
    });
    const updatedAccount = await lockAccount(tx, input.userId);
    return { reservation: updatedReservation, balance: toBalance(updatedAccount), reused: false };
  });
}

export async function settleReservation(input: {
  reservationId: string;
  userId: string;
  chargedCredits: number;
  status?: Extract<AiCreditReservationStatus, "settled" | "released" | "expired" | "disconnected">;
  connectedDurationMs?: number;
  idempotencyKey: string;
  providerUsage?: {
    provider: string;
    mode: string;
    region?: string;
    model?: string;
    durationMs?: number;
    inputUnits?: number;
    outputUnits?: number;
    providerRequestId?: string;
    evidence?: Record<string, unknown>;
    idempotencyKey: string;
  };
}): Promise<ReservationResult> {
  assertNonNegativeInteger(input.chargedCredits, "chargedCredits");
  if (input.connectedDurationMs !== undefined) assertNonNegativeInteger(input.connectedDurationMs, "connectedDurationMs");

  return db.transaction(async (tx: any) => {
    const reservation = assertReservationOwner(await findReservation(tx, input.reservationId), input.userId);
    const [settlementEvent] = await tx
      .select()
      .from(aiCreditReservationEventsTable)
      .where(and(
        eq(aiCreditReservationEventsTable.reservationId, input.reservationId),
        eq(aiCreditReservationEventsTable.idempotencyKey, reservationEventKey(input.idempotencyKey)),
      ))
      .limit(1);
    if (settlementEvent) {
      const account = await lockAccount(tx, input.userId);
      return { reservation, balance: toBalance(account), reused: true };
    }
    if (!["active", "reserved"].includes(reservation.status)) {
      throw new CreditLedgerError(`Cannot settle a ${reservation.status} reservation`);
    }
    if (input.chargedCredits > reservation.reservedCredits) {
      throw new CreditLedgerError("Settlement cannot exceed the reserved amount; extend before charging more");
    }

    const account = await lockAccount(tx, input.userId);
    const releasedCredits = reservation.reservedCredits - input.chargedCredits;
    const status = input.status ?? (input.chargedCredits > 0 ? "settled" : "released");
    const [updatedReservation] = await tx
      .update(aiCreditReservationsTable)
      .set({
        status,
        reservedCredits: 0,
        settledCredits: input.chargedCredits,
        connectedDurationMs: input.connectedDurationMs ?? reservation.connectedDurationMs,
        closedAt: new Date(),
      })
      .where(eq(aiCreditReservationsTable.id, input.reservationId))
      .returning();
    await tx
      .update(aiCreditAccountsTable)
      .set({
        reservedCredits: account.reservedCredits - reservation.reservedCredits,
        spentCredits: account.spentCredits + input.chargedCredits,
      })
      .where(eq(aiCreditAccountsTable.userId, input.userId));
    await tx.insert(aiCreditReservationEventsTable).values({
      reservationId: input.reservationId,
      userId: input.userId,
      eventType: "settle",
      credits: input.chargedCredits,
      durationMs: input.connectedDurationMs,
      idempotencyKey: reservationEventKey(input.idempotencyKey),
      details: { status, releasedCredits },
    });
    if (releasedCredits > 0) {
      await tx.insert(aiCreditReservationEventsTable).values({
        reservationId: input.reservationId,
        userId: input.userId,
        eventType: "release",
        credits: releasedCredits,
        idempotencyKey: reservationEventKey(`${input.idempotencyKey}:release`),
      });
    }
    if (input.providerUsage) {
      await tx.insert(aiProviderUsageEvidenceTable).values({
        reservationId: input.reservationId,
        userId: input.userId,
        provider: input.providerUsage.provider,
        mode: input.providerUsage.mode,
        region: input.providerUsage.region,
        model: input.providerUsage.model,
        durationMs: input.providerUsage.durationMs,
        inputUnits: input.providerUsage.inputUnits,
        outputUnits: input.providerUsage.outputUnits,
        providerRequestId: input.providerUsage.providerRequestId,
        idempotencyKey: input.providerUsage.idempotencyKey,
        evidence: safeTelemetry(input.providerUsage.evidence),
      }).onConflictDoNothing();
    }
    const updatedAccount = await lockAccount(tx, input.userId);
    return { reservation: updatedReservation, balance: toBalance(updatedAccount), reused: false };
  });
}

export async function refundCredits(input: {
  reservationId: string;
  userId: string;
  amountCredits: number;
  idempotencyKey: string;
  reason?: string;
}): Promise<ReservationResult> {
  assertPositiveInteger(input.amountCredits, "amountCredits");
  return db.transaction(async (tx: any) => {
    const reservation = assertReservationOwner(await findReservation(tx, input.reservationId), input.userId);
    const [event] = await tx
      .select()
      .from(aiCreditReservationEventsTable)
      .where(and(
        eq(aiCreditReservationEventsTable.reservationId, input.reservationId),
        eq(aiCreditReservationEventsTable.idempotencyKey, reservationEventKey(input.idempotencyKey)),
      ))
      .limit(1);
    if (event) {
      const account = await lockAccount(tx, input.userId);
      return { reservation, balance: toBalance(account), reused: true };
    }
    if (reservation.settledCredits - reservation.refundedCredits < input.amountCredits) {
      throw new CreditLedgerError("Refund exceeds the unsettled charge");
    }
    const account = await lockAccount(tx, input.userId);
    const [updatedReservation] = await tx
      .update(aiCreditReservationsTable)
      .set({ refundedCredits: reservation.refundedCredits + input.amountCredits })
      .where(eq(aiCreditReservationsTable.id, input.reservationId))
      .returning();
    await tx
      .update(aiCreditAccountsTable)
      .set({
        refundedCredits: account.refundedCredits + input.amountCredits,
      })
      .where(eq(aiCreditAccountsTable.userId, input.userId));
    await tx.insert(aiCreditReservationEventsTable).values({
      reservationId: input.reservationId,
      userId: input.userId,
      eventType: "refund",
      credits: input.amountCredits,
      idempotencyKey: reservationEventKey(input.idempotencyKey),
      details: input.reason ? { reason: input.reason } : {},
    });
    const updatedAccount = await lockAccount(tx, input.userId);
    return { reservation: updatedReservation, balance: toBalance(updatedAccount), reused: false };
  });
}

export async function claimProviderToken(input: {
  reservationId: string;
  userId: string;
  token: string;
}): Promise<boolean> {
  const tokenHash = hashProviderToken(input.token);
  const claimed = await db
    .update(aiCreditReservationsTable)
    .set({ providerTokenUsedAt: new Date() })
    .where(and(
      eq(aiCreditReservationsTable.id, input.reservationId),
      eq(aiCreditReservationsTable.userId, input.userId),
      eq(aiCreditReservationsTable.providerTokenHash, tokenHash),
      eq(aiCreditReservationsTable.status, "active"),
      gt(aiCreditReservationsTable.expiresAt, new Date()),
      isNull(aiCreditReservationsTable.providerTokenUsedAt),
    ))
    .returning({ id: aiCreditReservationsTable.id });
  return claimed.length === 1;
}

export async function heartbeatConnectedSession(input: {
  reservationId: string;
  userId: string;
  connectedDurationMs: number;
  inactivityTimeoutSeconds: number;
  idempotencyKey: string;
}): Promise<AiCreditReservation> {
  assertNonNegativeInteger(input.connectedDurationMs, "connectedDurationMs");
  assertPositiveInteger(input.inactivityTimeoutSeconds, "inactivityTimeoutSeconds");
  return db.transaction(async (tx: any) => {
    const reservation = assertReservationOwner(await findReservation(tx, input.reservationId), input.userId);
    const eventKey = reservationEventKey(input.idempotencyKey);
    const [existing] = await tx
      .select()
      .from(aiCreditReservationEventsTable)
      .where(and(
        eq(aiCreditReservationEventsTable.reservationId, input.reservationId),
        eq(aiCreditReservationEventsTable.idempotencyKey, eventKey),
      ))
      .limit(1);
    if (existing) return reservation;
    if (reservation.status !== "active") {
      throw new CreditLedgerError(`Cannot heartbeat a ${reservation.status} reservation`);
    }
    if (!["realtime_transcript", "managed_voice_session"].includes(reservation.operationType)) {
      throw new CreditLedgerError("Heartbeats are only valid for connected sessions");
    }
    const previousDuration = reservation.connectedDurationMs ?? 0;
    if (input.connectedDurationMs < previousDuration) {
      throw new CreditLedgerError("Connected duration cannot move backwards");
    }
    const hardCapMs = (reservation.durationSeconds ?? 0) * 1000;
    if (hardCapMs > 0 && input.connectedDurationMs > hardCapMs) {
      throw new CreditLimitError("Connected session exceeded its reserved hard cap");
    }
    const [updated] = await tx
      .update(aiCreditReservationsTable)
      .set({
        connectedDurationMs: input.connectedDurationMs,
        expiresAt: new Date(Date.now() + input.inactivityTimeoutSeconds * 1000),
      })
      .where(eq(aiCreditReservationsTable.id, input.reservationId))
      .returning();
    await tx.insert(aiCreditReservationEventsTable).values({
      reservationId: input.reservationId,
      userId: input.userId,
      eventType: "heartbeat",
      credits: 0,
      durationMs: input.connectedDurationMs,
      idempotencyKey: eventKey,
    });
    return updated;
  });
}

export async function closeConnectedSession(input: {
  reservationId: string;
  userId: string;
  reason: "explicit_close" | "inactivity" | "token_expired" | "network_loss" | "browser_crash" | "provider_terminated";
  connectedDurationMs?: number;
  providerDurationMs?: number;
  providerRequestId?: string;
  region?: string;
  idempotencyKey: string;
}): Promise<ReservationResult> {
  const [reservation] = await db
    .select()
    .from(aiCreditReservationsTable)
    .where(and(
      eq(aiCreditReservationsTable.id, input.reservationId),
      eq(aiCreditReservationsTable.userId, input.userId),
    ))
    .limit(1);
  if (!reservation) throw new CreditLedgerError("AI credit reservation was not found");
  if (!["realtime_transcript", "managed_voice_session"].includes(reservation.operationType)) {
    throw new CreditLedgerError("Only connected sessions can use connected-session settlement");
  }
  const serverElapsedMs = Math.max(0, Date.now() - reservation.startedAt.getTime());
  const observedDurationMs = Math.max(
    reservation.connectedDurationMs ?? 0,
    input.connectedDurationMs ?? 0,
    input.providerDurationMs ?? 0,
    serverElapsedMs,
  );
  const cappedDurationMs = reservation.durationSeconds
    ? Math.min(observedDurationMs, reservation.durationSeconds * 1000)
    : observedDurationMs;
  const chargedCredits = Math.min(
    reservation.reservedCredits,
    Math.max(1, Math.ceil(cappedDurationMs / 1000) * reservation.unitRate),
  );
  return settleReservation({
    reservationId: input.reservationId,
    userId: input.userId,
    chargedCredits,
    status: input.reason === "explicit_close" || input.reason === "provider_terminated"
      ? "settled"
      : "disconnected",
    connectedDurationMs: cappedDurationMs,
    idempotencyKey: input.idempotencyKey,
    providerUsage: {
      provider: reservation.provider,
      mode: reservation.mode,
      model: reservation.model ?? undefined,
      region: input.region,
      durationMs: input.providerDurationMs ?? cappedDurationMs,
      providerRequestId: input.providerRequestId,
      idempotencyKey: `usage:${input.idempotencyKey}`.slice(0, 240),
      evidence: { terminationReason: input.reason },
    },
  });
}

async function claimReservationExecution(input: {
  reservationId: string;
  userId: string;
  idempotencyKey: string;
}): Promise<boolean> {
  return db.transaction(async (tx: any) => {
    const reservation = assertReservationOwner(await findReservation(tx, input.reservationId), input.userId);
    const eventKey = reservationEventKey(input.idempotencyKey);
    const [existing] = await tx
      .select()
      .from(aiCreditReservationEventsTable)
      .where(and(
        eq(aiCreditReservationEventsTable.reservationId, input.reservationId),
        eq(aiCreditReservationEventsTable.idempotencyKey, eventKey),
      ))
      .limit(1);
    if (existing) return false;
    if (reservation.status !== "reserved") return false;
    await tx
      .update(aiCreditReservationsTable)
      .set({ status: "active" })
      .where(eq(aiCreditReservationsTable.id, input.reservationId));
    await tx.insert(aiCreditReservationEventsTable).values({
      reservationId: input.reservationId,
      userId: input.userId,
      eventType: "claim",
      credits: 0,
      idempotencyKey: eventKey,
    });
    return true;
  });
}

export async function recordProviderUsage(input: {
  reservationId: string;
  userId: string;
  provider: string;
  mode: string;
  region?: string;
  model?: string;
  durationMs?: number;
  inputUnits?: number;
  outputUnits?: number;
  providerRequestId?: string;
  idempotencyKey: string;
  evidence?: Record<string, unknown>;
}): Promise<void> {
  await db.insert(aiProviderUsageEvidenceTable).values({
    reservationId: input.reservationId,
    userId: input.userId,
    provider: input.provider,
    mode: input.mode,
    region: input.region,
    model: input.model,
    durationMs: input.durationMs,
    inputUnits: input.inputUnits,
    outputUnits: input.outputUnits,
    providerRequestId: input.providerRequestId,
    idempotencyKey: input.idempotencyKey,
    evidence: safeTelemetry(input.evidence),
  }).onConflictDoNothing();
}

export async function expireStaleReservations(now = new Date(), limit = 100): Promise<number> {
  const stale = await db
    .select()
    .from(aiCreditReservationsTable)
    .where(and(
      inArray(aiCreditReservationsTable.status, ["active", "reserved"]),
      lt(aiCreditReservationsTable.expiresAt, now),
    ))
    .limit(limit);
  let settled = 0;
  for (const reservation of stale) {
    const elapsedMs = Math.max(0, now.getTime() - reservation.startedAt.getTime());
    const connectedDurationMs = reservation.durationSeconds
      ? Math.min(elapsedMs, reservation.durationSeconds * 1000)
      : undefined;
    const durationCredits = connectedDurationMs === undefined
      ? reservation.reservedCredits
      : Math.min(reservation.reservedCredits, Math.ceil(connectedDurationMs / 1000) * reservation.unitRate);
    await settleReservation({
      reservationId: reservation.id,
      userId: reservation.userId,
      chargedCredits: durationCredits,
      status: "expired",
      connectedDurationMs,
      idempotencyKey: `expiry:${reservation.id}:${reservation.expiresAt?.getTime() ?? 0}`,
    });
    settled += 1;
  }
  return settled;
}

export async function withCreditReservation<T>(
  input: ReserveCreditsInput,
  operation: (reservation: AiCreditReservation) => Promise<T>,
  settlement: {
    chargedCredits?: number;
    connectedDurationMs?: number;
    providerUsage?: Parameters<typeof settleReservation>[0]["providerUsage"];
  } = {},
): Promise<T> {
  const reserved = await reserveCredits(input);
  if (reserved.reservation.status !== "reserved") {
    throw new CreditLedgerError(`An idempotent AI operation is already ${reserved.reservation.status}`);
  }
  const claimed = await claimReservationExecution({
    reservationId: reserved.reservation.id,
    userId: input.userId,
    idempotencyKey: `${input.idempotencyKey}:claim`,
  });
  if (!claimed) {
    throw new CreditLedgerError("An idempotent AI operation is already in progress");
  }
  try {
    const result = await operation(reserved.reservation);
    await settleReservation({
      reservationId: reserved.reservation.id,
      userId: input.userId,
      chargedCredits: settlement.chargedCredits ?? input.estimatedCredits,
      connectedDurationMs: settlement.connectedDurationMs,
      providerUsage: settlement.providerUsage,
      idempotencyKey: `${input.idempotencyKey}:settle`,
    });
    return result;
  } catch (error) {
    await settleReservation({
      reservationId: reserved.reservation.id,
      userId: input.userId,
      // Once provider work starts, a timeout or disconnect is an ambiguous
      // outcome. Conservatively settle the reservation so unknown provider
      // completions cannot become free or remain locked forever.
      chargedCredits: reserved.reservation.reservedCredits,
      status: "disconnected",
      providerUsage: settlement.providerUsage
        ? {
            ...settlement.providerUsage,
            idempotencyKey: `failure:${settlement.providerUsage.idempotencyKey}`.slice(0, 240),
            evidence: {
              ...(settlement.providerUsage.evidence ?? {}),
              outcome: "ambiguous_failure",
            },
          }
        : undefined,
      idempotencyKey: `${input.idempotencyKey}:failure`,
    }).catch(() => undefined);
    throw error;
  }
}

export async function withPricedCreditReservation<T>(
  input: {
    userId: string;
    pricingKey: CreditPricingKey;
    units: number;
    idempotencyKey: string;
    requestFingerprint?: string;
    expiresInSeconds?: number;
    providerTokenHash?: string;
    metadata?: Record<string, unknown>;
  },
  operation: (reservation: AiCreditReservation) => Promise<T>,
  settlement: {
    actualUnits?: number;
    connectedDurationMs?: number;
    providerRequestId?: string;
    region?: string;
    evidence?: Record<string, unknown>;
  } = {},
): Promise<T> {
  const estimate = estimateCredits(input.pricingKey, input.units);
  const actual = estimateCredits(input.pricingKey, settlement.actualUnits ?? input.units);
  if (actual.estimatedCredits > estimate.estimatedCredits) {
    throw new CreditLedgerError("Actual usage exceeds the validated reservation; extend before settlement");
  }
  const [configuredEntitlement] = await db
    .select({ id: aiCreditGrantsTable.id })
    .from(aiCreditGrantsTable)
    .where(and(
      eq(aiCreditGrantsTable.userId, input.userId),
      ne(aiCreditGrantsTable.sourceType, "metering_bridge"),
    ))
    .limit(1);
  if (!configuredEntitlement) {
    // Until subscription policy is configured, fund exactly this server-priced
    // operation. This avoids disabling existing AI features without accepting
    // any client-controlled price or hiding usage from the ledger.
    await grantCredits({
      userId: input.userId,
      sourceType: "metering_bridge",
      amountCredits: estimate.estimatedCredits,
      entitlementKey: "pre-entitlement-transition",
      idempotencyKey: `bridge:${input.idempotencyKey}`.slice(0, 200),
      metadata: {
        operation: estimate.pricing.operationType,
        pricingKey: estimate.pricing.key,
        estimatedUnits: estimate.units,
      },
    });
  }
  return withCreditReservation(
    {
      userId: input.userId,
      operationType: estimate.pricing.operationType,
      provider: estimate.pricing.provider,
      mode: estimate.pricing.mode,
      model: estimate.pricing.model,
      estimatedCredits: estimate.estimatedCredits,
      maxCredits: estimate.pricing.maximumCredits,
      unitRate: estimate.pricing.creditsPerUnit,
      durationSeconds: estimate.pricing.unit === "second" ? input.units : undefined,
      expiresInSeconds: input.expiresInSeconds,
      idempotencyKey: input.idempotencyKey,
      requestFingerprint: input.requestFingerprint,
      providerTokenHash: input.providerTokenHash,
      metadata: {
        ...(input.metadata ?? {}),
        pricingKey: input.pricingKey,
        pricingUnit: estimate.pricing.unit,
        estimatedUnits: input.units,
      },
      enforceBalance: true,
    },
    operation,
    {
      chargedCredits: actual.estimatedCredits,
      connectedDurationMs: settlement.connectedDurationMs,
      providerUsage: {
        provider: estimate.pricing.provider,
        mode: estimate.pricing.mode,
        model: estimate.pricing.model,
        region: settlement.region,
        durationMs: settlement.connectedDurationMs,
        providerRequestId: settlement.providerRequestId,
        idempotencyKey: `usage:${input.idempotencyKey}`.slice(0, 240),
        evidence: {
          ...(settlement.evidence ?? {}),
          pricingKey: input.pricingKey,
          billedUnits: settlement.actualUnits ?? input.units,
        },
      },
    },
  );
}