import { useRef, useEffect } from 'react';
import { format } from 'date-fns';
import { cn } from '@workspace/askolo-design-system/lib/utils';
import { EventChip } from './event-chip';
import { layoutEvents } from './types';
import { snapTimeFromY } from './snap-time';
import type { Event } from '@workspace/api-client-react';
import type { DailyPlan, Habit } from './types';
import { useLocale } from '@/contexts/locale-context';

const PX_PER_HOUR = 64;
const TOTAL_HEIGHT = PX_PER_HOUR * 24;
const HOURS = Array.from({ length: 24 }, (_, i) => i);

interface DayGridProps {
  currentDate: Date;
  events: Event[];
  plans: DailyPlan[];
  habits: Habit[];
  completionsByDate: Map<string, Set<number>>;
  todayStr: string;
  onEventClick: (event: Event) => void;
  onSlotClick?: (date: Date, time: string) => void;
}

export function DayGrid({
  currentDate,
  events,
  plans,
  habits,
  completionsByDate,
  todayStr,
  onEventClick,
  onSlotClick,
}: DayGridProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const { formatDate, formatNumber, t } = useLocale();
  const dateStr = format(currentDate, 'yyyy-MM-dd');
  const isTodayCell = dateStr === todayStr;

  useEffect(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = PX_PER_HOUR * 7;
  }, []);

  const now = new Date();
  const nowTop = ((now.getHours() * 60 + now.getMinutes()) / 60) * PX_PER_HOUR;

  const allDayEvents = events.filter((e) => {
    const end = e.endDate || e.startDate;
    return e.allDay && e.startDate <= dateStr && end >= dateStr;
  });
  const dayPlans = plans.filter((p) => p.date === dateStr);
  const timedEvents = events.filter((e) => !e.allDay && e.startDate === dateStr && e.startTime);
  const positioned = layoutEvents(timedEvents);

  const completed = completionsByDate.get(dateStr) ?? new Set<number>();
  const hasHabits = habits.length > 0;
  const hasAllDay = allDayEvents.length > 0 || dayPlans.length > 0 || hasHabits;

  return (
    <div className="flex flex-col flex-1 min-h-0 overflow-hidden">
      {/* Day header */}
      <div
        className={cn('flex items-center gap-3 px-4 py-3 border-b border-border/50 shrink-0', isTodayCell && 'text-primary')}
      >
        <div
          className={cn(
            'h-10 w-10 rounded-full flex items-center justify-center text-xl font-bold',
            isTodayCell ? 'bg-primary text-primary-foreground' : 'bg-muted',
          )}
        >
          {formatNumber(currentDate.getDate(), { useGrouping: false })}
        </div>
        <div>
          <p className="font-semibold">{formatDate(currentDate, { weekday: 'long' })}</p>
          <p className="text-sm text-muted-foreground">{formatDate(currentDate, { month: 'long', year: 'numeric' })}</p>
        </div>
      </div>

      {/* All-day + habit strip */}
      {hasAllDay && (
        <div className="px-4 py-2 border-b border-border/40 space-y-1">
          <p className="text-xs text-muted-foreground mb-1">{t('calendar.allDay')}</p>
          {allDayEvents.map((e) => (
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
              {p.completed ? <s className="opacity-60"><bdi dir="auto">{p.title}</bdi></s> : <bdi dir="auto">{p.title}</bdi>}
            </div>
          ))}
          {/* Habit dots */}
          {hasHabits && (
            <div className="flex items-center gap-1 flex-wrap pt-1">
              {habits.map((h) => {
                const done = completed.has(h.id);
                return (
                  <div
                    key={h.id}
                    className="flex items-center gap-1"
                    title={`${h.name}${done ? ' ✓' : ''}`}
                  >
                    <div
                      className={cn('h-2 w-2 rounded-full shrink-0', done ? 'opacity-100' : 'opacity-25')}
                      style={{ backgroundColor: h.color || 'hsl(var(--chart-1))' }}
                    />
                    <span className={cn('text-xs', done ? 'text-foreground' : 'text-muted-foreground/50 line-through')}>
                      <bdi dir="auto">{h.name}</bdi>
                    </span>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}

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

          {/* Single day column */}
          <div
            className={cn('flex-1 relative', onSlotClick && 'cursor-pointer')}
            onClick={(e) => {
              if (!onSlotClick) return;
              const rect = e.currentTarget.getBoundingClientRect();
              const y = e.clientY - rect.top;
              onSlotClick(currentDate, snapTimeFromY(y, TOTAL_HEIGHT));
            }}
          >
            {HOURS.map((h) => (
              <div key={h} className="absolute start-0 end-0 border-t border-border/25" style={{ top: h * PX_PER_HOUR }} />
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
                  className="absolute z-5 pe-1"
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
        </div>
      </div>
    </div>
  );
}
