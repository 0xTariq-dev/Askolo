import { format, isSameMonth, eachDayOfInterval, startOfWeek, endOfWeek, startOfMonth, endOfMonth, addDays } from 'date-fns';
import { cn } from '@/lib/utils';
import { EventChip } from './event-chip';
import type { Event } from '@workspace/api-client-react';
import type { DailyPlan, Habit } from './types';
import { useLocale } from '@/contexts/locale-context';

const REFERENCE_MONDAY = new Date(2024, 0, 1);
const MAX_CHIPS = 3;
const MAX_HABIT_DOTS = 6;

interface MonthGridProps {
  currentDate: Date;
  events: Event[];
  plans: DailyPlan[];
  habits: Habit[];
  completionsByDate: Map<string, Set<number>>;
  todayStr: string;
  onEventClick: (event: Event) => void;
  onDayClick: (date: Date) => void;
}

export function MonthGrid({
  currentDate,
  events,
  plans,
  habits,
  completionsByDate,
  todayStr,
  onEventClick,
  onDayClick,
}: MonthGridProps) {
  const { formatDate, formatNumber, plural, weekStartsOn } = useLocale();
  const monthStart = startOfMonth(currentDate);
  const monthEnd = endOfMonth(currentDate);
  const gridStart = startOfWeek(monthStart, { weekStartsOn });
  const gridEnd = endOfWeek(monthEnd, { weekStartsOn });
  const cells = eachDayOfInterval({ start: gridStart, end: gridEnd });
  const hasHabits = habits.length > 0;
  const weekDays = Array.from({ length: 7 }, (_, index) =>
    formatDate(addDays(REFERENCE_MONDAY, index), { weekday: 'short' }),
  );

  return (
    <div className="flex flex-col flex-1 min-h-0">
      {/* Day-of-week header */}
      <div className="grid grid-cols-7 border-b border-border/50">
        {weekDays.map((d) => (
          <div key={d} className="py-2 text-center text-xs font-medium text-muted-foreground">
            {d}
          </div>
        ))}
      </div>

      {/* Calendar grid */}
      <div className="grid grid-cols-7 flex-1" style={{ gridAutoRows: 'minmax(100px, 1fr)' }}>
        {cells.map((day) => {
          const dateStr = format(day, 'yyyy-MM-dd');
          const isCurrentMonth = isSameMonth(day, currentDate);
          const isTodayCell = dateStr === todayStr;

          // Events spanning this day
          const dayEvents = events.filter((e) => {
            const end = e.endDate || e.startDate;
            return e.startDate <= dateStr && end >= dateStr;
          });
          const allDayEvents = dayEvents.filter((e) => e.allDay);
          const timedEvents = dayEvents
            .filter((e) => !e.allDay)
            .sort((a, b) => (a.startTime || '00:00').localeCompare(b.startTime || '00:00'));
          const orderedEvents = [...allDayEvents, ...timedEvents];
          const visible = orderedEvents.slice(0, MAX_CHIPS);
          const overflow = orderedEvents.length - MAX_CHIPS;

          const dayPlans = plans.filter((p) => p.date === dateStr);

          // Habit completion dots — all days
          const completed = completionsByDate.get(dateStr) ?? new Set<number>();

          return (
            <div
              key={dateStr}
              className={cn(
                'border-b border-e border-border/40 p-1 flex flex-col gap-0.5 overflow-hidden',
                !isCurrentMonth && 'bg-muted/20',
              )}
            >
              {/* Day number */}
              <button
                onClick={() => onDayClick(day)}
                aria-label={formatDate(day, { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' })}
                data-testid={`calendar-day-${dateStr}`}
                className={cn(
                  'self-start h-6 w-6 rounded-full text-xs font-medium flex items-center justify-center shrink-0 transition-colors hover:bg-muted',
                  isTodayCell && 'bg-primary text-primary-foreground hover:bg-primary/90',
                  !isCurrentMonth && !isTodayCell && 'text-muted-foreground/50',
                )}
              >
                  {formatNumber(day.getDate(), { useGrouping: false })}
              </button>

              {/* Event + plan chips */}
              <div className="flex flex-col gap-0.5 min-h-0 overflow-hidden">
                {visible.map((event) => (
                  <EventChip key={event.id} event={event} onClick={() => onEventClick(event)} />
                ))}
                {dayPlans.slice(0, overflow < 0 ? MAX_CHIPS - visible.length : 0).map((plan) => (
                  <PlanChip key={plan.id} plan={plan} />
                ))}
                {overflow > 0 && (
                  <button
                    onClick={() => onDayClick(day)}
                    className="text-[10px] text-muted-foreground hover:text-foreground text-start px-1"
                  >
                    {plural('calendar.moreEvents', overflow)}
                  </button>
                )}
              </div>

              {/* Habit completion dots */}
              {hasHabits && (
                <div className="flex items-center gap-0.5 mt-auto pt-0.5 flex-wrap">
                  {habits.slice(0, MAX_HABIT_DOTS).map((h) => {
                    const done = completed.has(h.id);
                    return (
                      <div
                        key={h.id}
                        className={cn(
                          'h-1.5 w-1.5 rounded-full shrink-0 transition-opacity',
                          done ? 'opacity-100' : 'opacity-20',
                        )}
                        style={{ backgroundColor: h.color || 'hsl(var(--chart-1))' }}
                        title={`${h.name}${done ? ' ✓' : ''}`}
                      />
                    );
                  })}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

function PlanChip({ plan }: { plan: DailyPlan }) {
  return (
    <div
      className={cn(
        'w-full text-start text-xs px-1.5 py-0.5 rounded truncate border-s-2',
        plan.priority === 'high'
          ? 'bg-destructive/10 text-destructive border-destructive'
          : plan.priority === 'medium'
            ? 'bg-warning/10 text-warning border-warning'
            : 'bg-muted text-muted-foreground border-border',
      )}
      title={plan.title}
      dir="auto"
    >
      {plan.completed ? <s className="opacity-60">{plan.title}</s> : plan.title}
    </div>
  );
}
