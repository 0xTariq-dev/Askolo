import { useState, useEffect, useRef, useCallback } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import {
  format,
  startOfMonth,
  endOfMonth,
  startOfWeek,
  endOfWeek,
  addMonths,
  subMonths,
  addWeeks,
  subWeeks,
  addDays,
  subDays,
} from 'date-fns';
import { z } from 'zod';
import { useForm as useHookForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  Calendar as CalendarIcon,
  Plus,
  ChevronLeft,
  ChevronRight,
  RefreshCw,
} from 'lucide-react';
import {
  useListEvents,
  useCreateEvent,
  useUpdateEvent,
  useDeleteEvent,
  useGetGoogleStatus,
  useSyncGoogleCalendar,
  useCreateGoogleCalendarEvent,
  useUpdateGoogleCalendarEvent,
  useDeleteGoogleCalendarEvent,
  useListHabits,
  useListDailyPlans,
  useListHabitCompletions,
  getListEventsQueryKey,
  getGetDashboardSummaryQueryKey,
  type Event,
} from '@workspace/api-client-react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { PageTransition } from '@/components/ui/page-transition';
import { useToast } from '@/hooks/use-toast';
import { useLocation } from 'wouter';
import { cn } from '@/lib/utils';
import { THEME_PRESETS, useAskoloTheme } from '@/lib/theme';
import { MonthGrid } from '@/components/calendar/month-grid';
import { WeekGrid } from '@/components/calendar/week-grid';
import { DayGrid } from '@/components/calendar/day-grid';
import type { CalendarView } from '@/components/calendar/types';
import { useLocale } from '@/contexts/locale-context';

// ─── Form schema ────────────────────────────────────────────────────────────
const eventSchema = z.object({
  title: z.string().min(1, 'Title is required'),
  description: z.string().optional(),
  startDate: z.string().min(1, 'Date is required'),
  startTime: z.string().optional(),
  endDate: z.string().optional(),
  endTime: z.string().optional(),
  allDay: z.boolean().default(false),
  location: z.string().optional(),
});

type EventFormValues = z.infer<typeof eventSchema>;

const AUTO_SYNC_INTERVAL_MS = 5 * 60 * 1000;

// ─── Date range helpers ──────────────────────────────────────────────────────
function getViewRange(date: Date, view: CalendarView): { from: string; to: string } {
  switch (view) {
    case 'month': {
      const ms = startOfMonth(date);
      const me = endOfMonth(date);
      // Include the partial weeks shown in the grid
      return {
        from: format(startOfWeek(ms, { weekStartsOn: 1 }), 'yyyy-MM-dd'),
        to: format(endOfWeek(me, { weekStartsOn: 1 }), 'yyyy-MM-dd'),
      };
    }
    case 'week': {
      return {
        from: format(startOfWeek(date, { weekStartsOn: 1 }), 'yyyy-MM-dd'),
        to: format(endOfWeek(date, { weekStartsOn: 1 }), 'yyyy-MM-dd'),
      };
    }
    case 'day':
      return {
        from: format(date, 'yyyy-MM-dd'),
        to: format(date, 'yyyy-MM-dd'),
      };
  }
}

function navigatePrev(date: Date, view: CalendarView): Date {
  if (view === 'month') return subMonths(date, 1);
  if (view === 'week') return subWeeks(date, 1);
  return subDays(date, 1);
}

function navigateNext(date: Date, view: CalendarView): Date {
  if (view === 'month') return addMonths(date, 1);
  if (view === 'week') return addWeeks(date, 1);
  return addDays(date, 1);
}

function headerLabel(
  date: Date,
  view: CalendarView,
  formatLocaleDate: (value: Date | number, options?: Intl.DateTimeFormatOptions) => string,
): string {
  if (view === 'month') return formatLocaleDate(date, { month: 'long', year: 'numeric' });
  if (view === 'week') {
    const ws = startOfWeek(date, { weekStartsOn: 1 });
    const we = endOfWeek(date, { weekStartsOn: 1 });
    return `${formatLocaleDate(ws, { month: 'short', day: 'numeric' })} – ${formatLocaleDate(we, { month: 'short', day: 'numeric', year: 'numeric' })}`;
  }
  return formatLocaleDate(date, { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric' });
}

// ─── Page ───────────────────────────────────────────────────────────────────
export function CalendarPage() {
  const { formatDate, formatRelativeDate, t } = useLocale();
  const { preferences } = useAskoloTheme();
  const defaultEventColor =
    THEME_PRESETS[preferences.accountThemeId][preferences.mode].chart1;
  const qc = useQueryClient();
  const { toast } = useToast();
  const [currentDate, setCurrentDate] = useState(new Date());
  const [view, setView] = useState<CalendarView>('month');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingEvent, setEditingEvent] = useState<Event | null>(null);
  const [isSaving, setIsSaving] = useState(false);
  const [lastSyncedAt, setLastSyncedAt] = useState<Date | null>(null);
  const [, setTick] = useState(0);
  const [, setLocation] = useLocation();

  const todayStr = format(new Date(), 'yyyy-MM-dd');
  const { from, to } = getViewRange(currentDate, view);

  // ── Data ──────────────────────────────────────────────────────────────────
  const { data: events = [], isLoading } = useListEvents({ from, to });
  const { data: googleStatus, refetch: refetchGoogleStatus } = useGetGoogleStatus();
  const { data: habits = [] } = useListHabits();
  const { data: allPlans = [] } = useListDailyPlans();
  const { data: rawCompletions = [] } = useListHabitCompletions({ from, to });

  const calendarConnected = googleStatus?.calendarConnected ?? false;

  // ── Mutations ─────────────────────────────────────────────────────────────
  const createEvent = useCreateEvent();
  const updateEvent = useUpdateEvent();
  const deleteEvent = useDeleteEvent();
  const createGoogleEvent = useCreateGoogleCalendarEvent();
  const updateGoogleEvent = useUpdateGoogleCalendarEvent();
  const deleteGoogleEvent = useDeleteGoogleCalendarEvent();
  const syncGoogleCalendar = useSyncGoogleCalendar();

  // ── Form ──────────────────────────────────────────────────────────────────
  const form = useHookForm<EventFormValues>({
    resolver: zodResolver(eventSchema),
    defaultValues: {
      title: '',
      description: '',
      startDate: todayStr,
      startTime: '',
      endDate: '',
      endTime: '',
      allDay: false,
      location: '',
    },
  });

  const openAdd = (date?: Date, time?: string) => {
    setEditingEvent(null);
    form.reset({
      title: '',
      description: '',
      startDate: format(date ?? currentDate, 'yyyy-MM-dd'),
      startTime: time ?? '',
      endDate: '',
      endTime: '',
      allDay: false,
      location: '',
    });
    setDialogOpen(true);
  };

  const openEdit = (event: Event) => {
    setEditingEvent(event);
    form.reset({
      title: event.title,
      description: event.description || '',
      startDate: event.startDate,
      startTime: event.startTime || '',
      endDate: event.endDate || '',
      endTime: event.endTime || '',
      allDay: event.allDay,
      location: event.location || '',
    });
    setDialogOpen(true);
  };

  const invalidateCalendar = useCallback(() => {
    qc.invalidateQueries({ queryKey: getListEventsQueryKey() });
    qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
  }, [qc]);

  const handleMutationError = (action: string, error: Error) => {
    toast({ variant: 'destructive', title: `Failed to ${action}`, description: error.message || 'Please try again.' });
    setIsSaving(false);
  };

  const syncRef = useRef<(() => void) | null>(null);

  const onSubmit = (data: EventFormValues) => {
    setIsSaving(true);
    const payload = {
      ...data,
      endDate: data.endDate || data.startDate,
      color: editingEvent?.color || defaultEventColor,
    };
    const googleEventId = editingEvent?.googleEventId ?? undefined;

    if (editingEvent) {
      if (googleEventId && calendarConnected) {
        updateGoogleEvent.mutate(
          { eventId: googleEventId, data: { calendarId: 'primary', ...payload } },
          {
            onSuccess: () => {
              updateEvent.mutate(
                { id: editingEvent.id, data: payload },
                {
                  onSuccess: () => { invalidateCalendar(); setDialogOpen(false); setIsSaving(false); },
                  onError: (e) => handleMutationError('update local event', e),
                },
              );
            },
            onError: (e) => handleMutationError('update Google Calendar event', e),
          },
        );
      } else {
        updateEvent.mutate(
          { id: editingEvent.id, data: payload },
          {
            onSuccess: () => { invalidateCalendar(); setDialogOpen(false); setIsSaving(false); },
            onError: (e) => handleMutationError('update event', e),
          },
        );
      }
    } else if (calendarConnected) {
      createGoogleEvent.mutate(
        { data: { calendarId: 'primary', ...payload } },
        {
          onSuccess: (created) => {
            createEvent.mutate(
              { data: { ...payload, googleEventId: created.googleEventId } },
              {
                onSuccess: () => { invalidateCalendar(); setDialogOpen(false); setIsSaving(false); },
                onError: (e) => handleMutationError('save local event', e),
              },
            );
          },
          onError: (e) => handleMutationError('create Google Calendar event', e),
        },
      );
    } else {
      createEvent.mutate(
        { data: payload },
        {
          onSuccess: () => { invalidateCalendar(); setDialogOpen(false); setIsSaving(false); },
          onError: (e) => handleMutationError('create event', e),
        },
      );
    }
  };

  const handleDelete = (event: Event) => {
    if (!confirm('Delete this event?')) return;
    const googleEventId = event.googleEventId ?? undefined;
    if (googleEventId && calendarConnected) {
      deleteGoogleEvent.mutate(
        { eventId: googleEventId, data: { calendarId: 'primary' } },
        {
          onSuccess: () => {
            deleteEvent.mutate({ id: event.id }, {
              onSuccess: () => invalidateCalendar(),
              onError: (e) => handleMutationError('delete local event', e),
            });
          },
          onError: (e) => handleMutationError('delete Google Calendar event', e),
        },
      );
    } else {
      deleteEvent.mutate({ id: event.id }, {
        onSuccess: () => invalidateCalendar(),
        onError: (e) => handleMutationError('delete event', e),
      });
    }
  };

  const handleSync = useCallback(() => {
    const start = new Date(Date.now() - 30 * 86400000).toISOString();
    const end = new Date(Date.now() + 30 * 86400000).toISOString();
    syncGoogleCalendar.mutate(
      { data: { from: start, to: end } },
      {
        onSuccess: () => { invalidateCalendar(); setLastSyncedAt(new Date()); },
        onError: (e) => handleMutationError('sync Google Calendar', e),
      },
    );
  }, [syncGoogleCalendar, invalidateCalendar]); // eslint-disable-line react-hooks/exhaustive-deps

  syncRef.current = handleSync;

  useEffect(() => {
    if (!calendarConnected) return;
    syncRef.current?.();
    const id = setInterval(() => syncRef.current?.(), AUTO_SYNC_INTERVAL_MS);
    return () => clearInterval(id);
  }, [calendarConnected]);

  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 30_000);
    return () => clearInterval(id);
  }, []);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (params.get('google') === 'connected') {
      toast({ title: 'Google connected', description: 'Calendar access is now enabled.' });
      refetchGoogleStatus();
      invalidateCalendar();
      setLocation('/calendar', { replace: true });
    } else if (params.get('google') === 'error') {
      toast({ variant: 'destructive', title: 'Google connection failed', description: 'Please try connecting again.' });
      setLocation('/calendar', { replace: true });
    }
  }, [toast, refetchGoogleStatus, invalidateCalendar, setLocation]);

  // ── Shape data for child components ───────────────────────────────────────
  const habitItems = habits.map((h) => ({
    id: h.id,
    name: h.name,
    color: h.color,
  }));

  // Build completions lookup: dateStr → Set<habitId>
  const completionsByDate = new Map<string, Set<number>>();
  for (const c of rawCompletions) {
    if (!completionsByDate.has(c.date)) completionsByDate.set(c.date, new Set());
    completionsByDate.get(c.date)!.add(c.habitId);
  }

  const planItems = allPlans.map((p) => ({
    id: p.id,
    date: p.date,
    title: p.title,
    completed: p.completed,
    priority: p.priority,
  }));

  // Filter plans to current view range for child components
  const visiblePlans = planItems.filter((p) => p.date >= from && p.date <= to);

  // ── Render ─────────────────────────────────────────────────────────────────
  return (
    <PageTransition className="flex flex-col h-full max-w-6xl mx-auto gap-0">
      {/* ── Page header ─────────────────────────────────────────────────── */}
      <header className="flex flex-col sm:flex-row sm:items-end justify-between gap-4 mb-4 shrink-0">
        <div>
          <h1 className="text-3xl md:text-4xl font-display font-bold tracking-tight">Family Calendar</h1>
          <p className="text-muted-foreground mt-1 text-base">Local events and synced Google Calendar in one view.</p>
        </div>
        <div className="flex flex-col items-end gap-2 shrink-0">
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              onClick={handleSync}
              disabled={syncGoogleCalendar.isPending || !calendarConnected}
              title={!calendarConnected ? 'Google Calendar is not connected — sync is unavailable' : undefined}
              className="shrink-0"
            >
              <RefreshCw className={cn('h-4 w-4 mr-2', syncGoogleCalendar.isPending && 'animate-spin')} />
              {calendarConnected ? 'Sync Google' : 'Google not connected'}
            </Button>
            <Button onClick={() => openAdd()} className="shrink-0" data-testid="button-add-event">
              <Plus className="h-4 w-4 me-2" /> Add Event
            </Button>
          </div>
          {calendarConnected && (
            <p className="text-xs text-muted-foreground">
              {syncGoogleCalendar.isPending
                ? 'Syncing…'
                : lastSyncedAt
                  ? t('calendar.lastSynced', { time: formatRelativeDate(lastSyncedAt) })
                  : t('calendar.notSynced')}
            </p>
          )}
        </div>
      </header>

      {/* ── Calendar toolbar ─────────────────────────────────────────────── */}
      <div className="flex items-center justify-between mb-3 shrink-0 gap-3 flex-wrap">
        {/* Prev / label / Next */}
        <div className="flex items-center gap-2">
          <Button variant="outline" size="icon" aria-label={t('calendar.previous')} onClick={() => setCurrentDate((d) => navigatePrev(d, view))}>
            <ChevronLeft className="h-4 w-4 rtl:rotate-180" aria-hidden="true" />
          </Button>
          <h2 className="text-lg font-display font-semibold min-w-[200px] text-center">
            {headerLabel(currentDate, view, formatDate)}
          </h2>
          <Button variant="outline" size="icon" aria-label={t('calendar.next')} onClick={() => setCurrentDate((d) => navigateNext(d, view))}>
            <ChevronRight className="h-4 w-4 rtl:rotate-180" aria-hidden="true" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setCurrentDate(new Date())}
            className="text-xs text-muted-foreground ms-1"
          >
            {t('calendar.today')}
          </Button>
        </div>

        {/* View switcher */}
        <div className="flex items-center rounded-lg border border-border overflow-hidden">
          {(['month', 'week', 'day'] as CalendarView[]).map((v) => (
            <button
              key={v}
              onClick={() => setView(v)}
              aria-pressed={view === v}
              className={cn(
                'px-3 py-1.5 text-sm font-medium capitalize transition-colors',
                view === v
                  ? 'bg-primary text-primary-foreground'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground',
              )}
            >
              {t(`calendar.view${v[0].toUpperCase()}${v.slice(1)}`)}
            </button>
          ))}
        </div>
      </div>

      {/* ── Calendar body ────────────────────────────────────────────────── */}
      <div className="flex-1 min-h-0 border border-border rounded-2xl overflow-hidden bg-card shadow-sm">
        {isLoading ? (
          <div className="flex items-center justify-center h-full">
            <div className="space-y-3 w-full p-6">
              {[...Array(5)].map((_, i) => (
                <div key={i} className="h-16 bg-card rounded-xl animate-pulse" />
              ))}
            </div>
          </div>
        ) : events.length === 0 && view === 'month' ? (
          <EmptyState calendarConnected={calendarConnected} onSync={handleSync} onAdd={() => openAdd()} isPending={syncGoogleCalendar.isPending} />
        ) : view === 'month' ? (
          <MonthGrid
            currentDate={currentDate}
            events={events}
            plans={visiblePlans}
            habits={habitItems}
            completionsByDate={completionsByDate}
            todayStr={todayStr}
            onEventClick={openEdit}
            onDayClick={(day) => { setCurrentDate(day); setView('day'); }}
          />
        ) : view === 'week' ? (
          <WeekGrid
            currentDate={currentDate}
            events={events}
            plans={visiblePlans}
            habits={habitItems}
            completionsByDate={completionsByDate}
            todayStr={todayStr}
            onEventClick={openEdit}
            onSlotClick={(date, time) => openAdd(date, time)}
          />
        ) : (
          <DayGrid
            currentDate={currentDate}
            events={events}
            plans={visiblePlans}
            habits={habitItems}
            completionsByDate={completionsByDate}
            todayStr={todayStr}
            onEventClick={openEdit}
            onSlotClick={(date, time) => openAdd(date, time)}
          />
        )}
      </div>

      {/* ── CRUD dialog ─────────────────────────────────────────────────── */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-[480px]">
          <DialogHeader>
            <DialogTitle>{editingEvent ? 'Edit Event' : 'New Event'}</DialogTitle>
          </DialogHeader>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="title">Title</Label>
              <Input id="title" placeholder="e.g. Family dinner" {...form.register('title')} />
              {form.formState.errors.title && (
                <p className="text-xs text-destructive">{form.formState.errors.title.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Textarea id="description" placeholder="Add details..." {...form.register('description')} className="resize-none" />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="startDate">Start Date</Label>
                <Input id="startDate" type="date" {...form.register('startDate')} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="startTime">Start Time</Label>
                <Input id="startTime" type="time" {...form.register('startTime')} />
              </div>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="endDate">End Date</Label>
                <Input id="endDate" type="date" {...form.register('endDate')} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="endTime">End Time</Label>
                <Input id="endTime" type="time" {...form.register('endTime')} />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="location">Location</Label>
              <Input id="location" placeholder="e.g. Home" {...form.register('location')} />
            </div>
            <div className="flex items-center gap-2">
              <input id="allDay" type="checkbox" {...form.register('allDay')} className="accent-primary" />
              <Label htmlFor="allDay" className="mb-0">All day event</Label>
            </div>

            {/* Delete button for existing events */}
            {editingEvent && (
              <div className="pt-2 border-t border-border/50">
                <button
                  type="button"
                  onClick={() => { setDialogOpen(false); handleDelete(editingEvent); }}
                  className="text-sm text-destructive hover:underline"
                >
                  Delete this event
                </button>
              </div>
            )}

            <DialogFooter className="pt-4">
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={isSaving}>
                {isSaving ? 'Saving...' : editingEvent ? 'Save Changes' : 'Create Event'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </PageTransition>
  );
}

// ─── Empty state ─────────────────────────────────────────────────────────────
function EmptyState({
  calendarConnected,
  onSync,
  onAdd,
  isPending,
}: {
  calendarConnected: boolean;
  onSync: () => void;
  onAdd: () => void;
  isPending: boolean;
}) {
  return (
    <div className="flex items-center justify-center h-full">
      <div className="text-center py-16">
        <div className="h-16 w-16 bg-primary/10 rounded-full flex items-center justify-center mx-auto mb-4">
          <CalendarIcon className="h-8 w-8 text-primary" />
        </div>
        <h2 className="text-xl font-display font-semibold mb-2">No events this period</h2>
        <p className="text-muted-foreground mb-6 max-w-sm mx-auto">
          Add events manually or sync with Google Calendar.
        </p>
        <div className="flex justify-center gap-2">
          {calendarConnected && (
            <Button variant="outline" onClick={onSync} disabled={isPending}>
              <RefreshCw className={cn('h-4 w-4 mr-2', isPending && 'animate-spin')} /> Sync Google Calendar
            </Button>
          )}
          <Button onClick={onAdd}>
            <Plus className="h-4 w-4 mr-2" /> Add Event
          </Button>
        </div>
      </div>
    </div>
  );
}
