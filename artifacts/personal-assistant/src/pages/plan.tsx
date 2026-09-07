import { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { format, addDays, subDays } from 'date-fns';
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
  AlertCircle,
  Calendar,
  ListTodo,
  Mic,
  MicOff,
  Pause,
  Square,
  RotateCcw,
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

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Textarea } from '@/components/ui/textarea';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter, DialogDescription } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { PageTransition } from '@/components/ui/page-transition';
import { useVoiceTranscription } from '@/hooks/use-voice-transcription';
import { cn } from '@/lib/utils';

const priorityColors = {
  high: 'border-rose-500 bg-rose-500/10 text-rose-500',
  medium: 'border-amber-500 bg-amber-500/10 text-amber-500',
  low: 'border-blue-500 bg-blue-500/10 text-blue-500',
};

const planSchema = z.object({
  title: z.string().min(1, 'Title is required'),
  timeBlock: z.string().optional(),
  priority: z.enum(['high', 'medium', 'low']),
});

type PlanFormValues = z.infer<typeof planSchema>;

function appendVoiceText(base: string, spoken: string): string {
  return [base.trim(), spoken.trim()].filter(Boolean).join(' ');
}

function formatRecordingTime(seconds: number): string {
  const minutes = Math.floor(seconds / 60).toString().padStart(2, '0');
  const remainder = (seconds % 60).toString().padStart(2, '0');
  return `${minutes}:${remainder}`;
}

export function PlanPage() {
  const qc = useQueryClient();
  const [selectedDate, setSelectedDate] = useState<Date>(new Date());
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
  const [isPlaying, setIsPlaying] = useState(false);
  const voiceBaseNotesRef = useRef('');
  const audioRef = useRef<HTMLAudioElement>(null);
  const [recordingUrl, setRecordingUrl] = useState('');
  const { data: transcriptionPreferences } = useGetTranscriptionPreferences();
  const updateTranscriptionPreferences = useUpdateTranscriptionPreferences();
  const {
    state: voiceState,
    mode: voiceMode,
    status: voiceStatus,
    error: voiceError,
    transcript,
    liveText,
    reviewSignals: voiceReviewSignals,
    recordingSeconds,
    recording,
    isBusy: voiceIsBusy,
    isListening,
    start: startVoiceInput,
    stop: stopListening,
    cancel: cancelVoiceInput,
    reset: resetVoiceInput,
    retry: retryVoiceRecording,
    clearRecording,
  } = useVoiceTranscription();

  useEffect(() => {
    if (!recording) {
      setRecordingUrl('');
      setIsPlaying(false);
      return;
    }
    const url = URL.createObjectURL(recording.blob);
    setRecordingUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [recording]);

  const form = useForm<PlanFormValues>({
    resolver: zodResolver(planSchema),
    defaultValues: { title: '', timeBlock: '', priority: 'medium' },
  });

  useEffect(() => {
    if (voiceState === 'listening' && liveText) {
      setNotes(appendVoiceText(voiceBaseNotesRef.current, liveText));
    }
    if (voiceState === 'review' && transcript) {
      setNotes(appendVoiceText(voiceBaseNotesRef.current, transcript));
    }
  }, [liveText, transcript, voiceState]);

  const generateWithAI = async () => {
    if (!notes.trim()) return;
    setIsGenerating(true);
    setGenerationError('');

    try {
      const res = await fetch('/api/ai/generate-plan', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ notes, date: dateStr }),
      });

      if (!res.ok) {
        const payload = await res.json().catch(() => null);
        throw new Error(payload?.error || 'The plan could not be generated. Try again.');
      }

      setNotes('');
      setGenerationError('');
      resetVoiceInput();
      await qc.invalidateQueries({ queryKey: getListDailyPlansQueryKey() });
      await qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
    } catch (error) {
      // Keep the reviewed transcript in the editor so the user can retry safely.
      const message = error instanceof Error ? error.message : 'The plan could not be generated.';
      setGenerationError(message);
      console.error('Plan generation failed', { message });
    } finally {
      setIsGenerating(false);
    }
  };

  const handleStartVoice = async () => {
    if (!transcriptionPreferences?.consentGiven) {
      setConsentError('');
      setConsentOpen(true);
      return;
    }
    voiceBaseNotesRef.current = notes;
    await startVoiceInput();
  };

  const saveVoiceConsent = async () => {
    setConsentSaving(true);
    setConsentError('');
    try {
      const updated = await updateTranscriptionPreferences.mutateAsync({ data: { consent: true } });
      qc.setQueryData(getGetTranscriptionPreferencesQueryKey(), updated);
      setConsentOpen(false);
    } catch {
      setConsentError('Consent could not be saved. Please try again.');
    } finally {
      setConsentSaving(false);
    }
  };

  const stopPlayback = () => {
    audioRef.current?.pause();
    if (audioRef.current) audioRef.current.currentTime = 0;
    setIsPlaying(false);
  };

  const togglePlayback = async () => {
    if (!audioRef.current) return;
    if (audioRef.current.paused) {
      await audioRef.current.play();
      setIsPlaying(true);
    } else {
      audioRef.current.pause();
      setIsPlaying(false);
    }
  };

  const deleteRecording = () => {
    stopPlayback();
    clearRecording();
  };

  const handleCancelVoice = () => {
    const originalNotes = voiceBaseNotesRef.current;
    cancelVoiceInput();
    setNotes(originalNotes);
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
    <PageTransition className="h-full flex flex-col max-w-7xl mx-auto">
      <header className="shrink-0 mb-6 flex flex-col sm:flex-row sm:items-end justify-between gap-4">
        <div>
          <h1 className="text-3xl md:text-4xl font-display font-bold tracking-tight flex items-center gap-3">
            <LayoutList className="h-8 w-8 text-primary" />
            Daily Planner
          </h1>
          <p className="text-muted-foreground mt-2 text-lg">Brain dump your thoughts, and let AI structure your day.</p>
        </div>
        <div className="flex bg-card/50 p-1 rounded-xl border border-border/50 backdrop-blur-sm self-start sm:self-auto">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setSelectedDate(subDays(new Date(), 1))}
            className={cn(
              'rounded-lg px-4 h-9',
              dateStr === format(subDays(new Date(), 1), 'yyyy-MM-dd') &&
                'bg-primary text-primary-foreground hover:bg-primary hover:text-primary-foreground shadow-sm',
            )}
          >
            Yesterday
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setSelectedDate(new Date())}
            className={cn(
              'rounded-lg px-4 h-9',
              dateStr === format(new Date(), 'yyyy-MM-dd') &&
                'bg-primary text-primary-foreground hover:bg-primary hover:text-primary-foreground shadow-sm',
            )}
          >
            Today
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setSelectedDate(addDays(new Date(), 1))}
            className={cn(
              'rounded-lg px-4 h-9',
              dateStr === format(addDays(new Date(), 1), 'yyyy-MM-dd') &&
                'bg-primary text-primary-foreground hover:bg-primary hover:text-primary-foreground shadow-sm',
            )}
          >
            Tomorrow
          </Button>
          <div className="w-px bg-border/50 mx-1 my-1" />
          <div className="relative">
            <input
              type="date"
              aria-label="Choose plan date"
              className="opacity-0 absolute inset-0 w-full h-full cursor-pointer z-10"
              value={dateStr}
              onChange={(event) => {
                if (event.target.value) setSelectedDate(new Date(`${event.target.value}T12:00:00`));
              }}
            />
            <Button variant="ghost" size="icon" className="h-9 w-9 rounded-lg text-muted-foreground" aria-label="Choose plan date">
              <Calendar className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </header>

      <div className="flex-1 min-h-0 grid grid-cols-1 lg:grid-cols-2 gap-6 pb-6">
        <Card className="flex flex-col border-border bg-card/30 backdrop-blur-sm h-full overflow-hidden shadow-sm">
          <CardHeader className="pb-3 border-b border-border/50 shrink-0 bg-card/50">
            <CardTitle className="text-lg font-display flex items-center gap-2">
              <Sparkles className="h-5 w-5 text-primary" />
              What&apos;s on your mind?
            </CardTitle>
          </CardHeader>
          <CardContent className="flex-1 flex flex-col p-4 min-h-0">
            <Textarea
              aria-label="Daily plan notes"
              className="flex-1 resize-none bg-black/20 border-white/5 focus-visible:ring-1 focus-visible:ring-primary/50 text-lg leading-relaxed placeholder:text-muted-foreground/40 p-4"
              placeholder={`Write your intentions, tasks, meetings, anything on your mind...\n\nExample:\n"I have a marketing sync at 10am, need to buy groceries for dinner, finish the Q3 report by EOD, and squeeze in a 30min run."`}
              value={notes}
              onChange={(event) => setNotes(event.target.value)}
            />

            {(voiceState !== 'idle' || voiceStatus || voiceError || generationError) && (
              <div className="mt-3 space-y-1 text-xs" aria-live="polite" aria-atomic="true">
                {voiceState === 'listening' && (
                  <p className="flex items-center gap-2 text-rose-500">
                    <span className="h-2 w-2 rounded-full bg-rose-500 animate-pulse" aria-hidden="true" />
                    {voiceMode === 'recorded' ? 'AssemblyAI recorded transcription' : 'Live US transcription'} · {formatRecordingTime(recordingSeconds)}
                  </p>
                )}
                {voiceStatus && <p className="text-muted-foreground">{voiceStatus}</p>}
                {voiceState === 'review' && transcript && (
                  <p className="text-emerald-600 dark:text-emerald-400">Transcript ready. Review the text above before generating.</p>
                )}
                {voiceReviewSignals.length > 0 && (
                  <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-2 text-amber-700 dark:text-amber-300">
                    <p className="font-medium">Please verify low-confidence details:</p>
                    <ul className="mt-1 list-disc pl-4">
                      {voiceReviewSignals.map((signal) => (
                        <li key={`${signal.startMs ?? 'unknown'}-${signal.text}`}>
                          “{signal.text}” ({Math.round(signal.confidence * 100)}% confidence)
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
                {voiceError && (
                  <p role="alert" className="text-destructive">
                    {voiceError}
                  </p>
                )}
                {generationError && (
                  <p role="alert" className="text-destructive">
                    {generationError}
                  </p>
                )}
              </div>
            )}

            <div className="pt-4 shrink-0 flex flex-col gap-3">
              <div className="flex items-center justify-between gap-3">
                <span className="text-xs text-muted-foreground flex items-center gap-1.5">
                  <AlertCircle className="h-3.5 w-3.5 shrink-0" />
                  AI will extract tasks, estimate priorities, and suggest time blocks.
                </span>
                <div className="flex items-center gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    onPointerDown={(event) => {
                      event.currentTarget.setPointerCapture?.(event.pointerId);
                      void handleStartVoice();
                    }}
                    onPointerUp={stopListening}
                    onPointerCancel={stopListening}
                    onKeyDown={(event) => {
                      if ((event.key === ' ' || event.key === 'Enter') && !event.repeat) {
                        event.preventDefault();
                        void handleStartVoice();
                      }
                    }}
                    onKeyUp={(event) => {
                      if (event.key === ' ' || event.key === 'Enter') {
                        event.preventDefault();
                        stopListening();
                      }
                    }}
                    onClick={(event) => event.preventDefault()}
                    disabled={voiceState === 'processing' || consentSaving}
                    aria-label={isListening ? 'Release to stop voice recording' : 'Press and hold to record voice note'}
                    aria-pressed={isListening}
                    title="Press and hold to record"
                    className={cn(
                      'border-border',
                      isListening && 'bg-rose-500/10 text-rose-500 border-rose-500/30 animate-pulse',
                    )}
                    data-testid="button-voice-record"
                  >
                    {isListening ? <MicOff className="h-4 w-4" /> : <Mic className="h-4 w-4" />}
                  </Button>
                  <Button
                    onClick={generateWithAI}
                    disabled={!canGenerate}
                    className="bg-primary/20 hover:bg-primary/30 text-primary border border-primary/30"
                  >
                    {isGenerating ? 'Thinking...' : 'Generate Plan with AI'}
                    <Sparkles className="h-4 w-4 ml-2" />
                  </Button>
                </div>
              </div>
              <p className="text-[11px] text-muted-foreground" role="note">
                Press and hold to record. Audio is sent to AssemblyAI only for transcription, then deleted. The returned text is reviewed before it reaches your plan.
              </p>
              {recording && recordingUrl && (
                <div className="rounded-lg border border-primary/20 bg-primary/5 p-3 space-y-3" aria-label="Completed voice recording">
                  <audio
                    ref={audioRef}
                    src={recordingUrl}
                    onEnded={() => setIsPlaying(false)}
                    className="hidden"
                  />
                  <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                    <div className="min-w-0">
                      <p className="text-sm font-medium">Voice note ready</p>
                      <p className="text-xs text-muted-foreground">
                        {formatRecordingTime(Math.round(recording.durationMs / 1000))} · held in memory only
                      </p>
                    </div>
                    <div className="flex items-center gap-1.5">
                      <Button type="button" size="icon" variant="outline" onClick={() => void togglePlayback()} aria-label={isPlaying ? 'Pause recording' : 'Play recording'}>
                        {isPlaying ? <Pause className="h-4 w-4" /> : <Mic className="h-4 w-4" />}
                      </Button>
                      <Button type="button" size="icon" variant="outline" onClick={stopPlayback} aria-label="Stop playback">
                        <Square className="h-3.5 w-3.5 fill-current" />
                      </Button>
                      <Button type="button" size="icon" variant="outline" onClick={() => void retryVoiceRecording()} disabled={voiceState === 'processing'} aria-label="Retry transcription">
                        <RotateCcw className="h-4 w-4" />
                      </Button>
                      <Button type="button" size="icon" variant="ghost" onClick={deleteRecording} aria-label="Delete recording now" className="text-destructive hover:text-destructive">
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                </div>
              )}
              {voiceIsBusy && (
                <Button type="button" variant="ghost" size="sm" onClick={handleCancelVoice} className="self-end text-muted-foreground">
                  Cancel voice input
                </Button>
              )}
            </div>
          </CardContent>
        </Card>

        <Card className="flex flex-col border-border bg-card/50 backdrop-blur-sm h-full overflow-hidden shadow-sm">
          <CardHeader className="pb-3 border-b border-border/50 shrink-0 flex flex-row items-center justify-between bg-card/50">
            <CardTitle className="text-lg font-display flex items-center gap-2">
              <ListTodo className="h-5 w-5 text-foreground" />
              Structured Plan
              <span className="ml-2 text-xs font-medium text-muted-foreground bg-black/20 px-2 py-0.5 rounded-md border border-white/5">
                {format(selectedDate, 'MMM d')}
              </span>
            </CardTitle>
            <Button variant="ghost" size="icon" onClick={openAdd} className="h-8 w-8 text-muted-foreground hover:text-foreground" aria-label="Add plan item">
              <Plus className="h-4 w-4" />
            </Button>
          </CardHeader>
          <div className="flex-1 overflow-y-auto p-4 space-y-3">
            {isLoading ? (
              <div className="space-y-3">
                {[...Array(5)].map((_, index) => <div key={index} className="h-16 w-full bg-white/5 animate-pulse rounded-xl" />)}
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
                      ? 'bg-black/20 border-white/5 opacity-60'
                      : 'bg-card hover:bg-card/80 border-white/10 hover:border-white/20 shadow-sm',
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
                <div className="h-16 w-16 bg-black/20 rounded-full flex items-center justify-center mb-4 border border-white/5">
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
              <Button variant="ghost" className="w-full mt-4 text-muted-foreground border border-dashed border-white/10 hover:border-white/20 hover:bg-white/5" onClick={openAdd}>
                <Plus className="h-4 w-4 mr-2" /> Add item
              </Button>
            )}
          </div>
        </Card>
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
              Your recording is sent to AssemblyAI for transcription and is deleted after processing. Neither Askolo nor AssemblyAI keeps the recording or uses it to train models.
            </p>
            <p>
              PII is redacted from AI interactions across this flow before the result is returned. You will always review the transcript before it is used to build your plan.
            </p>
            <p>
              By continuing, you consent to in-app AI processing and AssemblyAI transcription for this voice note.
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
                    <input type="radio" value={priority} {...form.register('priority')} className={`accent-${priority === 'high' ? 'rose' : priority === 'medium' ? 'amber' : 'blue'}-500`} />
                    <span className={cn('font-medium', priority === 'high' ? 'text-rose-500' : priority === 'medium' ? 'text-amber-500' : 'text-blue-500')}>
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