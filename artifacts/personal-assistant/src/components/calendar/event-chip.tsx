import { parseISO } from 'date-fns';
import { Clock, MapPin, Link2 } from 'lucide-react';
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card';
import { cn } from '@/lib/utils';
import type { Event } from '@workspace/api-client-react';
import { useLocale } from '@/contexts/locale-context';

interface EventChipProps {
  event: Event;
  onClick?: () => void;
  className?: string;
  /** compact = single-line chip (month/week); expanded = taller block (week/day time grid) */
  variant?: 'compact' | 'block';
}

export function EventChip({ event, onClick, className, variant = 'compact' }: EventChipProps) {
  const { direction, formatDate, t } = useLocale();
  const isGoogle = !!event.googleEventId;
  const color = event.color;
  const startDate = parseISO(`${event.startDate}T00:00:00`);
  const formattedStartDate = formatDate(startDate, { weekday: 'short', month: 'short', day: 'numeric' });

  return (
    <HoverCard openDelay={250} closeDelay={100}>
      <HoverCardTrigger asChild>
        <button
          onClick={onClick}
          className={cn(
        'text-start text-xs font-medium truncate cursor-pointer transition-opacity hover:opacity-80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
            variant === 'compact'
              ? 'w-full px-1.5 py-0.5 rounded'
              : 'w-full h-full px-2 py-1 rounded-md flex flex-col gap-0.5',
            !color && 'border-s-2 border-chart-1 bg-chart-1/10 text-chart-1',
            className,
          )}
          style={color ? { backgroundColor: `${color}25`, color, borderInlineStart: `2px solid ${color}` } : undefined}
          dir="auto"
        >
          <span className="truncate block" dir="auto">{event.title}</span>
          {variant === 'block' && !event.allDay && event.startTime && (
            <span className="truncate block opacity-80 text-[10px]"><bdi dir="ltr">{event.startTime}</bdi></span>
          )}
        </button>
      </HoverCardTrigger>
      <HoverCardContent className="w-72 p-0 shadow-lg" side={direction === 'rtl' ? 'left' : 'right'} align="start">
        <div className="p-3 space-y-2">
          {/* title row */}
          <div className="flex items-start gap-2">
            <div
              className={cn('h-3 w-3 rounded-sm mt-0.5 shrink-0', !color && 'bg-chart-1')}
              style={color ? { backgroundColor: color } : undefined}
            />
            <div className="flex-1 min-w-0">
              <p className="font-semibold text-sm leading-tight" dir="auto">{event.title}</p>
              {isGoogle && (
                <span lang="en" dir="ltr" className="text-xs bg-info/10 text-info border border-info/20 px-1.5 py-0.5 rounded inline-flex items-center gap-1 mt-1">
                  <Link2 className="h-3 w-3" aria-hidden="true" /> Google
                </span>
              )}
            </div>
          </div>

          {/* time */}
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <Clock className="h-3 w-3 shrink-0" />
            {event.allDay ? (
              <span>{t('calendar.allDay')} · <bdi>{formattedStartDate}</bdi></span>
            ) : (
              <span>
                <bdi>{formattedStartDate}</bdi>
                {event.startTime && <> · <bdi dir="ltr">{event.startTime}</bdi></>}
                {event.endTime && <> – <bdi dir="ltr">{event.endTime}</bdi></>}
              </span>
            )}
          </div>

          {/* location */}
          {event.location && (
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <MapPin className="h-3 w-3 shrink-0" />
              <span className="truncate" dir="auto">{event.location}</span>
            </div>
          )}

          {/* description */}
          {event.description && (
            <p className="text-xs text-muted-foreground line-clamp-3" dir="auto">{event.description}</p>
          )}

          <div className="pt-1 border-t border-border/50">
            <button onClick={onClick} className="text-xs text-primary hover:underline">
              Edit event
            </button>
          </div>
        </div>
      </HoverCardContent>
    </HoverCard>
  );
}
