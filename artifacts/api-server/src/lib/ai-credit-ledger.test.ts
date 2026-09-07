import assert from "node:assert/strict";
import { randomUUID } from "node:crypto";
import test from "node:test";
import { eq } from "drizzle-orm";
import { db, usersTable } from "@workspace/db";

import {
  CreditLedgerError,
  CreditLimitError,
  adjustCredits,
  claimProviderToken,
  closeConnectedSession,
  deriveScopedIdempotencyKey,
  estimateCredits,
  estimateToolCredits,
  expireStaleReservations,
  getCreditBalance,
  grantCredits,
  hashProviderToken,
  heartbeatConnectedSession,
  refundCredits,
  reserveCredits,
  settleReservation,
  withPricedCreditReservation,
} from "./ai-credit-ledger";

test("AI credit ledger is atomic, idempotent, replay-safe, and server-priced", async () => {
  const userId = `ledger-test-${randomUUID()}`;
  const newUserId = `ledger-new-user-${randomUUID()}`;
  await db.insert(usersTable).values({ id: userId });
  await db.insert(usersTable).values({ id: newUserId });

  try {
    assert.equal(estimateCredits("model.assistant", 1).estimatedCredits, 1);
    assert.equal(estimateToolCredits("tool.write", 3).estimatedCredits, 6);
    assert.throws(() => estimateCredits("transcription.recorded", 121), CreditLimitError);
    assert.notEqual(
      deriveScopedIdempotencyKey("shared-batch-key", "email-priority", "message-a"),
      deriveScopedIdempotencyKey("shared-batch-key", "email-priority", "message-b"),
    );

    let firstRequestExecutions = 0;
    await withPricedCreditReservation(
      {
        userId: newUserId,
        pricingKey: "model.assistant",
        units: 1,
        idempotencyKey: `first-request-${randomUUID()}`,
        requestFingerprint: "c".repeat(64),
      },
      async () => {
        firstRequestExecutions += 1;
        return "welcome";
      },
    );
    assert.equal(firstRequestExecutions, 1);
    assert.deepEqual(await getCreditBalance(newUserId), {
      grantedCredits: 1,
      adjustmentCredits: 0,
      reservedCredits: 0,
      spentCredits: 1,
      refundedCredits: 0,
      availableCredits: 0,
    });
    await grantCredits({
      userId: newUserId,
      sourceType: "subscription",
      amountCredits: 1,
      entitlementKey: "test-subscription",
      idempotencyKey: `subscription-${randomUUID()}`,
    });
    await assert.rejects(
      () => withPricedCreditReservation(
        {
          userId: newUserId,
          pricingKey: "tool.external",
          units: 1,
          idempotencyKey: `strict-after-entitlement-${randomUUID()}`,
          requestFingerprint: "d".repeat(64),
        },
        async () => "must not run",
      ),
      CreditLimitError,
    );

    const grantKey = `test-grant-${randomUUID()}`;
    const grants = await Promise.all([
      grantCredits({
        userId,
        sourceType: "test",
        amountCredits: 10,
        idempotencyKey: grantKey,
      }),
      grantCredits({
        userId,
        sourceType: "test",
        amountCredits: 10,
        idempotencyKey: grantKey,
      }),
    ]);
    assert.equal(grants.filter((result) => result.reused).length, 1);
    assert.equal((await getCreditBalance(userId)).grantedCredits, 10);

    const concurrent = await Promise.allSettled([
      reserveCredits({
        userId,
        operationType: "tool_run",
        provider: "askolo",
        mode: "tool-write",
        estimatedCredits: 7,
        maxCredits: 7,
        idempotencyKey: `concurrent-a-${randomUUID()}`,
        enforceBalance: true,
      }),
      reserveCredits({
        userId,
        operationType: "tool_run",
        provider: "askolo",
        mode: "tool-write",
        estimatedCredits: 7,
        maxCredits: 7,
        idempotencyKey: `concurrent-b-${randomUUID()}`,
        enforceBalance: true,
      }),
    ]);
    const successfulReservation = concurrent.find(
      (result): result is PromiseFulfilledResult<Awaited<ReturnType<typeof reserveCredits>>> =>
        result.status === "fulfilled",
    );
    assert.ok(successfulReservation);
    assert.equal(concurrent.filter((result) => result.status === "rejected").length, 1);
    const rejected = concurrent.find((result) => result.status === "rejected") as PromiseRejectedResult;
    assert.ok(rejected.reason instanceof CreditLimitError);

    const settlementKey = `settle-${randomUUID()}`;
    const concurrentSettlements = await Promise.all([
      settleReservation({
        reservationId: successfulReservation.value.reservation.id,
        userId,
        chargedCredits: 5,
        idempotencyKey: settlementKey,
      }),
      settleReservation({
        reservationId: successfulReservation.value.reservation.id,
        userId,
        chargedCredits: 5,
        idempotencyKey: settlementKey,
      }),
    ]);
    assert.equal(concurrentSettlements.filter((result) => result.reused).length, 1);
    assert.deepEqual(
      await getCreditBalance(userId),
      {
        grantedCredits: 10,
        adjustmentCredits: 0,
        reservedCredits: 0,
        spentCredits: 5,
        refundedCredits: 0,
        availableCredits: 5,
      },
    );

    const refundKey = `refund-${randomUUID()}`;
    const concurrentRefunds = await Promise.all([
      refundCredits({
        reservationId: successfulReservation.value.reservation.id,
        userId,
        amountCredits: 2,
        idempotencyKey: refundKey,
      }),
      refundCredits({
        reservationId: successfulReservation.value.reservation.id,
        userId,
        amountCredits: 2,
        idempotencyKey: refundKey,
      }),
    ]);
    assert.equal(concurrentRefunds.filter((result) => result.reused).length, 1);
    assert.equal((await getCreditBalance(userId)).availableCredits, 7);

    const pricedKey = `priced-tool-${randomUUID()}`;
    let toolExecutions = 0;
    await withPricedCreditReservation(
      { userId, pricingKey: "tool.read", units: 2, idempotencyKey: pricedKey },
      async () => {
        toolExecutions += 1;
        return "ok";
      },
    );
    await assert.rejects(
      () => withPricedCreditReservation(
        { userId, pricingKey: "tool.read", units: 2, idempotencyKey: pricedKey },
        async () => {
          toolExecutions += 1;
          return "duplicate";
        },
      ),
      CreditLedgerError,
    );
    assert.equal(toolExecutions, 1);

    const concurrentExecutionKey = `concurrent-execution-${randomUUID()}`;
    let concurrentExecutions = 0;
    const duplicateExecutions = await Promise.allSettled([
      withPricedCreditReservation(
        {
          userId,
          pricingKey: "tool.read",
          units: 1,
          idempotencyKey: concurrentExecutionKey,
          requestFingerprint: "b".repeat(64),
          expiresInSeconds: 30,
        },
        async () => {
          concurrentExecutions += 1;
          await new Promise((resolve) => setTimeout(resolve, 40));
          return "first";
        },
      ),
      withPricedCreditReservation(
        {
          userId,
          pricingKey: "tool.read",
          units: 1,
          idempotencyKey: concurrentExecutionKey,
          requestFingerprint: "b".repeat(64),
          expiresInSeconds: 30,
        },
        async () => {
          concurrentExecutions += 1;
          await new Promise((resolve) => setTimeout(resolve, 40));
          return "second";
        },
      ),
    ]);
    assert.equal(concurrentExecutions, 1);
    assert.equal(duplicateExecutions.filter((result) => result.status === "fulfilled").length, 1);
    assert.equal(duplicateExecutions.filter((result) => result.status === "rejected").length, 1);

    await grantCredits({
      userId,
      sourceType: "test",
      amountCredits: 20,
      idempotencyKey: `test-grant-${randomUUID()}`,
    });
    const adjustmentKey = `adjust-${randomUUID()}`;
    const adjustments = await Promise.all([
      adjustCredits({
        userId,
        amountCredits: -1,
        reason: "test correction",
        idempotencyKey: adjustmentKey,
      }),
      adjustCredits({
        userId,
        amountCredits: -1,
        reason: "test correction",
        idempotencyKey: adjustmentKey,
      }),
    ]);
    assert.equal(adjustments.filter((result) => result.reused).length, 1);
    assert.equal((await getCreditBalance(userId)).adjustmentCredits, -1);

    const providerToken = randomUUID();
    const session = await reserveCredits({
      userId,
      operationType: "managed_voice_session",
      provider: "assemblyai",
      mode: "managed-agent",
      estimatedCredits: 5,
      maxCredits: 10,
      unitRate: 1,
      durationSeconds: 5,
      expiresInSeconds: 30,
      idempotencyKey: `session-${randomUUID()}`,
      providerTokenHash: hashProviderToken(providerToken),
      enforceBalance: true,
    });
    assert.equal(await claimProviderToken({
      reservationId: session.reservation.id,
      userId,
      token: providerToken,
    }), true);
    assert.equal(await claimProviderToken({
      reservationId: session.reservation.id,
      userId,
      token: providerToken,
    }), false);

    await heartbeatConnectedSession({
      reservationId: session.reservation.id,
      userId,
      connectedDurationMs: 1_000,
      inactivityTimeoutSeconds: 10,
      idempotencyKey: `heartbeat-${randomUUID()}`,
    });
    const closed = await closeConnectedSession({
      reservationId: session.reservation.id,
      userId,
      reason: "network_loss",
      connectedDurationMs: 1_000,
      idempotencyKey: `close-${randomUUID()}`,
    });
    assert.equal(closed.reservation.status, "disconnected");
    assert.ok(closed.reservation.settledCredits >= 1);

    const ambiguousFailureKey = `ambiguous-${randomUUID()}`;
    const spentBeforeFailure = (await getCreditBalance(userId)).spentCredits;
    await assert.rejects(
      () => withPricedCreditReservation(
        {
          userId,
          pricingKey: "tool.external",
          units: 1,
          idempotencyKey: ambiguousFailureKey,
          requestFingerprint: "a".repeat(64),
          expiresInSeconds: 30,
        },
        async () => {
          throw new Error("provider connection was lost");
        },
      ),
      /provider connection was lost/,
    );
    const afterFailure = await getCreditBalance(userId);
    assert.equal(afterFailure.reservedCredits, 0);
    assert.equal(afterFailure.spentCredits, spentBeforeFailure + 3);

    const expiring = await reserveCredits({
      userId,
      operationType: "realtime_transcript",
      provider: "assemblyai",
      mode: "realtime",
      estimatedCredits: 5,
      maxCredits: 5,
      unitRate: 1,
      durationSeconds: 5,
      expiresInSeconds: 1,
      idempotencyKey: `expiring-${randomUUID()}`,
      enforceBalance: true,
    });
    assert.equal(await expireStaleReservations(new Date(Date.now() + 2_000)), 1);
    const balanceAfterExpiry = await getCreditBalance(userId);
    assert.equal(balanceAfterExpiry.reservedCredits, 0);
    assert.ok(balanceAfterExpiry.spentCredits >= expiring.reservation.settledCredits);
  } finally {
    await db.delete(usersTable).where(eq(usersTable.id, userId));
    await db.delete(usersTable).where(eq(usersTable.id, newUserId));
  }
});