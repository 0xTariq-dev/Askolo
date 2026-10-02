import { useRef, useEffect } from 'react';
import { format, eachDayOfInterval, startOfWeek, endOfWeek } from 'date-fns';
import { cn } from '@/lib/utils';
import { EventChip } from './event-chip';
import { layoutEvents } from './types';
import { snapTimeFromY } from './snap-time';
import type { Event } from '@workspace/api-client-react';
import type { DailyPlan, Habit } from './types';
import { useLocale } from '@/contexts/locale-context';

const PX_PER_HOUR = 64;
const TOTAL_HEIGHT = PX_PER_HOUR * 24;
const HOURS = Array.from({ length: 24 }, (_, i) => i);

interface WeekGridProps {
  currentDate: Date;
  events: Event[];
  plans: DailyPlan[];
  habits: Habit[];
  completionsByDate: Map<string, Set<number>>;
  todayStr: string;
  onEventClick: (event: Event) => void;
  onSlotClick?: (date: Date, time: string) => void;
}

export function WeekGrid({
  currentDate,
  events,
  plans,
  habits,
  completionsByDate,
  todayStr,
  onEventClick,
  onSlotClick,
}: WeekGridProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const { formatDate, formatNumber, weekStartsOn } = useLocale();
  const weekStart = startOfWeek(currentDate, { weekStartsOn });
  const weekEnd = endOfWeek(currentDate, { weekStartsOn });
  const days = eachDayOfInterval({ start: weekStart, end: weekEnd });

  useEffect(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = PX_PER_HOUR * 7;
  }, []);

  const now = new Date();
  const nowMinutes = now.getHours() * 60 + now.getMinutes();
  const nowTop = (nowMinutes / 60) * PX_PER_HOUR;
  const hasHabits = habits.length > 0;

  return (
    <div className="flex flex-col flex-1 min-h-0 overflow-hidden">
      {/* Day headers */}
      <div className="flex border-b border-border/50 shrink-0">
        <div className="w-14 shrink-0" />
        {days.map((day) => {
          const dateStr = format(day, 'yyyy-MM-dd');
          const isTodayCell = dateStr === todayStr;
          const dayAllDay = events.filter((e) => {
            const end = e.endDate || e.startDate;
            return e.allDay && e.startDate <= dateStr && end >= dateStr;
          });
          const dayPlans = plans.filter((p) => p.date === dateStr);
          const completed = completionsByDate.get(dateStr) ?? new Set<number>();
          const hasAllDayContent = dayAllDay.length > 0 || dayPlans.length > 0 || hasHabits;

          return (
            <div key={dateStr} className="flex-1 min-w-0 border-s border-border/30">
              {/* Day label */}
              <div className={cn('flex flex-col items-center py-2', isTodayCell && 'text-primary')}>
                <span className="text-xs font-medium text-muted-foreground">{formatDate(day, { weekday: 'short' })}</span>
                <span
                  className={cn(
                    'h-7 w-7 rounded-full text-sm font-semibold flex items-center justify-center',
                    isTodayCell && 'bg-primary text-primary-foreground',
                  )}
                >
                  {formatNumber(day.getDate(), { useGrouping: false })}
                </span>
              </div>

              {/* All-day + habit strip */}
              {hasAllDayContent && (
                <div className="px-1 pb-1 space-y-0.5 border-t border-border/30">
                  {dayAllDay.map((e) => (
                    <EventChip key={e.id} event={e} onClick={() => onEventClick(e)} />
                  ))}
                  {dayPlans.map((p) => (
                    <div
                      key={p.id}
                      className={cn(
                        'text-xs px-1.5 py-0.5 rounded truncate border-s-2',
                        p.priority === 'high'
                          ? 'bg-destructive/10 text-destructive border-destructive'
                          : p.priority === 'medium'
                            ? 'bg-warning/10 text-warning border-warning'
                            : 'bg-muted text-muted-foreground border-border',
                      )}
                    >
                      <bdi dir="auto">{p.title}</bdi>
                    </div>
                  ))}
                  {/* Habit dots */}
                  {hasHabits && (
                    <div className="flex items-center gap-0.5 pt-0.5 flex-wrap">
                      {habits.slice(0, 6).map((h) => {
                        const done = completed.has(h.id);
                        return (
                          <div
                            key={h.id}
                            className={cn('h-1.5 w-1.5 rounded-full shrink-0', done ? 'opacity-100' : 'opacity-20')}
                            style={{ backgroundColor: h.color || 'hsl(var(--chart-1))' }}
                            title={`${h.name}${done ? ' ✓' : ''}`}
                          />
                        );
                      })}
                    </div>
                  )}
                </div>
              )}
            </div>
          );
        })}
      </div>

      {/* Scrollable time grid */}
      <div ref={scrollRef} className="flex-1 overflow-y-auto">
        <div className="flex relative" style={{ height: TOTAL_HEIGHT }}>
          {/* Time axis */}
          <div className="w-14 shrink-0 relative">
            {HOURS.map((h) => (
              <div
                key={h}
                className="absolute start-0 end-0 flex items-start justify-end pe-2"
                style={{ top: h * PX_PER_HOUR - 8 }}
              >
                {h > 0 && (
                  <span className="text-[10px] text-muted-foreground tabular-nums">
                    <bdi dir="ltr">{formatNumber(h, { minimumIntegerDigits: 2, useGrouping: false })}:00</bdi>
                  </span>
                )}
              </div>
            ))}
          </div>

          {/* Day columns */}
          {days.map((day) => {
            const dateStr = format(day, 'yyyy-MM-dd');
            const isTodayCell = dateStr === todayStr;
            const dayEvents = events.filter((e) => !e.allDay && e.startDate === dateStr && e.startTime);
            const positioned = layoutEvents(dayEvents);

            return (
              <div
                key={dateStr}
              className={cn('flex-1 min-w-0 relative border-s border-border/30', isTodayCell && 'bg-primary/3', onSlotClick && 'cursor-pointer')}
                onClick={(e) => {
                  if (!onSlotClick) return;
                  const rect = e.currentTarget.getBoundingClientRect();
                  const y = e.clientY - rect.top;
                  onSlotClick(day, snapTimeFromY(y, TOTAL_HEIGHT));
                }}
              >
                {HOURS.map((h) => (
                  <div key={h} className="absolute start-0 end-0 border-t border-border/20" style={{ top: h * PX_PER_HOUR }} />
                ))}
                {HOURS.map((h) => (
                  <div
                    key={`${h}-half`}
                    className="absolute start-0 end-0 border-t border-border/10 border-dashed"
                    style={{ top: h * PX_PER_HOUR + PX_PER_HOUR / 2 }}
                  />
                ))}

                {isTodayCell && (
                  <div className="absolute start-0 end-0 z-10 flex items-center" style={{ top: nowTop }}>
                    <div className="h-2 w-2 rounded-full bg-primary -ms-1 shrink-0" />
                    <div className="flex-1 h-px bg-primary" />
                  </div>
                )}

                {positioned.map(({ event, startMinute, endMinute, column, totalColumns }) => {
                  const colW = 100 / totalColumns;
                  return (
                    <div
                      key={event.id}
                      className="absolute z-5 pe-0.5"
                      style={{
                        top: (startMinute / 60) * PX_PER_HOUR + 1,
                        height: Math.max(((endMinute - startMinute) / 60) * PX_PER_HOUR - 2, 20),
                        insetInlineStart: `${column * colW}%`,
                        width: `${colW}%`,
                      }}
                      onClick={(e) => e.stopPropagation()}
                    >
                      <EventChip event={event} onClick={() => onEventClick(event)} variant="block" className="h-full" />
                    </div>
                  );
                })}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
