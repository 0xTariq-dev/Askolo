import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { format, startOfMonth, endOfMonth, addMonths, subMonths } from 'date-fns';
import { z } from 'zod';
import { useForm as useHookForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  Calendar as CalendarIcon,
  Plus,
  ChevronLeft,
  ChevronRight,
  RefreshCw,
  Trash2,
  Edit2,
  MoreVertical,
  Clock,
  MapPin,
  Link2,
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
  getListEventsQueryKey,
  getGetDashboardSummaryQueryKey,
  Event,
} from '@workspace/api-client-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { PageTransition } from '@/components/ui/page-transition';
import { useToast } from '@/hooks/use-toast';
import { cn } from '@/lib/utils';

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

const DEFAULT_COLOR = '#3b82f6';

export function CalendarPage() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [currentDate, setCurrentDate] = useState(new Date());
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingEvent, setEditingEvent] = useState<Event | null>(null);
  const [isSaving, setIsSaving] = useState(false);

  const from = format(startOfMonth(currentDate), 'yyyy-MM-dd');
  const to = format(endOfMonth(currentDate), 'yyyy-MM-dd');

  const { data: events, isLoading } = useListEvents({ from, to });
  const { data: googleStatus } = useGetGoogleStatus();
  const createEvent = useCreateEvent();
  const updateEvent = useUpdateEvent();
  const deleteEvent = useDeleteEvent();
  const createGoogleEvent = useCreateGoogleCalendarEvent();
  const updateGoogleEvent = useUpdateGoogleCalendarEvent();
  const deleteGoogleEvent = useDeleteGoogleCalendarEvent();
  const syncGoogleCalendar = useSyncGoogleCalendar();

  const calendarConnected = googleStatus?.calendarConnected ?? false;

  const form = useHookForm<EventFormValues>({
    resolver: zodResolver(eventSchema),
    defaultValues: {
      title: '',
      description: '',
      startDate: format(new Date(), 'yyyy-MM-dd'),
      startTime: '',
      endDate: '',
      endTime: '',
      allDay: false,
      location: '',
    },
  });

  const openAdd = () => {
    setEditingEvent(null);
    form.reset({
      title: '',
      description: '',
      startDate: format(currentDate, 'yyyy-MM-dd'),
      startTime: '',
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

  const invalidateCalendar = () => {
    qc.invalidateQueries({ queryKey: getListEventsQueryKey() });
    qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
  };

  const handleMutationError = (action: string, error: Error) => {
    toast({
      variant: 'destructive',
      title: `Failed to ${action}`,
      description: error.message || 'Please try again.',
    });
    setIsSaving(false);
  };

  const onSubmit = (data: EventFormValues) => {
    setIsSaving(true);
    const payload = {
      ...data,
      endDate: data.endDate || data.startDate,
      color: editingEvent?.color || DEFAULT_COLOR,
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
                  onSuccess: () => {
                    invalidateCalendar();
                    setDialogOpen(false);
                    setIsSaving(false);
                  },
                  onError: (error) => handleMutationError('update local event', error),
                },
              );
            },
            onError: (error) => handleMutationError('update Google Calendar event', error),
          },
        );
      } else {
        updateEvent.mutate(
          { id: editingEvent.id, data: payload },
          {
            onSuccess: () => {
              invalidateCalendar();
              setDialogOpen(false);
              setIsSaving(false);
            },
            onError: (error) => handleMutationError('update event', error),
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
                onSuccess: () => {
                  invalidateCalendar();
                  setDialogOpen(false);
                  setIsSaving(false);
                },
                onError: (error) => handleMutationError('save local event', error),
              },
            );
          },
          onError: (error) => handleMutationError('create Google Calendar event', error),
        },
      );
    } else {
      createEvent.mutate(
        { data: payload },
        {
          onSuccess: () => {
            invalidateCalendar();
            setDialogOpen(false);
            setIsSaving(false);
          },
          onError: (error) => handleMutationError('create event', error),
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
            deleteEvent.mutate(
              { id: event.id },
              {
                onSuccess: () => invalidateCalendar(),
                onError: (error) => handleMutationError('delete local event', error),
              },
            );
          },
          onError: (error) => handleMutationError('delete Google Calendar event', error),
        },
      );
    } else {
      deleteEvent.mutate(
        { id: event.id },
        {
          onSuccess: () => invalidateCalendar(),
          onError: (error) => handleMutationError('delete event', error),
        },
      );
    }
  };

  const handleSync = () => {
    const start = new Date(Date.now() - 30 * 86400000).toISOString();
    const end = new Date(Date.now() + 30 * 86400000).toISOString();
    syncGoogleCalendar.mutate(
      { data: { from: start, to: end } },
      {
        onSuccess: () => invalidateCalendar(),
        onError: (error) => handleMutationError('sync Google Calendar', error),
      },
    );
  };

  const sortedEvents = events?.slice().sort((a, b) => {
    const aStr = `${a.startDate}T${a.startTime || '00:00'}`;
    const bStr = `${b.startDate}T${b.startTime || '00:00'}`;
    return aStr.localeCompare(bStr);
  });

  return (
    <PageTransition className="space-y-8 max-w-5xl mx-auto pb-10">
      <header className="flex flex-col sm:flex-row sm:items-end justify-between gap-4">
        <div>
          <h1 className="text-3xl md:text-4xl font-display font-bold tracking-tight">Family Calendar</h1>
          <p className="text-muted-foreground mt-2 text-lg">
            Local events and synced Google Calendar in one view.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            onClick={handleSync}
            disabled={syncGoogleCalendar.isPending || !calendarConnected}
            className="shrink-0"
          >
            <RefreshCw className={cn('h-4 w-4 mr-2', syncGoogleCalendar.isPending && 'animate-spin')} />
            {calendarConnected ? 'Sync Google' : 'Google not connected'}
          </Button>
          <Button onClick={openAdd} className="shrink-0" data-testid="button-add-event">
            <Plus className="h-4 w-4 mr-2" /> Add Event
          </Button>
        </div>
      </header>

      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Button variant="outline" size="icon" onClick={() => setCurrentDate(subMonths(currentDate, 1))}>
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <h2 className="text-xl font-display font-semibold min-w-[160px] text-center">
            {format(currentDate, 'MMMM yyyy')}
          </h2>
          <Button variant="outline" size="icon" onClick={() => setCurrentDate(addMonths(currentDate, 1))}>
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {[...Array(4)].map((_, i) => (
            <div key={i} className="h-20 bg-card rounded-xl animate-pulse" />
          ))}
        </div>
      ) : sortedEvents && sortedEvents.length > 0 ? (
        <div className="space-y-3">
          {sortedEvents.map((event) => (
            <EventCard
              key={event.id}
              event={event}
              onEdit={() => openEdit(event)}
              onDelete={() => handleDelete(event)}
            />
          ))}
        </div>
      ) : (
        <div className="text-center py-20 bg-card/50 rounded-2xl border border-dashed border-border/60 backdrop-blur-sm">
          <div className="h-16 w-16 bg-primary/10 rounded-full flex items-center justify-center mx-auto mb-4">
            <CalendarIcon className="h-8 w-8 text-primary" />
          </div>
          <h2 className="text-xl font-display font-semibold mb-2">No events this month</h2>
          <p className="text-muted-foreground mb-6 max-w-md mx-auto">
            Add events manually or sync with Google Calendar.
          </p>
          <div className="flex justify-center gap-2">
            {calendarConnected && (
              <Button variant="outline" onClick={handleSync}>
                <RefreshCw className="h-4 w-4 mr-2" /> Sync Google Calendar
              </Button>
            )}
            <Button onClick={openAdd}>
              <Plus className="h-4 w-4 mr-2" /> Add Event
            </Button>
          </div>
        </div>
      )}

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
            <DialogFooter className="pt-4">
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>
                Cancel
              </Button>
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

function EventCard({
  event,
  onEdit,
  onDelete,
}: {
  event: Event;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const isGoogle = !!event.googleEventId;
  return (
    <Card className="group bg-card/40 hover:bg-card/60 border-border transition-colors">
      <CardContent className="p-4 flex items-start gap-4">
        <div
          className="h-12 w-12 rounded-xl flex flex-col items-center justify-center border border-white/10 shrink-0"
          style={{ backgroundColor: event.color || 'hsl(var(--primary))' }}
        >
          <span className="text-xs font-bold uppercase text-white/90">{format(new Date(event.startDate), 'MMM')}</span>
          <span className="text-lg font-bold leading-none text-white">{format(new Date(event.startDate), 'd')}</span>
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="font-medium truncate">{event.title}</h3>
            {isGoogle && (
              <span className="text-xs bg-blue-500/10 text-blue-400 border border-blue-500/20 px-1.5 py-0.5 rounded flex items-center gap-1">
                <Link2 className="h-3 w-3" /> Google
              </span>
            )}
          </div>
          <div className="flex flex-wrap items-center gap-x-4 gap-y-1 mt-1 text-xs text-muted-foreground">
            {event.allDay ? (
              <span className="flex items-center gap-1"><Clock className="h-3 w-3" /> All day</span>
            ) : (
              <span className="flex items-center gap-1"><Clock className="h-3 w-3" /> {event.startTime || '—'} {event.endTime ? `– ${event.endTime}` : ''}</span>
            )}
            {event.location && (
              <span className="flex items-center gap-1"><MapPin className="h-3 w-3" /> {event.location}</span>
            )}
          </div>
          {event.description && <p className="text-xs text-muted-foreground mt-2 line-clamp-2">{event.description}</p>}
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" className="h-8 w-8 opacity-0 group-hover:opacity-100 transition-opacity">
              <MoreVertical className="h-4 w-4 text-muted-foreground" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-40">
            <DropdownMenuItem onClick={onEdit}>
              <Edit2 className="h-4 w-4 mr-2" /> Edit
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onDelete} className="text-destructive focus:text-destructive focus:bg-destructive/10">
              <Trash2 className="h-4 w-4 mr-2" /> Delete
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </CardContent>
    </Card>
  );
}
