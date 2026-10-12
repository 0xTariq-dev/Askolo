import { useEffect, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { format } from 'date-fns';
import { useLocation } from 'wouter';
import { z } from 'zod';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  Check,
  ChevronLeft,
  ChevronRight,
  Clock3,
  CalendarDays,
  ListTodo,
  Plus,
  Sparkles,
  Target,
  Trash2,
} from 'lucide-react';
import {
  useGetTranscriptionPreferences,
  useUpdateTranscriptionPreferences,
  getGetTranscriptionPreferencesQueryKey,
} from '@workspace/api-client-react';
import { useLocale } from '@/contexts/locale-context';
import { useSpaceScope } from '@/contexts/space-context';
import { useAssistantState } from '@/contexts/assistant-context';
import {
  archiveV2Action,
  getV2AgendaRange,
  updateV2Action,
  updateV2ActionOccurrence,
  createV2Action,
  getV2GoalsForScope,
  type V2AgendaTask,
} from '@/lib/agenda-v2-api';
import { getMonthGridAgendaRange } from '@/lib/agenda-date-range';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { PageTransition } from '@/components/ui/page-transition';
import { useVoiceTranscription } from '@/hooks/use-voice-transcription';
import {
  VoiceCaptureButton,
  VoiceCaptureFeedback,
  VoiceTranscriptReview,
} from '@/components/voice/voice-session-ui';
import { CURRENT_VOICE_CONSENT_VERSION } from '@/lib/voice-consent';
import { getVoiceConsentErrorMessage } from '@/lib/voice-consent-errors';
import { getReviewedVoiceValue } from '@/lib/voice-flow';
import { cn } from '@/lib/utils';
import { VoiceConsentDialog } from '@/components/voice-consent-dialog';
import { AgendaCalendar } from '@/components/agenda-calendar';
import { GoalsPage } from '@/pages/goals';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

const planSchema = z.object({
  title: z.string().min(1, 'Title is required'),
  notes: z.string().max(6000).optional(),
  label: z.string().max(100).optional(),
  plannedStartTime: z.string().optional(),
  durationMinutes: z.string().optional(),
  dueOn: z.string().optional(),
  dueTime: z.string().optional(),
  kind: z.enum(['task', 'habit']),
  priority: z.enum(['none', 'high', 'normal', 'low']),
}).superRefine((values, context) => {
  const durationText = values.durationMinutes?.trim();
  if (durationText && (!/^\d+$/.test(durationText) || Number(durationText) < 1 || Number(durationText) > 1440)) {
    context.addIssue({ code: z.ZodIssueCode.custom, path: ['durationMinutes'], message: 'Enter a whole number from 1 to 1440.' });
  }
  if (Boolean(values.plannedStartTime) !== Boolean(durationText)) {
    context.addIssue({ code: z.ZodIssueCode.custom, path: ['durationMinutes'], message: 'Enter both a start time and a duration.' });
  }
  if (values.dueTime && !values.dueOn) {
    context.addIssue({ code: z.ZodIssueCode.custom, path: ['dueOn'], message: 'Add a deadline date when you specify a time.' });
  }
});
type PlanFormValues = z.infer<typeof planSchema>;

const priorityStyles = {
  none: '',
  high: 'border-s-2 border-destructive',
  normal: 'border-s-2 border-warning',
  low: 'border-s-2 border-info',
};

export function PlanPage() {
  const { formatDate, formatNumber, weekStartsOn, locale } = useLocale();
  const queryClient = useQueryClient();
  const { openWithDraft } = useAssistantState();
  const { selectedSpaceId, spaces } = useSpaceScope();
  const [, setLocation] = useLocation();
  const [selectedDate, setSelectedDate] = useState(new Date());
  const search = new URLSearchParams(window.location.search);
  const [kindFilter, setKindFilter] = useState<'all' | PlanFormValues['kind']>(
    search.get('kind') === 'habit' ? 'habit' : 'all',
  );
  const [monthDate, setMonthDate] = useState(new Date());
  const [dialogOpen, setDialogOpen] = useState(false);
  const [notes, setNotes] = useState('');
  const [isGenerating, setIsGenerating] = useState(false);
  const [generationError, setGenerationError] = useState('');
  const [agendaTasks, setAgendaTasks] = useState<V2AgendaTask[]>([]);
  const [agendaLoading, setAgendaLoading] = useState(true);
  const [agendaError, setAgendaError] = useState<string | null>(null);
  const [agendaReloadKey, setAgendaReloadKey] = useState(0);
  const [isSavingTask, setIsSavingTask] = useState(false);
  const [targetSpaceId, setTargetSpaceId] = useState('');
  const [activeAgendaTab, setActiveAgendaTab] = useState(
    search.get('tab') === 'actions' || search.get('tab') === 'tasks' ? 'actions' : 'calendar',
  );
  const [calendarEventCount, setCalendarEventCount] = useState(0);
  const [goalCount, setGoalCount] = useState<number | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    setGoalCount(null);
    getV2GoalsForScope(selectedSpaceId, controller.signal)
      .then((items) => setGoalCount(items.length))
      .catch(() => {
        if (!controller.signal.aborted) setGoalCount(-1);
      });
    return () => controller.abort();
  }, [selectedSpaceId]);
  const [consentOpen, setConsentOpen] = useState(false);
  const [consentError, setConsentError] = useState('');
  const [consentSaving, setConsentSaving] = useState(false);
  const [voiceReviewText, setVoiceReviewText] = useState('');
  const [voiceApplied, setVoiceApplied] = useState(false);
  const dateStr = format(selectedDate, 'yyyy-MM-dd');
  const { from, to } = getMonthGridAgendaRange(monthDate, weekStartsOn);
  const { data: transcriptionPreferences } = useGetTranscriptionPreferences();
  const updateTranscriptionPreferences = useUpdateTranscriptionPreferences();
  const voice = useVoiceTranscription();
  const { state: voiceState, transcript, isBusy: voiceIsBusy, start: startVoiceInput, reset: resetVoiceInput } = voice;

  const form = useForm<PlanFormValues>({
    resolver: zodResolver(planSchema),
    defaultValues: { title: '', kind: 'task', priority: 'none' },
  });

  useEffect(() => {
    const controller = new AbortController();
    setAgendaLoading(true);
    setAgendaError(null);
    getV2AgendaRange(selectedSpaceId, {
      from,
      to,
      timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
    }, controller.signal)
      .then((agenda) => setAgendaTasks(agenda.tasks))
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setAgendaTasks([]);
          setAgendaError(error instanceof Error ? error.message : 'Plan could not be loaded.');
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setAgendaLoading(false);
      });
    return () => controller.abort();
  }, [selectedSpaceId, from, to, agendaReloadKey]);
  useEffect(() => {
    if (voiceState === 'review' && transcript) {
      setVoiceReviewText(transcript);
      setVoiceApplied(false);
    }
  }, [transcript, voiceState]);

  const reloadData = () => {
    setAgendaReloadKey((value) => value + 1);
  };

  const openAdd = () => {
    setTargetSpaceId(selectedSpaceId ?? '');
    form.reset({ title: '', kind: 'task', priority: 'none' });
    setDialogOpen(true);
  };

  const taskItems = agendaTasks.map((task) => ({
    id: `${task.taskId}:${task.occurrenceOn ?? ''}`,
    taskId: task.taskId,
    workspaceId: task.workspaceId,
    occurrenceOn: task.occurrenceOn,
    date: task.occurrenceOn?.slice(0, 10) ?? task.plannedOn?.slice(0, 10) ?? task.dueOn?.slice(0, 10) ?? task.plannedStartAt?.slice(0, 10) ?? '',
    title: !selectedSpaceId && task.workspaceName ? `${task.workspaceName}: ${task.title}` : task.title,
    completed: task.status === 'done',
    kind: task.kind,
    priority: (['none', 'high', 'normal', 'low'].includes(task.priority) ? task.priority : 'none') as PlanFormValues['priority'],
    timeBlock: task.plannedStartTime && task.durationMinutes
      ? `${task.plannedStartTime.slice(0, 5)} · ${task.durationMinutes} min`
      : undefined,
  }));
  const plans = taskItems.filter((task) =>
    task.date === dateStr && (kindFilter === 'all' || task.kind === kindFilter),
  );

  const onSubmit = async (data: PlanFormValues) => {
    const workspaceId = selectedSpaceId ?? targetSpaceId;
    if (!workspaceId) {
      setAgendaError(isArabic ? 'اختر مساحةً لحفظ المهمة.' : 'Choose a Space to save this task.');
      return;
    }
    setIsSavingTask(true);
    try {
      await createV2Action(workspaceId, {
        kind: data.kind,
        title: data.title,
        details: data.kind === 'task' ? data.notes?.trim() || undefined : undefined,
        label: data.kind === 'task' ? data.label?.trim() || undefined : undefined,
        priority: data.kind === 'habit' ? 'none' : data.priority,
        plannedOn: data.kind === 'task' ? dateStr : undefined,
        plannedStartTime: data.kind === 'task' ? data.plannedStartTime || undefined : undefined,
        durationMinutes: data.kind === 'task' && data.durationMinutes
          ? Number(data.durationMinutes)
          : undefined,
        dueOn: data.kind === 'task' ? data.dueOn || undefined : undefined,
        dueTime: data.kind === 'task' ? data.dueTime || undefined : undefined,
        timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
        recurrence: data.kind === 'habit' ? {
          frequency: 'daily',
          interval: 1,
          startsOn: format(new Date(), 'yyyy-MM-dd'),
          endsOn: null,
          timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
        } : undefined,
      });
      setDialogOpen(false);
      form.reset({ title: '', kind: 'task', priority: 'none' });
      await reloadData();
    } catch (error) {
      setAgendaError(error instanceof Error ? error.message : 'Task could not be saved.');
    } finally {
      setIsSavingTask(false);
    }
  };

  const toggleComplete = async (item: (typeof taskItems)[number]) => {
    const workspaceId = item.workspaceId ?? selectedSpaceId;
    if (!workspaceId) return;
    const status = item.completed ? 'open' : 'done';
    try {
      if (item.occurrenceOn) {
        await updateV2ActionOccurrence(workspaceId, item.taskId, item.occurrenceOn.slice(0, 10), { status });
      } else {
        await updateV2Action(workspaceId, item.taskId, { status });
      }
      await reloadData();
    } catch (error) {
      setAgendaError(error instanceof Error ? error.message : 'Task could not be updated.');
    }
  };

  const handleDelete = async (item: (typeof taskItems)[number]) => {
    const workspaceId = item.workspaceId ?? selectedSpaceId;
    if (!workspaceId) return;
    try {
      if (item.occurrenceOn) {
        await updateV2ActionOccurrence(workspaceId, item.taskId, item.occurrenceOn.slice(0, 10), { status: 'cancelled' });
      } else {
        await archiveV2Action(workspaceId, item.taskId);
      }
      await reloadData();
    } catch (error) {
      setAgendaError(error instanceof Error ? error.message : 'Task could not be removed.');
    }
  };

  const generateWithAI = async () => {
    if (!notes.trim()) return;
    setIsGenerating(true);
    setGenerationError('');
    openWithDraft(
      `${isArabic ? `أنشئ خطةً لهذا اليوم (${dateStr}) من هذه الملاحظات، واعرض المهام المقترحة للمراجعة قبل حفظها:` : `Create a plan for ${dateStr} from these notes and show proposed tasks for review before saving:`}\n${notes.trim()}`,
    );
    setNotes('');
    resetVoiceInput();
    setLocation('/agent');
  };

  const handleStartVoice = async () => {
    if (!transcriptionPreferences?.consentGiven ||
      transcriptionPreferences.consentVersion !== CURRENT_VOICE_CONSENT_VERSION) {
      setConsentError('');
      setConsentOpen(true);
      return;
    }
    setVoiceReviewText('');
    setVoiceApplied(false);
    await startVoiceInput();
  };

  const saveVoiceConsent = async () => {
    setConsentSaving(true);
    setConsentError('');
    try {
      const updated = await updateTranscriptionPreferences.mutateAsync({ data: { consent: true } });
      queryClient.setQueryData(getGetTranscriptionPreferencesQueryKey(), updated);
      setConsentOpen(false);
    } catch (error) {
      setConsentError(getVoiceConsentErrorMessage(error));
    } finally {
      setConsentSaving(false);
    }
  };

  const sortedPlans = [...plans].sort((a, b) => {
    if (a.completed !== b.completed) return a.completed ? 1 : -1;
    const rank = { high: 0, normal: 1, low: 2, none: 3 };
    return rank[a.priority] - rank[b.priority];
  });
  const loading = agendaLoading;
  const hasError = Boolean(agendaError);
  const completedCount = sortedPlans.filter((item) => item.completed).length;
  const formattedSelectedDate = formatDate(selectedDate, { weekday: 'long', month: 'long', day: 'numeric' });
  const isArabic = locale === 'ar';

  return (
    <PageTransition surface={false} className="mx-auto flex w-full max-w-[1440px] flex-col gap-5">
      <header className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
        <div>
          <p className="mb-2 text-xs font-semibold uppercase tracking-[0.18em] text-primary">
            {isArabic ? 'مساحة يومك' : 'YOUR DAY, IN VIEW'}
          </p>
          <h1 className="text-3xl font-bold tracking-tight md:text-4xl">
            {isArabic ? 'الأجندة' : 'Agenda'}
          </h1>
          <p className="mt-2 text-sm text-muted-foreground md:text-base">
            {isArabic ? 'تقويم عائلتك وخطوات اليوم في مكان واحد.' : 'Your calendar and the next few steps, together.'}
          </p>
        </div>
      </header>

      <Tabs value={activeAgendaTab} onValueChange={setActiveAgendaTab} className="min-w-0">
        <TabsList
          className="grid !h-10 w-full grid-cols-3 items-center overflow-hidden rounded-[14px] border border-border/60 bg-muted/70 !p-1"
          aria-label={isArabic ? 'أقسام الأجندة' : 'Agenda sections'}
        >
          <TabsTrigger
            value="calendar"
            className="!h-7 min-w-0 gap-1 rounded-[10px] !border-0 px-1.5 text-xs text-muted-foreground data-[state=active]:bg-background data-[state=active]:text-foreground data-[state=active]:shadow-none dark:data-[state=active]:!bg-foreground/10"
          >
            <CalendarDays className="size-3.5 shrink-0" aria-hidden="true" />
            <span className="truncate">{isArabic ? 'التقويم' : 'Calendar'}</span>
            <span className="shrink-0 text-[10px] tabular-nums text-muted-foreground">{calendarEventCount}</span>
          </TabsTrigger>
          <TabsTrigger
            value="goals"
            className="!h-7 min-w-0 gap-1 rounded-[10px] !border-0 px-1.5 text-xs text-muted-foreground data-[state=active]:bg-background data-[state=active]:text-foreground data-[state=active]:shadow-none dark:data-[state=active]:!bg-foreground/10"
          >
            <Target className="size-3.5 shrink-0" aria-hidden="true" />
            <span className="truncate">{isArabic ? 'الأهداف' : 'Goals'}</span>
            <span className="shrink-0 text-[10px] tabular-nums text-muted-foreground" aria-label={goalCount === -1 ? 'Goals failed to load' : 'Goals'}>
              {goalCount === null ? '…' : goalCount < 0 ? '!' : goalCount}
            </span>
          </TabsTrigger>
          <TabsTrigger
            value="actions"
            className="!h-7 min-w-0 gap-1 rounded-[10px] !border-0 px-1.5 text-xs text-muted-foreground data-[state=active]:bg-background data-[state=active]:shadow-none dark:data-[state=active]:!bg-foreground/10"
          >
            <ListTodo className="size-3.5 shrink-0" aria-hidden="true" />
            <span className="truncate">{isArabic ? 'الإجراءات' : 'Actions'}</span>
            <span className="shrink-0 text-[10px] tabular-nums text-muted-foreground">{sortedPlans.length}</span>
          </TabsTrigger>
        </TabsList>
        <TabsContent value="calendar" className="mt-5 min-w-0">
          <AgendaCalendar onCountChange={setCalendarEventCount} />
        </TabsContent>
        <TabsContent value="goals" className="mt-5 min-w-0">
          <GoalsPage />
        </TabsContent>
        <TabsContent value="actions" className="mt-5 min-w-0">
      {hasError && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-destructive/30 bg-destructive/5 px-4 py-3" role="alert">
          <p className="text-sm text-destructive">{agendaError ?? (isArabic ? 'تعذر تحميل بعض بيانات الخطة.' : 'Some plan data could not be loaded.')}</p>
          <Button variant="outline" size="sm" onClick={reloadData}>{isArabic ? 'إعادة المحاولة' : 'Try again'}</Button>
        </div>
      )}

      <aside className="flex min-w-0 flex-col gap-4">
        <section className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border/80 bg-card px-4 py-3" aria-label={isArabic ? 'اختيار يوم المهام' : 'Choose task date'}>
          <div className="flex items-center gap-1">
            <Button variant="ghost" size="icon" aria-label={isArabic ? 'اليوم السابق' : 'Previous day'} onClick={() => setSelectedDate((date) => {
              const next = new Date(date.getFullYear(), date.getMonth(), date.getDate() - 1);
              setMonthDate(next);
              return next;
            })}>
              <ChevronLeft className="size-4 rtl:rotate-180" />
            </Button>
            <h2 className="min-w-[180px] text-center font-display text-lg font-semibold">{formattedSelectedDate}</h2>
            <Button variant="ghost" size="icon" aria-label={isArabic ? 'اليوم التالي' : 'Next day'} onClick={() => setSelectedDate((date) => {
              const next = new Date(date.getFullYear(), date.getMonth(), date.getDate() + 1);
              setMonthDate(next);
              return next;
            })}>
              <ChevronRight className="size-4 rtl:rotate-180" />
            </Button>
          </div>
          <div className="flex items-center gap-2">
            <Input aria-label={isArabic ? 'اختر تاريخ المهام' : 'Select task date'} type="date" value={dateStr} onChange={(event) => {
              if (!event.target.value) return;
              const next = new Date(`${event.target.value}T12:00:00`);
              setSelectedDate(next);
              setMonthDate(next);
            }} className="w-auto" />
            <Button variant="outline" size="sm" onClick={() => {
              const today = new Date();
              setSelectedDate(today);
              setMonthDate(today);
            }}>{isArabic ? 'اليوم' : 'Today'}</Button>
          </div>
        </section>
          <section className="min-w-0">
            <div className="flex items-start justify-between gap-3 border-b border-border/70 px-5 py-4">
              <div className="min-w-0">
                <p className="text-xs font-medium uppercase tracking-wide text-primary">{isArabic ? 'خطة اليوم' : 'DAY PLAN'}</p>
                <h2 className="mt-1 truncate font-display text-xl font-semibold">{formattedSelectedDate}</h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  {formatNumber(completedCount, { useGrouping: false })} / {formatNumber(sortedPlans.length, { useGrouping: false })} {isArabic ? 'مكتملة' : 'completed'}
                </p>
              </div>
              <Button onClick={openAdd} size="sm" className="shrink-0" data-testid="button-add-plan-item">
                <Plus className="me-1.5 size-4" />{isArabic ? 'إضافة' : 'Add'}
              </Button>
            </div>
            <div
              className="flex flex-wrap gap-2 px-4 pt-4"
              role="group"
              aria-label={isArabic ? 'تصفية المهام حسب النوع' : 'Filter agenda by task type'}
            >
              {(['all', 'task', 'habit'] as const).map((kind) => {
                const labels = isArabic
                  ? { all: 'الكل', task: 'المهام', habit: 'العادات' }
                  : { all: 'All', task: 'Tasks', habit: 'Habits' };
                return (
                  <Button
                    key={kind}
                    type="button"
                    variant={kindFilter === kind ? 'default' : 'outline'}
                    size="sm"
                    aria-pressed={kindFilter === kind}
                    onClick={() => setKindFilter(kind)}
                  >
                    {labels[kind]}
                  </Button>
                );
              })}
            </div>
            <div className="space-y-2 p-4">
              {loading ? (
                Array.from({ length: 3 }, (_, index) => <div key={index} className="h-[68px] animate-pulse rounded-xl bg-muted" />)
              ) : sortedPlans.length ? (
                sortedPlans.map((plan) => (
                  <div key={plan.id} data-testid={`plan-item-${plan.id}`} className={cn('group flex min-w-0 items-center gap-3 rounded-xl border border-border/70 bg-background/50 p-3 transition-colors hover:bg-muted/50', plan.completed && 'opacity-60')}>
                    <button
                      type="button"
                      onClick={() => toggleComplete(plan)}
                      aria-label={plan.completed ? `Mark ${plan.title} incomplete` : `Mark ${plan.title} complete`}
                      aria-pressed={plan.completed}
                      className={cn('flex size-6 shrink-0 items-center justify-center rounded-full border transition-colors', plan.completed ? 'border-primary bg-primary text-primary-foreground' : 'border-muted-foreground/40 hover:border-primary')}
                    >
                      {plan.completed && <Check className="size-3.5" />}
                    </button>
                    <div className={cn('min-w-0 flex-1 ps-2', priorityStyles[plan.priority])}>
                      <p dir="auto" className={cn('truncate text-sm font-medium', plan.completed && 'line-through text-muted-foreground')}>{plan.title}</p>
                      {plan.timeBlock && <p className="mt-1 flex items-center gap-1.5 text-xs text-muted-foreground"><Clock3 className="size-3" />{plan.timeBlock}</p>}
                    </div>
                    <Button type="button" variant="ghost" size="icon" onClick={() => handleDelete(plan)} aria-label={`Delete ${plan.title}`} className="size-9 shrink-0 text-muted-foreground hover:text-destructive">
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                ))
              ) : (
                <div className="flex flex-col items-center rounded-xl border border-dashed border-border px-5 py-8 text-center">
                  <div className="mb-3 flex size-11 items-center justify-center rounded-full bg-primary/10 text-primary"><ListTodo className="size-5" /></div>
                  <p className="font-medium">{isArabic ? 'لا توجد مهام لهذا اليوم' : 'A little room in the day'}</p>
                  <p className="mt-1 max-w-[240px] text-sm text-muted-foreground">{isArabic ? 'أضف خطوة صغيرة لتبدأ.' : 'Add a task or turn your notes into a plan.'}</p>
                  <Button variant="outline" size="sm" onClick={openAdd} className="mt-4"><Plus className="me-1.5 size-4" />{isArabic ? 'إضافة مهمة' : 'Add a task'}</Button>
                </div>
              )}
              {!!sortedPlans.length && <Button variant="ghost" className="w-full border border-dashed border-border text-muted-foreground" onClick={openAdd}><Plus className="me-2 size-4" />{isArabic ? 'إضافة عنصر آخر' : 'Add another task'}</Button>}
            </div>
          </section>

          {false && <section className="rounded-2xl border border-border bg-card p-4 shadow-sm md:p-5">
            <div className="mb-3 flex items-center gap-2">
              <div className="flex size-8 items-center justify-center rounded-lg bg-primary/10 text-primary"><Sparkles className="size-4" /></div>
              <div>
                <h2 className="font-display font-semibold">{isArabic ? 'حوّل أفكارك إلى خطة' : 'Shape your day with AI'}</h2>
                <p className="text-xs text-muted-foreground">{isArabic ? 'راجع النص قبل إنشاء المهام.' : 'Review your notes before creating tasks.'}</p>
              </div>
            </div>
            <Textarea
              aria-label="Daily plan notes"
              className="min-h-28 resize-y bg-muted/30 leading-relaxed"
              placeholder={isArabic ? 'اكتب ما يشغل بالك لهذا اليوم…' : 'Meetings, errands, loose thoughts…'}
              value={notes}
              onChange={(event) => setNotes(event.target.value)}
            />
            <div className="mt-3 space-y-2">
              <VoiceCaptureFeedback voice={voice} />
              <VoiceTranscriptReview id="plan-voice-transcript" state={voiceState} transcript={transcript} value={voiceReviewText} onChange={setVoiceReviewText} label="Editable plan transcript">
                <Button type="button" size="sm" onClick={() => {
                  if (!voiceReviewText.trim() || voiceApplied) return;
                  setNotes((current) => getReviewedVoiceValue(current, voiceReviewText, 'append') ?? current);
                  setVoiceApplied(true);
                }} disabled={!voiceReviewText.trim() || voiceApplied}>{voiceApplied ? 'Added to plan notes' : 'Add transcript to notes'}</Button>
                <Button type="button" size="sm" variant="ghost" onClick={resetVoiceInput}>Discard transcript</Button>
              </VoiceTranscriptReview>
              {generationError && <p role="alert" className="text-sm text-destructive">{generationError}</p>}
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-2">
              <VoiceCaptureButton
                voice={voice}
                onStart={handleStartVoice}
                startText="Press and hold or activate to record voice note"
                stopText="Recording voice note; release or activate to stop"
                variant="outline"
                size="icon"
                iconOnly
                title="Press and hold to record"
                disabled={voiceState === 'starting' || voiceState === 'processing' || consentSaving || isGenerating}
                data-testid="button-voice-record"
              />
              <Button onClick={() => void generateWithAI()} disabled={!notes.trim() || isGenerating || voiceIsBusy} className="flex-1">
                <Sparkles className="me-2 size-4" />{isGenerating ? (isArabic ? 'جارٍ إعداد الخطة…' : 'Building your plan…') : (isArabic ? 'إنشاء خطة' : 'Generate plan')}
              </Button>
              <Button type="button" variant="outline" disabled={!notes.trim() || isGenerating || voiceIsBusy} onClick={() => {
                openWithDraft(notes.trim());
                setLocation('/agent');
              }}>
                {isArabic ? 'اسأل الوكيل' : 'Ask Agent'}
              </Button>
            </div>
          </section>}
      </aside>
        </TabsContent>
      </Tabs>

      <VoiceConsentDialog open={consentOpen} onOpenChange={setConsentOpen} onConfirm={() => void saveVoiceConsent()} saving={consentSaving} error={consentError} />
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-[425px]">
          <DialogHeader><DialogTitle>{isArabic ? 'إضافة إلى الخطة' : 'Add to your plan'}</DialogTitle></DialogHeader>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4 py-4">
            {!selectedSpaceId && <div className="space-y-2">
              <Label htmlFor="plan-space">{isArabic ? 'المساحة' : 'Space'}</Label>
              <select id="plan-space" value={targetSpaceId} onChange={(event) => setTargetSpaceId(event.target.value)} className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm">
                <option value="">{isArabic ? 'اختر مساحة' : 'Choose a Space'}</option>
                {spaces.map((space) => <option key={space.id} value={space.id}>{space.name}</option>)}
              </select>
            </div>}
              <div className="space-y-2">
              <Label htmlFor="plan-title">{isArabic ? 'المهمة أو النية' : 'Task or intention'}</Label>
              <Input id="plan-title" placeholder={isArabic ? 'مثال: مراجعة أسبوعية' : 'For example, weekly review'} {...form.register('title')} />
              {form.formState.errors.title && <p className="text-xs text-destructive">{form.formState.errors.title.message}</p>}
            </div>
            <div className="space-y-2">
              <Label htmlFor="plan-kind">{isArabic ? 'نوع المهمة' : 'Task type'}</Label>
              <select id="plan-kind" {...form.register('kind')} className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm">
                <option value="task">{isArabic ? 'مهمة' : 'Task'}</option>
                <option value="habit">{isArabic ? 'عادة' : 'Habit'}</option>
              </select>
            </div>
            {form.watch('kind') === 'task' && <>
              <div className="space-y-2">
                <Label htmlFor="plan-notes">{isArabic ? 'ملاحظات (اختياري)' : 'Notes (optional)'}</Label>
                <Textarea id="plan-notes" maxLength={6000} rows={3} {...form.register('notes')} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="plan-label">{isArabic ? 'التصنيف (اختياري)' : 'Label (optional)'}</Label>
                <Input id="plan-label" list="task-label-presets" maxLength={100} {...form.register('label')} />
                <datalist id="task-label-presets">
                  <option value="Home" /><option value="Bills" /><option value="Work" /><option value="Family" />
                </datalist>
              </div>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="plan-start-time">{isArabic ? 'وقت البدء' : 'Start time (optional)'}</Label>
                  <Input id="plan-start-time" type="time" {...form.register('plannedStartTime')} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="plan-duration">{isArabic ? 'المدة بالدقائق' : 'Duration in minutes'}</Label>
                  <Input id="plan-duration" type="number" min={1} max={1440} step={1} {...form.register('durationMinutes')} />
                </div>
              </div>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="plan-due-on">{isArabic ? 'الموعد النهائي' : 'Deadline date (optional)'}</Label>
                  <Input id="plan-due-on" type="date" {...form.register('dueOn')} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="plan-due-time">{isArabic ? 'وقت الموعد' : 'Deadline time (optional)'}</Label>
                  <Input id="plan-due-time" type="time" {...form.register('dueTime')} />
                </div>
              </div>
            </>}
            <fieldset className="space-y-2">
              <legend className="text-sm font-medium">{isArabic ? 'الأولوية' : 'Priority'}</legend>
              <select id="plan-priority" {...form.register('priority')} disabled={form.watch('kind') === 'habit'} className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm">
                <option value="none">{isArabic ? 'بدون أولوية' : 'No priority'}</option>
                <option value="low">{isArabic ? 'منخفضة' : 'Low'}</option>
                <option value="normal">{isArabic ? 'عادية' : 'Normal'}</option>
                <option value="high">{isArabic ? 'عالية' : 'High'}</option>
              </select>
            </fieldset>
            <DialogFooter className="pt-3">
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>{isArabic ? 'إلغاء' : 'Cancel'}</Button>
              <Button type="submit" disabled={isSavingTask}>{isSavingTask ? (isArabic ? 'جارٍ الإضافة…' : 'Adding…') : (isArabic ? 'إضافة المهمة' : 'Add task')}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </PageTransition>
  );
}
