import { format, parseISO } from 'date-fns';
import { Clock, MapPin, Link2 } from 'lucide-react';
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card';
import { cn } from '@/lib/utils';
import type { Event } from '@workspace/api-client-react';

interface EventChipProps {
  event: Event;
  onClick?: () => void;
  className?: string;
  /** compact = single-line chip (month/week); expanded = taller block (week/day time grid) */
  variant?: 'compact' | 'block';
}

export function EventChip({ event, onClick, className, variant = 'compact' }: EventChipProps) {
  const isGoogle = !!event.googleEventId;
  const color = event.color || '#3b82f6';
  const bg = color + '25';

  return (
    <HoverCard openDelay={250} closeDelay={100}>
      <HoverCardTrigger asChild>
        <button
          onClick={onClick}
          className={cn(
            'text-left text-xs font-medium truncate cursor-pointer transition-opacity hover:opacity-80 focus:outline-none',
            variant === 'compact'
              ? 'w-full px-1.5 py-0.5 rounded'
              : 'w-full h-full px-2 py-1 rounded-md flex flex-col gap-0.5',
            className,
          )}
          style={{ backgroundColor: bg, color, borderLeft: `2px solid ${color}` }}
        >
          <span className="truncate block">{event.title}</span>
          {variant === 'block' && !event.allDay && event.startTime && (
            <span className="truncate block opacity-80 text-[10px]">{event.startTime}</span>
          )}
        </button>
      </HoverCardTrigger>
      <HoverCardContent className="w-72 p-0 shadow-lg" side="right" align="start">
        <div className="p-3 space-y-2">
          {/* title row */}
          <div className="flex items-start gap-2">
            <div className="h-3 w-3 rounded-sm mt-0.5 shrink-0" style={{ backgroundColor: color }} />
            <div className="flex-1 min-w-0">
              <p className="font-semibold text-sm leading-tight">{event.title}</p>
              {isGoogle && (
                <span className="text-xs bg-blue-500/10 text-blue-400 border border-blue-500/20 px-1.5 py-0.5 rounded inline-flex items-center gap-1 mt-1">
                  <Link2 className="h-3 w-3" /> Google
                </span>
              )}
            </div>
          </div>

          {/* time */}
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <Clock className="h-3 w-3 shrink-0" />
            {event.allDay ? (
              <span>All day · {format(parseISO(event.startDate + 'T00:00:00'), 'EEE, MMM d')}</span>
            ) : (
              <span>
                {format(parseISO(event.startDate + 'T00:00:00'), 'EEE, MMM d')}
                {event.startTime && ` · ${event.startTime}`}
                {event.endTime && ` – ${event.endTime}`}
              </span>
            )}
          </div>

          {/* location */}
          {event.location && (
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <MapPin className="h-3 w-3 shrink-0" />
              <span className="truncate">{event.location}</span>
            </div>
          )}

          {/* description */}
          {event.description && (
            <p className="text-xs text-muted-foreground line-clamp-3">{event.description}</p>
          )}

          <div className="pt-1 border-t border-border/50">
            <button onClick={onClick} className="text-xs text-primary hover:underline">
              Edit event →
            </button>
          </div>
        </div>
      </HoverCardContent>
    </HoverCard>
  );
}
