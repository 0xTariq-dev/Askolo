import { useEffect, useMemo, useState } from 'react';
import { useConfirmDialog } from '@/contexts/confirm-dialog-context';
import { addDays, endOfWeek, format, startOfMonth, startOfWeek } from 'date-fns';
import { RefreshCw, Trash2 } from 'lucide-react';
import {
  useGetGoogleStatus,
} from '@workspace/api-client-react';
import type { CalendarEvent } from '@/components/reui/event-calendar/event-calendar-types';
import { Pattern } from '@/components/examples/c-event-calendar-1';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { useLocale } from '@/contexts/locale-context';
import { useSpaceScope } from '@/contexts/space-context';
import { useToast } from '@/hooks/use-toast';
import {
  createV2CalendarEvent,
  deleteV2CalendarEvent,
  getV2AgendaRange,
  syncV2GoogleCalendar,
  updateV2CalendarEvent,
  type V2AgendaEvent,
} from '@/lib/agenda-v2-api';

function toDate(date: string, time?: string | null) {
  return new Date(`${date.slice(0, 10)}T${time || '00:00'}:00`);
}

function toCalendarEvent(event: V2AgendaEvent): CalendarEvent<V2AgendaEvent> {
  const start = new Date(event.startsAt);
  let end = event.endsAt ? new Date(event.endsAt) : start;
  if (event.allDay && event.endsAt) end = new Date(event.endsAt);
  else if (end <= start) end = new Date(start.getTime() + 30 * 60_000);
  return {
    id: `${event.workspaceId ?? ''}:${event.id}`,
    title: event.workspaceName ? `${event.workspaceName}: ${event.title}` : event.title,
    start,
    end,
    allDay: event.allDay,
    color: event.color || 'hsl(var(--primary))',
    data: event,
    readOnly: Boolean(event.sourceProvider),
    draggable: false,
    resizable: false,
  };
}

export function AgendaCalendar({ onCountChange }: { onCountChange?: (count: number) => void }) {
  const { confirm } = useConfirmDialog();
  const { locale, weekStartsOn } = useLocale();
  const { selectedSpaceId, spaces } = useSpaceScope();
  const { toast } = useToast();
  const [range, setRange] = useState(() => {
    const today = new Date();
    const from = startOfWeek(startOfMonth(today), { weekStartsOn });
    const to = endOfWeek(new Date(today.getFullYear(), today.getMonth() + 1, 0), { weekStartsOn });
    return { from: format(from, 'yyyy-MM-dd'), to: format(to, 'yyyy-MM-dd') };
  });
  const [editing, setEditing] = useState<V2AgendaEvent | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [title, setTitle] = useState('');
  const [startDate, setStartDate] = useState(format(new Date(), 'yyyy-MM-dd'));
  const [startTime, setStartTime] = useState('');
  const [saving, setSaving] = useState(false);
  const [events, setEvents] = useState<V2AgendaEvent[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isError, setIsError] = useState(false);
  const [refreshKey, setRefreshKey] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setIsLoading(true);
    setIsError(false);
    getV2AgendaRange(selectedSpaceId, {
      ...range,
      timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    }, controller.signal)
      .then((agenda) => setEvents(agenda.calendarEvents))
      .catch(() => {
        if (!controller.signal.aborted) setIsError(true);
      })
      .finally(() => {
        if (!controller.signal.aborted) setIsLoading(false);
      });
    return () => controller.abort();
  }, [range, selectedSpaceId, refreshKey]);
  useEffect(() => onCountChange?.(events.length), [events.length, onCountChange]);
  const { data: googleStatus } = useGetGoogleStatus();
  const personalWorkspaceId = spaces.find((space) => space.isPersonal)?.id;
  const syncWorkspaceId = selectedSpaceId ?? personalWorkspaceId;
  const calendarEvents = useMemo(() => events.map(toCalendarEvent), [events]);
  const isArabic = locale === 'ar';

  const openCreate = (date = new Date(), time = '') => {
    setEditing(null);
    setTitle('');
    setStartDate(format(date, 'yyyy-MM-dd'));
    setStartTime(time);
    setDialogOpen(true);
  };

  const openEdit = (event: V2AgendaEvent) => {
    if (event.sourceProvider) {
      toast({ title: isArabic ? 'حدث من التقويم' : 'Provider event', description: isArabic ? 'هذا الحدث للقراءة فقط.' : 'Provider events are read-only here.' });
      return;
    }
    setEditing(event);
    setTitle(event.title);
    const start = new Date(event.startsAt);
    setStartDate(format(start, 'yyyy-MM-dd'));
    setStartTime(event.allDay ? '' : format(start, 'HH:mm'));
    setDialogOpen(true);
  };

  const refresh = () => {
    setRefreshKey((key) => key + 1);
  };

  const saveEvent = async (formEvent: React.FormEvent<HTMLFormElement>) => {
    formEvent.preventDefault();
    if (!title.trim()) return;
    setSaving(true);
    const startsAt = new Date(`${startDate}T${startTime || '00:00'}:00`);
    const endsAt = startTime
      ? new Date(startsAt.getTime() + 30 * 60_000)
      : addDays(new Date(`${startDate}T00:00:00`), 1);
    const payload = {
      title: title.trim(),
      startsAt: startsAt.toISOString(),
      endsAt: endsAt.toISOString(),
      allDay: !startTime,
      timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      description: editing?.description || '',
      location: editing?.location || '',
      color: editing?.color || '#64748b',
    };
    try {
      const workspaceId = editing?.workspaceId ?? selectedSpaceId
        ?? spaces.find((space) => space.isPersonal)?.id ?? spaces[0]?.id;
      if (!workspaceId) throw new Error('Choose a Space before creating an event.');
      if (editing) await updateV2CalendarEvent(workspaceId, editing.id, payload);
      else await createV2CalendarEvent(workspaceId, payload);
      setDialogOpen(false);
      refresh();
    } catch (error) {
      toast({ variant: 'destructive', title: isArabic ? 'تعذر حفظ الحدث' : 'Event could not be saved', description: error instanceof Error ? error.message : undefined });
    } finally {
      setSaving(false);
    }
  };

  const removeEvent = async () => {
    if (!editing || !(await confirm({
      title: isArabic ? 'حذف الحدث؟' : 'Delete event?',
      description: isArabic ? 'سيُحذف هذا الحدث من التقويم.' : 'This event will be removed from the calendar.',
      confirmLabel: isArabic ? 'حذف' : 'Delete',
      variant: 'nested',
    }))) return;
    setSaving(true);
    try {
      const workspaceId = editing.workspaceId ?? selectedSpaceId;
      if (!workspaceId) throw new Error('This event has no Space owner.');
      await deleteV2CalendarEvent(workspaceId, editing.id);
      setDialogOpen(false);
      refresh();
    } catch (error) {
      toast({ variant: 'destructive', title: isArabic ? 'تعذر حذف الحدث' : 'Event could not be deleted', description: error instanceof Error ? error.message : undefined });
    } finally {
      setSaving(false);
    }
  };

  return (
    <section className="min-w-0" aria-label={isArabic ? 'التقويم' : 'Calendar'}>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 className="font-display text-xl font-semibold">{isArabic ? 'التقويم' : 'Calendar'}</h2>
          <p className="text-sm text-muted-foreground">{isArabic ? 'أحداثك، مباشرة من تقويمك' : 'Your events, synced with your calendar data.'}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          {googleStatus?.calendarConnected && (
            <Button type="button" variant="outline" size="sm" onClick={() => {
              if (!syncWorkspaceId) return;
              void syncV2GoogleCalendar(syncWorkspaceId, {
                from: new Date(Date.now() - 30 * 86400000).toISOString(),
                to: new Date(Date.now() + 30 * 86400000).toISOString(),
              }).then(refresh).catch((error: unknown) => toast({
                variant: 'destructive',
                title: isArabic ? 'تعذرت المزامنة' : 'Sync failed',
                description: error instanceof Error ? error.message : undefined,
              }));
            }} disabled={!syncWorkspaceId}>
              <RefreshCw className="me-2 size-4" />{isArabic ? 'مزامنة' : 'Sync Google'}
            </Button>
          )}
        </div>
      </div>
      {isError ? (
        <div className="rounded-xl border border-destructive/30 bg-destructive/5 p-5 text-sm" role="alert">
          <p className="text-destructive">{isArabic ? 'تعذر تحميل الأحداث.' : 'Calendar events could not be loaded.'}</p>
          <Button variant="outline" size="sm" className="mt-3" onClick={refresh}>{isArabic ? 'إعادة المحاولة' : 'Try again'}</Button>
        </div>
      ) : (
        <Pattern
          events={calendarEvents}
          loading={isLoading}
          onAddEvent={() => openCreate()}
          onEventClick={({ event }) => {
            if (event.data) openEdit(event.data);
          }}
          onSlotClick={(slot) => openCreate(slot.date, slot.allDay ? '' : format(slot.date, 'HH:mm'))}
          onRangeChange={({ range: visible }) => {
            const from = format(visible.start, 'yyyy-MM-dd');
            const to = format(addDays(visible.end, -1), 'yyyy-MM-dd');
            setRange((current) => current.from === from && current.to === to ? current : { from, to });
          }}
        />
      )}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader><DialogTitle>{editing ? (isArabic ? 'تعديل الحدث' : 'Edit event') : (isArabic ? 'إضافة حدث' : 'Add event')}</DialogTitle></DialogHeader>
          <form onSubmit={(event) => void saveEvent(event)} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="agenda-event-title">{isArabic ? 'العنوان' : 'Title'}</Label>
              <Input id="agenda-event-title" value={title} onChange={(event) => setTitle(event.target.value)} required autoFocus />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2"><Label htmlFor="agenda-event-date">{isArabic ? 'التاريخ' : 'Date'}</Label><Input id="agenda-event-date" type="date" value={startDate} onChange={(event) => setStartDate(event.target.value)} required /></div>
              <div className="space-y-2"><Label htmlFor="agenda-event-time">{isArabic ? 'الوقت' : 'Time'}</Label><Input id="agenda-event-time" type="time" value={startTime} onChange={(event) => setStartTime(event.target.value)} /></div>
            </div>
            <DialogFooter className="gap-2 sm:justify-between">
              {editing && <Button type="button" variant="ghost" className="text-destructive" onClick={() => void removeEvent()} disabled={saving}><Trash2 className="me-2 size-4" />{isArabic ? 'حذف' : 'Delete'}</Button>}
              <div className="flex gap-2">
                <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>{isArabic ? 'إلغاء' : 'Cancel'}</Button>
                <Button type="submit" disabled={saving || !title.trim()}>{saving ? (isArabic ? 'جارٍ الحفظ…' : 'Saving…') : (isArabic ? 'حفظ' : 'Save')}</Button>
              </div>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </section>
  );
}
