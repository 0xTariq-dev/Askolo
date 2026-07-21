import { useRef, useEffect } from 'react';
import { format, eachDayOfInterval, startOfWeek, endOfWeek, isToday } from 'date-fns';
import { cn } from '@/lib/utils';
import { EventChip } from './event-chip';
import { layoutEvents } from './types';
import type { Event } from '@workspace/api-client-react';
import type { DailyPlan } from './types';

const PX_PER_HOUR = 64;
const TOTAL_HEIGHT = PX_PER_HOUR * 24;
const HOURS = Array.from({ length: 24 }, (_, i) => i);

interface WeekGridProps {
  currentDate: Date;
  events: Event[];
  plans: DailyPlan[];
  todayStr: string;
  onEventClick: (event: Event) => void;
}

export function WeekGrid({ currentDate, events, plans, todayStr, onEventClick }: WeekGridProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const weekStart = startOfWeek(currentDate, { weekStartsOn: 1 });
  const weekEnd = endOfWeek(currentDate, { weekStartsOn: 1 });
  const days = eachDayOfInterval({ start: weekStart, end: weekEnd });

  // Scroll to 8am on mount
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = PX_PER_HOUR * 7;
    }
  }, []);

  // Current time indicator
  const now = new Date();
  const nowMinutes = now.getHours() * 60 + now.getMinutes();
  const nowTop = (nowMinutes / 60) * PX_PER_HOUR;

  return (
    <div className="flex flex-col flex-1 min-h-0 overflow-hidden">
      {/* Day headers */}
      <div className="flex border-b border-border/50 shrink-0">
        <div className="w-14 shrink-0" /> {/* time gutter */}
        {days.map((day) => {
          const dateStr = format(day, 'yyyy-MM-dd');
          const isTodayCell = dateStr === todayStr;
          const dayAllDay = events.filter((e) => {
            const end = e.endDate || e.startDate;
            return e.allDay && e.startDate <= dateStr && end >= dateStr;
          });
          const dayPlans = plans.filter((p) => p.date === dateStr);
          const hasAllDay = dayAllDay.length > 0 || dayPlans.length > 0;

          return (
            <div key={dateStr} className="flex-1 min-w-0 border-l border-border/30">
              {/* Day label */}
              <div className={cn('flex flex-col items-center py-2', isTodayCell && 'text-primary')}>
                <span className="text-xs font-medium text-muted-foreground">{format(day, 'EEE')}</span>
                <span
                  className={cn(
                    'h-7 w-7 rounded-full text-sm font-semibold flex items-center justify-center',
                    isTodayCell && 'bg-primary text-primary-foreground',
                  )}
                >
                  {format(day, 'd')}
                </span>
              </div>
              {/* All-day strip */}
              {hasAllDay && (
                <div className="px-1 pb-1 space-y-0.5 border-t border-border/30">
                  {dayAllDay.map((e) => (
                    <EventChip key={e.id} event={e} onClick={() => onEventClick(e)} />
                  ))}
                  {dayPlans.map((p) => (
                    <div
                      key={p.id}
                      className="text-xs px-1.5 py-0.5 rounded truncate"
                      style={{
                        backgroundColor: p.priority === 'high' ? '#ef444420' : '#f59e0b20',
                        color: p.priority === 'high' ? '#ef4444' : '#f59e0b',
                        borderLeft: `2px solid ${p.priority === 'high' ? '#ef4444' : '#f59e0b'}`,
                      }}
                    >
                      {p.title}
                    </div>
                  ))}
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
                className="absolute left-0 right-0 flex items-start justify-end pr-2"
                style={{ top: h * PX_PER_HOUR - 8 }}
              >
                {h > 0 && (
                  <span className="text-[10px] text-muted-foreground tabular-nums">
                    {String(h).padStart(2, '0')}:00
                  </span>
                )}
              </div>
            ))}
          </div>

          {/* Day columns */}
          {days.map((day) => {
            const dateStr = format(day, 'yyyy-MM-dd');
            const isTodayCell = dateStr === todayStr;
            const dayEvents = events.filter(
              (e) => !e.allDay && e.startDate === dateStr && e.startTime,
            );
            const positioned = layoutEvents(dayEvents);

            return (
              <div
                key={dateStr}
                className={cn('flex-1 min-w-0 relative border-l border-border/30', isTodayCell && 'bg-primary/3')}
              >
                {/* Hour lines */}
                {HOURS.map((h) => (
                  <div
                    key={h}
                    className="absolute left-0 right-0 border-t border-border/20"
                    style={{ top: h * PX_PER_HOUR }}
                  />
                ))}
                {/* Half-hour lines */}
                {HOURS.map((h) => (
                  <div
                    key={`${h}-half`}
                    className="absolute left-0 right-0 border-t border-border/10 border-dashed"
                    style={{ top: h * PX_PER_HOUR + PX_PER_HOUR / 2 }}
                  />
                ))}

                {/* Current time indicator */}
                {isTodayCell && (
                  <div
                    className="absolute left-0 right-0 z-10 flex items-center"
                    style={{ top: nowTop }}
                  >
                    <div className="h-2 w-2 rounded-full bg-primary -ml-1 shrink-0" />
                    <div className="flex-1 h-px bg-primary" />
                  </div>
                )}

                {/* Timed events */}
                {positioned.map(({ event, startMinute, endMinute, column, totalColumns }) => {
                  const colW = 100 / totalColumns;
                  return (
                    <div
                      key={event.id}
                      className="absolute z-5 pr-0.5"
                      style={{
                        top: (startMinute / 60) * PX_PER_HOUR + 1,
                        height: Math.max(((endMinute - startMinute) / 60) * PX_PER_HOUR - 2, 20),
                        left: `${column * colW}%`,
                        width: `${colW}%`,
                      }}
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
