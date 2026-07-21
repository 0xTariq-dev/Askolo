import { format, isSameMonth, eachDayOfInterval, startOfWeek, endOfWeek, startOfMonth, endOfMonth } from 'date-fns';
import { cn } from '@/lib/utils';
import { EventChip } from './event-chip';
import type { Event } from '@workspace/api-client-react';
import type { DailyPlan, Habit } from './types';

const WEEK_DAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
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
  const monthStart = startOfMonth(currentDate);
  const monthEnd = endOfMonth(currentDate);
  const gridStart = startOfWeek(monthStart, { weekStartsOn: 1 });
  const gridEnd = endOfWeek(monthEnd, { weekStartsOn: 1 });
  const cells = eachDayOfInterval({ start: gridStart, end: gridEnd });
  const hasHabits = habits.length > 0;

  return (
    <div className="flex flex-col flex-1 min-h-0">
      {/* Day-of-week header */}
      <div className="grid grid-cols-7 border-b border-border/50">
        {WEEK_DAYS.map((d) => (
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
                'border-b border-r border-border/40 p-1 flex flex-col gap-0.5 overflow-hidden',
                !isCurrentMonth && 'bg-muted/20',
              )}
            >
              {/* Day number */}
              <button
                onClick={() => onDayClick(day)}
                className={cn(
                  'self-start h-6 w-6 rounded-full text-xs font-medium flex items-center justify-center shrink-0 transition-colors hover:bg-muted',
                  isTodayCell && 'bg-primary text-primary-foreground hover:bg-primary/90',
                  !isCurrentMonth && !isTodayCell && 'text-muted-foreground/50',
                )}
              >
                {format(day, 'd')}
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
                    className="text-[10px] text-muted-foreground hover:text-foreground text-left px-1"
                  >
                    +{overflow} more
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
                        style={{ backgroundColor: h.color || '#3b82f6' }}
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
  const priorityColor =
    plan.priority === 'high' ? '#ef4444' : plan.priority === 'medium' ? '#f59e0b' : '#6b7280';
  return (
    <div
      className="w-full text-left text-xs px-1.5 py-0.5 rounded truncate"
      style={{
        backgroundColor: priorityColor + '20',
        color: priorityColor,
        borderLeft: `2px solid ${priorityColor}`,
      }}
      title={plan.title}
    >
      {plan.completed ? <s className="opacity-60">{plan.title}</s> : plan.title}
    </div>
  );
}
