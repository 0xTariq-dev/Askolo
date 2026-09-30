import { useEffect, useRef } from 'react';
import { Bell, Info, AlertTriangle, Zap, X, CheckCheck, VolumeX } from 'lucide-react';
import { useLocation } from 'wouter';
import { useLocale } from '@/contexts/locale-context';
import { useNotifications, type AppNotification } from '@/contexts/notification-context';
import { Button } from '@/components/ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { cn } from '@/lib/utils';

function typeIcon(type: AppNotification['type']) {
  if (type === 'warning') return <AlertTriangle className="h-3.5 w-3.5 text-warning shrink-0 mt-0.5" />;
  if (type === 'action') return <Zap className="h-3.5 w-3.5 text-primary shrink-0 mt-0.5" />;
  return <Info className="h-3.5 w-3.5 text-info shrink-0 mt-0.5" />;
}

export function NotificationBell({ className }: { className?: string }) {
  const { formatRelativeDate, formatNumber } = useLocale();
  const { notifications, unreadCount, markAllRead, clearAll, markRead, suppressNotification } = useNotifications();
  const [, setLocation] = useLocation();
  const popoverOpenRef = useRef(false);

  return (
    <Popover
      onOpenChange={(open) => {
        popoverOpenRef.current = open;
        if (open) markAllRead();
      }}
    >
      <PopoverTrigger asChild>
        <button
          aria-label={`Notifications${unreadCount > 0 ? ` (${unreadCount} unread)` : ''}`}
          className={cn(
            'relative h-8 w-8 flex items-center justify-center rounded-md text-sidebar-foreground/60 hover:text-sidebar-foreground hover:bg-sidebar-accent transition-colors',
            className,
          )}
        >
          <Bell className="h-4 w-4" />
          {unreadCount > 0 && (
            <span
              className="absolute -top-0.5 -right-0.5 h-4 min-w-4 px-0.5 flex items-center justify-center rounded-full bg-primary text-primary-foreground text-[9px] font-bold leading-none"
              aria-hidden
            >
              {unreadCount > 9 ? '9+' : formatNumber(unreadCount, { useGrouping: false })}
            </span>
          )}
        </button>
      </PopoverTrigger>

      <PopoverContent
        side="bottom"
        align="end"
        sideOffset={8}
        className="w-80 p-0 overflow-hidden"
      >
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-2.5 border-b border-border bg-card/50">
          <span className="text-sm font-semibold">Notifications</span>
          {notifications.length > 0 && (
            <button
              onClick={clearAll}
              className="text-xs text-muted-foreground hover:text-foreground transition-colors flex items-center gap-1"
            >
              <X className="h-3 w-3" /> Clear all
            </button>
          )}
        </div>

        {/* List */}
        <div className="max-h-[360px] overflow-y-auto divide-y divide-border/50">
          {notifications.length === 0 ? (
            <div className="py-10 text-center text-sm text-muted-foreground">
              <CheckCheck className="h-6 w-6 mx-auto mb-2 opacity-40" />
              You're all caught up
            </div>
          ) : (
            notifications.map((n) => (
              <div
                key={n.id}
                className={cn(
                  'px-4 py-3 flex gap-3 transition-colors',
                  !n.read ? 'bg-muted/40' : '',
                )}
              >
                {typeIcon(n.type)}
                <div className="flex-1 min-w-0">
                  <p className={cn('text-xs font-semibold leading-snug', n.read ? 'text-foreground/70' : 'text-foreground')}>
                    {n.title}
                  </p>
                  <p className="text-xs text-muted-foreground mt-0.5 leading-relaxed">{n.body}</p>
                  <div className="mt-1.5 flex flex-wrap items-center gap-3">
                    {n.action && (
                      <button
                        onClick={() => {
                          markRead(n.id);
                          setLocation(n.action!.href);
                        }}
                        className="text-xs font-medium text-primary hover:text-primary/80 transition-colors"
                      >
                        {n.action.label} →
                      </button>
                    )}
                    {n.suppressKey && (
                      <button
                        onClick={() => {
                          if (n.suppressKey) suppressNotification(n.id, n.suppressKey);
                        }}
                        className="text-xs text-muted-foreground hover:text-foreground transition-colors flex items-center gap-1"
                      >
                        <VolumeX className="h-3 w-3" /> Don't remind me
                      </button>
                    )}
                  </div>
                  <p className="text-[10px] text-muted-foreground/60 mt-1">
                    {formatRelativeDate(n.timestamp)}
                  </p>
                </div>
                {!n.read && (
                  <div className="h-2 w-2 rounded-full bg-primary shrink-0 mt-1" aria-label="Unread" />
                )}
              </div>
            ))
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}
