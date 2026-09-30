import { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { format } from 'date-fns';
import { useLocale } from '@/contexts/locale-context';
import { z } from 'zod';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  Sparkles,
  Plus,
  Trash2,
  CheckCircle2,
  Clock,
  LayoutList,
  Calendar as CalendarIcon,
  ListTodo,
  ShieldCheck,
} from 'lucide-react';

import {
  useListDailyPlans,
  useCreateDailyPlan,
  useUpdateDailyPlan,
  useDeleteDailyPlan,
  useGetTranscriptionPreferences,
  useUpdateTranscriptionPreferences,
  getListDailyPlansQueryKey,
  getGetDashboardSummaryQueryKey,
  getGetTranscriptionPreferencesQueryKey,
  DailyPlan,
} from '@workspace/api-client-react';
import { ApiError } from '@workspace/api-client-react';
import { getApiErrorMessage, goApi } from '@/lib/go-api';

import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter, DialogDescription } from '@/components/ui/dialog';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Calendar as DatePicker } from '@/components/ui/calendar';
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
import { useAssistantState } from '@/contexts/assistant-context';

const priorityColors = {
  high: 'border-destructive bg-destructive/10 text-destructive',
  medium: 'border-warning bg-warning/10 text-warning',
  low: 'border-info bg-info/10 text-info',
};

const planSchema = z.object({
  title: z.string().min(1, 'Title is required'),
  timeBlock: z.string().optional(),
  priority: z.enum(['high', 'medium', 'low']),
});

type PlanFormValues = z.infer<typeof planSchema>;

export function PlanPage() {
  const { formatDate } = useLocale();
  const qc = useQueryClient();
  const { openWithDraft } = useAssistantState();
  const [selectedDate, setSelectedDate] = useState<Date>(new Date());
  const [datePickerOpen, setDatePickerOpen] = useState(false);
  const dateStr = format(selectedDate, 'yyyy-MM-dd');
  const { data: plans, isLoading } = useListDailyPlans({ date: dateStr });
  const createPlan = useCreateDailyPlan();
  const updatePlan = useUpdateDailyPlan();
  const deletePlan = useDeleteDailyPlan();
  const [notes, setNotes] = useState('');
  const [isGenerating, setIsGenerating] = useState(false);
  const [generationError, setGenerationError] = useState('');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [consentOpen, setConsentOpen] = useState(false);
  const [consentError, setConsentError] = useState('');
  const [consentSaving, setConsentSaving] = useState(false);
  const [voiceReviewText, setVoiceReviewText] = useState('');
  const [voiceApplied, setVoiceApplied] = useState(false);
  const [liveMode, setLiveMode] = useState(false);
  const { data: transcriptionPreferences } = useGetTranscriptionPreferences();
  const updateTranscriptionPreferences = useUpdateTranscriptionPreferences();
  const voice = useVoiceTranscription({ realtime: liveMode });
  const {
    state: voiceState,
    transcript,
    isBusy: voiceIsBusy,
    start: startVoiceInput,
    reset: resetVoiceInput,
  } = voice;

  const form = useForm<PlanFormValues>({
    resolver: zodResolver(planSchema),
    defaultValues: { title: '', timeBlock: '', priority: 'medium' },
  });
  const generationControllerRef = useRef<AbortController | null>(null);

  useEffect(() => () => {
    generationControllerRef.current?.abort();
    generationControllerRef.current = null;
  }, []);

  useEffect(() => {
    if (voiceState === 'review' && transcript) {
      setVoiceReviewText(transcript);
      setVoiceApplied(false);
    }
  }, [transcript, voiceState]);

  const generateWithAI = async () => {
    if (!notes.trim()) return;
    generationControllerRef.current?.abort();
    const controller = new AbortController();
    generationControllerRef.current = controller;
    setIsGenerating(true);
    setGenerationError('');

    try {
      await goApi.generatePlan({ notes, date: dateStr }, controller.signal);

      setNotes('');
      setGenerationError('');
      resetVoiceInput();
      await qc.invalidateQueries({ queryKey: getListDailyPlansQueryKey() });
      await qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
    } catch (error) {
      if (controller.signal.aborted) return;
      // Keep the reviewed transcript in the editor so the user can retry safely.
      const message = getApiErrorMessage(error, 'The plan could not be generated. Try again.');
      setGenerationError(message);
      console.error('Plan generation failed', {
        code: error instanceof ApiError ? error.code : undefined,
        requestId: error instanceof ApiError ? error.requestId : undefined,
      });
    } finally {
      if (generationControllerRef.current === controller) {
        generationControllerRef.current = null;
        setIsGenerating(false);
      }
    }
  };

  const handleStartVoice = async () => {
    if (
      !transcriptionPreferences?.consentGiven ||
      transcriptionPreferences.consentVersion !== CURRENT_VOICE_CONSENT_VERSION
    ) {
      setConsentError('');
      setConsentOpen(true);
      return;
    }
    setVoiceReviewText('');
    setVoiceApplied(false);
    await startVoiceInput();
  };

  const applyPlanTranscript = () => {
    if (!voiceReviewText.trim() || voiceApplied) return;
    setNotes((current) => getReviewedVoiceValue(current, voiceReviewText, 'append') ?? current);
    setVoiceApplied(true);
  };

  const saveVoiceConsent = async () => {
    setConsentSaving(true);
    setConsentError('');
    try {
      const updated = await updateTranscriptionPreferences.mutateAsync({ data: { consent: true } });
      qc.setQueryData(getGetTranscriptionPreferencesQueryKey(), updated);
      setConsentOpen(false);
    } catch (error) {
      setConsentError(getVoiceConsentErrorMessage(error));
    } finally {
      setConsentSaving(false);
    }
  };

  const openAdd = () => {
    form.reset({ title: '', timeBlock: '', priority: 'medium' });
    setDialogOpen(true);
  };

  const onSubmit = (data: PlanFormValues) => {
    createPlan.mutate(
      { data: { ...data, date: dateStr } },
      {
        onSuccess: () => {
          void qc.invalidateQueries({ queryKey: getListDailyPlansQueryKey() });
          void qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
          setDialogOpen(false);
        },
      },
    );
  };

  const handleDelete = (id: number) => {
    deletePlan.mutate(
      { id },
      {
        onSuccess: () => {
          void qc.invalidateQueries({ queryKey: getListDailyPlansQueryKey() });
          void qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
        },
      },
    );
  };

  const toggleComplete = (item: DailyPlan) => {
    qc.setQueryData([...getListDailyPlansQueryKey(), dateStr], (old: DailyPlan[] | undefined) => {
      if (!old) return old;
      return old.map((plan) => (plan.id === item.id ? { ...plan, completed: !item.completed } : plan));
    });

    updatePlan.mutate(
      { id: item.id, data: { completed: !item.completed } },
      {
        onSuccess: () => {
          void qc.invalidateQueries({ queryKey: getListDailyPlansQueryKey() });
          void qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
        },
      },
    );
  };

  const sortedPlans = plans
    ? [...plans].sort((a, b) => {
        if (a.completed !== b.completed) return a.completed ? 1 : -1;
        const priorityScore = { high: 0, medium: 1, low: 2 };
        return priorityScore[a.priority] - priorityScore[b.priority];
      })
    : [];
  const canGenerate = Boolean(notes.trim()) && !isGenerating && !voiceIsBusy;

  return (
    <PageTransition surface={false} className="h-full flex flex-col max-w-7xl mx-auto">
      <header className="shrink-0 mb-6 flex flex-col sm:flex-row sm:items-end justify-between gap-4">
        <div>
          <h1 className="text-3xl md:text-4xl font-display font-bold tracking-tight flex items-center gap-3">
            <LayoutList className="h-8 w-8 text-primary" />
            Daily Planner
          </h1>
          <p className="text-muted-foreground mt-2 text-lg">Brain dump your thoughts, and let AI structure your day.</p>
        </div>
      </header>

      <div className="flex-1 min-h-0 pb-6">
        <section className="flex flex-col overflow-hidden">
          <div className="flex shrink-0 justify-end pb-3">
            <Button variant="ghost" size="icon" onClick={openAdd} className="h-8 w-8 text-muted-foreground hover:text-foreground" aria-label="Add plan item">
              <Plus className="h-4 w-4" />
            </Button>
          </div>
          <div className="max-h-[min(42vh,34rem)] overflow-y-auto p-4 space-y-3">
            {isLoading ? (
              <div className="space-y-3">
                {[...Array(5)].map((_, index) => <div key={index} className="h-16 w-full bg-muted animate-pulse rounded-xl" />)}
              </div>
            ) : isGenerating ? (
              <div className="space-y-3">
                {[...Array(3)].map((_, index) => <div key={index} className="h-16 w-full bg-primary/5 border border-primary/10 animate-pulse rounded-xl" />)}
              </div>
            ) : sortedPlans.length > 0 ? (
              sortedPlans.map((plan) => (
                <div
                  key={plan.id}
                  className={cn(
                    'group flex items-center justify-between p-3.5 rounded-xl border transition-all duration-200',
                    plan.completed
                      ? 'bg-muted border-border opacity-60'
                      : 'bg-card hover:bg-accent border-border hover:border-primary/30 shadow-sm',
                  )}
                >
                  <div className="flex items-center gap-4 flex-1 overflow-hidden">
                    <button
                      type="button"
                      onClick={() => toggleComplete(plan)}
                      aria-label={plan.completed ? `Mark ${plan.title} incomplete` : `Mark ${plan.title} complete`}
                      aria-pressed={plan.completed}
                      className={cn(
                        'shrink-0 h-6 w-6 rounded-full border flex items-center justify-center transition-colors',
                        plan.completed
                          ? 'bg-primary border-primary text-primary-foreground'
                          : 'border-muted-foreground/40 hover:border-primary/50',
                      )}
                    >
                      {plan.completed && <CheckCircle2 className="h-4 w-4" />}
                    </button>
                    <div className={cn('w-1 h-8 rounded-full shrink-0', priorityColors[plan.priority].split(' ')[0], priorityColors[plan.priority].split(' ')[1])} />
                    <div className="flex-1 min-w-0">
                      <p className={cn('font-medium truncate transition-colors', plan.completed ? 'line-through text-muted-foreground' : 'text-foreground')}>
                        {plan.title}
                      </p>
                      {plan.timeBlock && (
                        <div className="flex items-center gap-1.5 mt-1 text-xs text-muted-foreground">
                          <Clock className="h-3 w-3" />
                          <span>{plan.timeBlock}</span>
                        </div>
                      )}
                    </div>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => handleDelete(plan.id)}
                    aria-label={`Delete ${plan.title}`}
                    className="shrink-0 h-8 w-8 opacity-0 group-hover:opacity-100 focus-visible:opacity-100 transition-opacity text-muted-foreground hover:text-destructive hover:bg-destructive/10"
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              ))
            ) : (
              <div className="h-full flex flex-col items-center justify-center text-center p-6 text-muted-foreground">
                <div className="h-16 w-16 bg-muted rounded-full flex items-center justify-center mb-4 border border-border">
                  <ListTodo className="h-8 w-8 opacity-50" />
                </div>
                <p>No plan for this day yet.</p>
                <p className="text-sm mt-1 max-w-[250px]">Use the AI generator on the left or add items manually.</p>
                <Button variant="outline" size="sm" onClick={openAdd} className="mt-6">
                  <Plus className="h-4 w-4 mr-2" /> Add manually
                </Button>
              </div>
            )}
            {sortedPlans.length > 0 && !isLoading && !isGenerating && (
              <Button variant="ghost" className="w-full mt-4 text-muted-foreground border border-dashed border-border hover:border-primary/30 hover:bg-accent" onClick={openAdd}>
                <Plus className="h-4 w-4 mr-2" /> Add item
              </Button>
            )}
          </div>
          <div className="border-t border-border/50" />
          <div className="p-4 sm:p-6">
            <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <h2 className="text-lg font-display flex items-center gap-2">
                <Sparkles className="h-5 w-5 text-primary" />
                What&apos;s on your mind?
              </h2>
              <Popover open={datePickerOpen} onOpenChange={setDatePickerOpen}>
                <PopoverTrigger asChild>
                  <Button type="button" variant="outline" size="sm" className="w-full justify-start sm:w-auto">
                    <CalendarIcon className="me-2 h-4 w-4" />
                    Plan for {formatDate(selectedDate, { month: 'short', day: 'numeric', year: 'numeric' })}
                  </Button>
                </PopoverTrigger>
                <PopoverContent align="end" className="w-auto p-0">
                  <DatePicker
                    mode="single"
                    autoFocus
                    selected={selectedDate}
                    onSelect={(date) => {
                      if (date) {
                        setSelectedDate(date);
                        setDatePickerOpen(false);
                      }
                    }}
                  />
                </PopoverContent>
              </Popover>
            </div>
            <Textarea
              aria-label="Daily plan notes"
              className="min-h-[180px] resize-y bg-muted/40 border-border focus-visible:ring-1 focus-visible:ring-primary/50 text-lg leading-relaxed placeholder:text-muted-foreground/40 p-4"
              placeholder={`Write your intentions, tasks, meetings, anything on your mind...\n\nExample:\n"I have a marketing sync at 10am, need to buy groceries for dinner, finish the Q3 report by EOD, and squeeze in a 30min run."`}
              value={notes}
              onChange={(event) => setNotes(event.target.value)}
            />

            <div className="mt-3 space-y-3">
              <VoiceCaptureFeedback voice={voice} />
              <VoiceTranscriptReview
                id="plan-voice-transcript"
                state={voiceState}
                transcript={transcript}
                value={voiceReviewText}
                onChange={setVoiceReviewText}
                label="Editable plan transcript"
              >
                <Button type="button" size="sm" onClick={applyPlanTranscript} disabled={!voiceReviewText.trim() || voiceApplied}>
                  {voiceApplied ? 'Added to plan notes' : 'Add transcript to plan notes'}
                </Button>
                <Button type="button" size="sm" variant="ghost" onClick={resetVoiceInput}>
                  Discard transcript
                </Button>
              </VoiceTranscriptReview>
              {generationError && <p role="alert" className="text-sm text-destructive">{generationError}</p>}
            </div>

            <div className="pt-4 flex flex-col gap-3">
              <div className="flex items-center justify-between gap-3 text-xs text-muted-foreground">
                <span>Voice is optional. Review the transcript before generating your plan.</span>
                <Button type="button" variant="ghost" size="sm" onClick={() => setLiveMode((value) => !value)} disabled={voiceIsBusy || isGenerating} aria-pressed={liveMode} data-testid="button-toggle-live-voice">
                  {liveMode ? 'Use recorded mode' : 'Use live mode'}
                </Button>
              </div>
              <div className="flex items-center justify-end gap-2">
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
                <Button
                  onClick={generateWithAI}
                  disabled={!canGenerate}
                  className="bg-primary/20 hover:bg-primary/30 text-primary border border-primary/30"
                >
                  {isGenerating ? 'Thinking...' : 'Generate Plan with AI'}
                  <Sparkles className="h-4 w-4 ml-2" />
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  disabled={!notes.trim() || isGenerating || voiceIsBusy}
                  onClick={() => openWithDraft(notes.trim())}
                >
                  Ask Assistant about these notes
                </Button>
              </div>
              <p className="text-[11px] text-muted-foreground" role="note">
                Press and hold to record. Audio is sent to AssemblyAI only after the browser validates the recording.
              </p>
            </div>
          </div>
        </section>
      </div>

      <Dialog open={consentOpen} onOpenChange={setConsentOpen}>
        <DialogContent className="sm:max-w-[520px]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <ShieldCheck className="h-5 w-5 text-primary" />
              Before you use voice input
            </DialogTitle>
            <DialogDescription>
              Please review how Askolo handles voice notes and AI processing.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3 text-sm text-muted-foreground">
            <p>
              Voice audio is sent through Askolo’s secure server connection to AssemblyAI for live or recorded transcription. Askolo does not store audio or live transcripts. AssemblyAI processes voice data under its own retention and model-improvement settings. For recorded transcription, Askolo requests deletion of the provider transcript and reports whether deletion is confirmed; the provider may retain data under its settings.
            </p>
            <p>
              We request automatic redaction of detected personal information before AssemblyAI returns the transcript. Redaction can miss details, so review every word before using the transcript to build your plan.
            </p>
            <p>
              Live transcription sessions end after 180 seconds at most. By continuing, you consent to Askolo processing voice audio through AssemblyAI for transcription.
            </p>
            {consentError && <p role="alert" className="text-destructive">{consentError}</p>}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setConsentOpen(false)} disabled={consentSaving}>
              Not now
            </Button>
            <Button type="button" onClick={() => void saveVoiceConsent()} disabled={consentSaving}>
              {consentSaving ? 'Saving…' : 'I understand and continue'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-[425px]">
          <DialogHeader><DialogTitle>Add to Plan</DialogTitle></DialogHeader>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="title">Task or Intent</Label>
              <Input id="title" placeholder="e.g. Weekly review meeting" {...form.register('title')} />
              {form.formState.errors.title && <p className="text-xs text-destructive">{form.formState.errors.title.message}</p>}
            </div>
            <div className="space-y-2">
              <Label htmlFor="timeBlock">Time Block (optional)</Label>
              <Input id="timeBlock" placeholder="e.g. 10:00 AM - 11:00 AM" {...form.register('timeBlock')} />
            </div>
            <div className="space-y-2">
              <Label>Priority</Label>
              <div className="flex gap-4">
                {(['high', 'medium', 'low'] as const).map((priority) => (
                  <label key={priority} className="flex items-center gap-2 text-sm cursor-pointer">
                    <input type="radio" value={priority} {...form.register('priority')} className="accent-primary" />
                    <span className={cn('font-medium', priority === 'high' ? 'text-destructive' : priority === 'medium' ? 'text-warning' : 'text-info')}>
                      {priority[0].toUpperCase() + priority.slice(1)}
                    </span>
                  </label>
                ))}
              </div>
            </div>
            <DialogFooter className="pt-4">
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={form.formState.isSubmitting}>{form.formState.isSubmitting ? 'Adding...' : 'Add Item'}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </PageTransition>
  );
}
