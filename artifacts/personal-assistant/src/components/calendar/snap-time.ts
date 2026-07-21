/**
 * Given a raw pixel offset `y` inside a 24-hour time grid of total height
 * `totalHeight`, return the time string ("HH:MM") snapped to the nearest
 * 30-minute slot boundary using floor (i.e. the slot that *contains* the
 * click, not the nearest boundary).
 *
 * Flooring guarantees that minutes is always 0 or 30 — never 60 — so the
 * hour value never wraps incorrectly near end-of-day.
 */
export function snapTimeFromY(y: number, totalHeight: number): string {
  const totalMinutes = Math.max(0, Math.min(Math.floor((y / totalHeight) * 24 * 60), 23 * 60 + 30));
  const snapped = Math.floor(totalMinutes / 30) * 30; // floor to 30-min slot
  const hours = Math.floor(snapped / 60);
  const minutes = snapped % 60;
  return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}`;
}
