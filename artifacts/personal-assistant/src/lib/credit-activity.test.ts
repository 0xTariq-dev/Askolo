import assert from 'node:assert/strict';
import test from 'node:test';
import {
  getConductedCreditActivities,
  type CreditActivity,
} from './credit-activity.ts';
import type { CreditEvent, CreditReservation } from './credit-api.ts';

function reservation(id: string, status: string, createdAt: string): CreditReservation {
  return {
    id,
    operationType: 'voice.transcribe',
    provider: 'assemblyai',
    model: 'universal-3-5-pro',
    mode: 'recorded',
    status,
    reservedUsdMicros: 100,
    settledUsdMicros: status === 'settled' ? 75 : 0,
    refundedUsdMicros: 0,
    usageUnit: 'milliseconds',
    usageUnits: 10_000,
    policyVersion: 1,
    expiresAt: null,
    createdAt,
  };
}

function event(id: string, eventType: string, createdAt: string): CreditEvent {
  return {
    id,
    reservationId: `reservation-${id}`,
    eventType,
    amountUsdMicros: 100,
    usageUnit: 'milliseconds',
    usageUnits: 10_000,
    details: {},
    createdAt,
  };
}

test('credit activity includes completed operations and refunds, not reservation lifecycle noise', () => {
  const completed = reservation('completed', 'settled', '2026-09-30T09:00:00Z');
  const refund = event('refund', 'refunded', '2026-09-30T09:02:00Z');
  const activities = getConductedCreditActivities({
    reservations: [
      completed,
      reservation('held', 'reserved', '2026-09-30T09:04:00Z'),
      reservation('released', 'released', '2026-09-30T09:03:00Z'),
      reservation('expired', 'expired', '2026-09-30T09:01:00Z'),
    ],
    events: [
      event('reserve-event', 'reserved', '2026-09-30T09:04:00Z'),
      event('release-event', 'released', '2026-09-30T09:03:00Z'),
      event('settle-event', 'settled', '2026-09-30T09:00:01Z'),
      event('expiry-event', 'expired', '2026-09-30T09:01:00Z'),
      refund,
    ],
  });

  assert.deepEqual(activities, [refund, completed] satisfies CreditActivity[]);
  assert.deepEqual(
    activities.map((activity) => activity.id),
    ['refund', 'completed'],
  );
});