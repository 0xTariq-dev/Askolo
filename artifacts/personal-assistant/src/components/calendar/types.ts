import type { Event } from '@workspace/api-client-react';

export type CalendarView = 'month' | 'week' | 'day';

export interface DailyPlan {
  id: number;
  date: string; // yyyy-MM-dd
  title: string;
  completed: boolean;
  priority: 'high' | 'medium' | 'low';
}

export interface Habit {
  id: number;
  name: string;
  color: string | null | undefined;
  completedToday: boolean; // treat undefined as false
}

/** An event with computed layout columns for overlap resolution */
export interface PositionedEvent {
  event: Event;
  startMinute: number; // minutes from midnight
  endMinute: number;
  column: number;
  totalColumns: number;
}

/** Resolve overlapping timed events into columns */
export function layoutEvents(events: Event[]): PositionedEvent[] {
  const parsed = events
    .filter((e) => !e.allDay && e.startTime)
    .map((e) => {
      const [sh, sm] = (e.startTime || '00:00').split(':').map(Number);
      const [eh, em] = (e.endTime || e.startTime || '01:00').split(':').map(Number);
      const startMinute = sh * 60 + sm;
      let endMinute = eh * 60 + em;
      if (endMinute <= startMinute) endMinute = startMinute + 30; // minimum 30 min
      return { event: e, startMinute, endMinute, column: 0, totalColumns: 1 };
    })
    .sort((a, b) => a.startMinute - b.startMinute);

  // Assign columns greedily
  const colEnds: number[] = [];
  for (const item of parsed) {
    let col = colEnds.findIndex((end) => end <= item.startMinute);
    if (col === -1) col = colEnds.length;
    colEnds[col] = item.endMinute;
    item.column = col;
  }

  // Compute totalColumns for each overlapping group
  for (const item of parsed) {
    const overlapping = parsed.filter(
      (other) => other.startMinute < item.endMinute && other.endMinute > item.startMinute,
    );
    item.totalColumns = Math.max(...overlapping.map((o) => o.column)) + 1;
  }

  return parsed;
}
