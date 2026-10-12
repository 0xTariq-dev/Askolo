import assert from 'node:assert/strict';
import test from 'node:test';
import { getMonthGridAgendaRange } from './agenda-date-range';

test('month agenda range covers all 42 visible grid dates for every month and week start', () => {
  for (let month = 0; month < 12; month += 1) {
    for (const weekStartsOn of [0, 1, 2, 3, 4, 5, 6] as const) {
      const range = getMonthGridAgendaRange(new Date(2026, month, 15), weekStartsOn);
      const from = new Date(`${range.from}T00:00:00Z`);
      const to = new Date(`${range.to}T00:00:00Z`);
      const inclusiveDays = (to.getTime() - from.getTime()) / 86_400_000 + 1;

      assert.equal(
        inclusiveDays,
        42,
        `month ${month + 1}, week starts ${weekStartsOn}: ${range.from}..${range.to}`,
      );
    }
  }
});
