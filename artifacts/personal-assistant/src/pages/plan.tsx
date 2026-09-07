import { useState, useRef, useEffect } from 'react';
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
  MicOff
} from 'lucide-react';

import {
  useListDailyPlans,
  useCreateDailyPlan,
  useUpdateDailyPlan,
  useDeleteDailyPlan,
  useTranscribeAudio,
  getListDailyPlansQueryKey,
  getGetDashboardSummaryQueryKey,
  DailyPlan
} from '@workspace/api-client-react';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Textarea } from '@/components/ui/textarea';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { PageTransition } from '@/components/ui/page-transition';
import { cn } from '@/lib/utils';

const priorityColors = {
  high: 'border-rose-500 bg-rose-500/10 text-rose-500',
  medium: 'border-amber-500 bg-amber-500/10 text-amber-500',
  low: 'border-blue-500 bg-blue-500/10 text-blue-500',
};

const planSchema = z.object({
  title: z.string().min(1, "Title is required"),
  timeBlock: z.string().optional(),
  priority: z.enum(['high', 'medium', 'low']),
});

type PlanFormValues = z.infer<typeof planSchema>;

const MAX_RECORDING_MS = 2 * 60 * 1000;
const MAX_AUDIO_BYTES = 8 * 1024 * 1024;

type VoiceState = 'idle' | 'starting' | 'listening' | 'processing' | 'review' | 'error';
type VoiceMode = 'live' | 'recorded';

function normalizeSpeech(value: string): string {
  return value.replace(/\s+/g, ' ').trim();
}

function removeRepeatedTail(value: string): string {
  const words = normalizeSpeech(value).split(' ').filter(Boolean);
  const maxRepeatedWords = Math.min(12, Math.floor(words.length / 2));

  for (let size = maxRepeatedWords; size >= 5; size -= 1) {
    const previous = words.slice(words.length - size * 2, words.length - size);
    const last = words.slice(words.length - size);
    if (previous.length === size && previous.join(' ') === last.join(' ')) {
      return words.slice(0, words.length - size).join(' ');
    }
  }

  return words.join(' ');
}

function mergeVoiceText(base: string, spoken: string): string {
  const cleanBase = normalizeSpeech(base);
  const cleanSpoken = removeRepeatedTail(spoken);
  if (!cleanSpoken) return cleanBase;
  return cleanBase ? `${cleanBase} ${cleanSpoken}` : cleanSpoken;
}

function formatRecordingTime(seconds: number): string {
  const minutes = Math.floor(seconds / 60).toString().padStart(2, '0');
  const remainder = (seconds % 60).toString().padStart(2, '0');
  return `${minutes}:${remainder}`;
}

function getRecorderMimeType(): string | null {
  if (typeof MediaRecorder === 'undefined') return null;
  const supportedTypes = [
    'audio/webm;codecs=opus',
    'audio/webm',
    'audio/mp4',
    'audio/ogg;codecs=opus',
    'audio/mpeg',
  ];
  return supportedTypes.find((type) => MediaRecorder.isTypeSupported(type)) ?? null;
}

async function blobToBase64(blob: Blob): Promise<string> {
  const buffer = await blob.arrayBuffer();
  const bytes = new Uint8Array(buffer);
  let binary = '';
  const chunkSize = 0x8000;
  for (let index = 0; index < bytes.length; index += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(index, index + chunkSize));
  }
  return btoa(binary);
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
  const [voiceState, setVoiceState] = useState<VoiceState>('idle');
  const [voiceMode, setVoiceMode] = useState<VoiceMode | null>(null);
  const [voiceStatus, setVoiceStatus] = useState('');
  const [voiceError, setVoiceError] = useState('');
  const [recordingSeconds, setRecordingSeconds] = useState(0);
  const [transcript, setTranscript] = useState('');
  const recognitionRef = useRef<any>(null);
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const mediaStreamRef = useRef<MediaStream | null>(null);
  const audioChunksRef = useRef<Blob[]>([]);
  const liveSegmentsRef = useRef<Array<{ text: string; isFinal: boolean }>>([]);
  const voiceBaseNotesRef = useRef('');
  const voiceSessionRef = useRef(0);
  const cancelRequestedRef = useRef(false);
  const stopRequestedRef = useRef(false);
  const durationStopRequestedRef = useRef(false);
  const transcribeAudio = useTranscribeAudio();

  const [dialogOpen, setDialogOpen] = useState(false);

  const form = useForm<PlanFormValues>({
    resolver: zodResolver(planSchema),
    defaultValues: {
      title: '',
      timeBlock: '',
      priority: 'medium',
    }
  });

  const generateWithAI = async () => {
    if (!notes.trim()) return;
    setIsGenerating(true);
    
    try {
      const res = await fetch('/api/ai/generate-plan', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ notes, date: dateStr })
      });
      
      if (res.ok) {
        setNotes('');
        setTranscript('');
        setVoiceState('idle');
        setVoiceMode(null);
        setVoiceStatus('');
        setVoiceError('');
        // Refetch the plan for the currently selected date specifically
        await qc.invalidateQueries({ queryKey: getListDailyPlansQueryKey() });
        await qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
      } else {
        const payload = await res.json().catch(() => null);
        setVoiceError(payload?.error || 'The plan could not be generated. Try again.');
      }
    } catch {
      setVoiceError('The plan could not be generated. Check your connection and try again.');
    } finally {
      setIsGenerating(false);
    }
  };

  const cleanupMediaStream = () => {
    mediaStreamRef.current?.getTracks().forEach((track) => track.stop());
    mediaStreamRef.current = null;
    mediaRecorderRef.current = null;
    audioChunksRef.current = [];
  };

  const refreshLiveTranscript = () => {
    const finalText = removeRepeatedTail(
      liveSegmentsRef.current.filter((segment) => segment.isFinal).map((segment) => segment.text).join(' '),
    );
    const interimText = normalizeSpeech(
      liveSegmentsRef.current.filter((segment) => !segment.isFinal).map((segment) => segment.text).join(' '),
    );
    setTranscript(finalText);
    setNotes(mergeVoiceText(voiceBaseNotesRef.current, [finalText, interimText].filter(Boolean).join(' ')));
  };

  const finishLiveReview = (sessionId: number) => {
    if (voiceSessionRef.current !== sessionId || cancelRequestedRef.current) return;
    const finalText = removeRepeatedTail(
      liveSegmentsRef.current.filter((segment) => segment.isFinal).map((segment) => segment.text).join(' '),
    );

    if (!finalText) {
      setVoiceState('error');
      setVoiceError('No speech was detected. Try again or type your plan instead.');
      setVoiceStatus('');
      return;
    }

    setTranscript(finalText);
    setNotes(mergeVoiceText(voiceBaseNotesRef.current, finalText));
    setVoiceState('review');
    setVoiceStatus('Review the transcript above before generating your plan.');
  };

  const startRecordedSession = async (sessionId: number) => {
    if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') {
      setVoiceState('error');
      setVoiceError('This browser cannot record audio. You can type your plan instead.');
      setVoiceStatus('');
      return;
    }

    const mimeType = getRecorderMimeType();
    if (!mimeType) {
      setVoiceState('error');
      setVoiceError('This browser has no supported audio recording format.');
      setVoiceStatus('');
      return;
    }

    setVoiceMode('recorded');
    setVoiceState('starting');
    setVoiceStatus('Requesting microphone permission…');

    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      if (voiceSessionRef.current !== sessionId || cancelRequestedRef.current) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }

      const recorder = new MediaRecorder(stream, { mimeType });
      mediaStreamRef.current = stream;
      mediaRecorderRef.current = recorder;
      audioChunksRef.current = [];

      recorder.ondataavailable = (event) => {
        if (event.data.size > 0) audioChunksRef.current.push(event.data);
      };
      recorder.onerror = () => {
        cleanupMediaStream();
        setVoiceState('error');
        setVoiceError('The browser could not record this voice note. Try again or type your plan.');
        setVoiceStatus('');
      };
      recorder.onstop = async () => {
        const canceled = cancelRequestedRef.current || voiceSessionRef.current !== sessionId;
        const blob = new Blob(audioChunksRef.current, { type: recorder.mimeType || mimeType });
        cleanupMediaStream();
        if (canceled) return;

        if (blob.size === 0) {
          setVoiceState('error');
          setVoiceError('No audio was recorded. Check microphone permission and try again.');
          setVoiceStatus('');
          return;
        }
        if (blob.size > MAX_AUDIO_BYTES) {
          setVoiceState('error');
          setVoiceError('This recording is too large. Keep voice notes under 2 minutes.');
          setVoiceStatus('');
          return;
        }

        setVoiceState('processing');
        setVoiceStatus('Transcribing your recording…');
        try {
          const audioBase64 = await blobToBase64(blob);
          if (voiceSessionRef.current !== sessionId || cancelRequestedRef.current) return;
          transcribeAudio.mutate(
            {
              data: {
                audioBase64,
                mimeType: (blob.type || mimeType).split(';')[0],
              },
            },
            {
              onSuccess: (data) => {
                if (voiceSessionRef.current !== sessionId || cancelRequestedRef.current) return;
                const spokenText = removeRepeatedTail(data.transcript);
                if (!spokenText) {
                  setVoiceState('error');
                  setVoiceError('No speech was detected. Try again or type your plan instead.');
                  setVoiceStatus('');
                  return;
                }
                setTranscript(spokenText);
                setNotes(mergeVoiceText(voiceBaseNotesRef.current, spokenText));
                setVoiceState('review');
                setVoiceStatus('Review the transcript above before generating your plan.');
              },
              onError: (error) => {
                if (voiceSessionRef.current !== sessionId || cancelRequestedRef.current) return;
                const message = error instanceof Error ? error.message : '';
                setVoiceState('error');
                setVoiceError(message || 'Voice transcription failed. Try again or type your plan instead.');
                setVoiceStatus('');
              },
            },
          );
        } catch {
          setVoiceState('error');
          setVoiceError('The recording could not be prepared. Try again or type your plan instead.');
          setVoiceStatus('');
        }
      };

      recorder.start(1000);
      setVoiceState('listening');
      setVoiceStatus('Speak naturally. Your transcript will be reviewable before submission.');
    } catch (error) {
      const denied = error instanceof DOMException && (error.name === 'NotAllowedError' || error.name === 'SecurityError');
      setVoiceState('error');
      setVoiceError(
        denied
          ? 'Microphone permission was denied. Allow microphone access or type your plan instead.'
          : 'The microphone could not be started. Check your browser permissions and try again.',
      );
      setVoiceStatus('');
    }
  };

  const startListening = async () => {
    if (voiceState === 'starting' || voiceState === 'listening' || voiceState === 'processing') return;

    const sessionId = voiceSessionRef.current + 1;
    voiceSessionRef.current = sessionId;
    voiceBaseNotesRef.current = notes;
    cancelRequestedRef.current = false;
    stopRequestedRef.current = false;
    durationStopRequestedRef.current = false;
    liveSegmentsRef.current = [];
    audioChunksRef.current = [];
    setTranscript('');
    setRecordingSeconds(0);
    setVoiceError('');
    setVoiceStatus('');

    const SpeechRecognition = (window as any).SpeechRecognition || (window as any).webkitSpeechRecognition;
    if (!SpeechRecognition) {
      await startRecordedSession(sessionId);
      return;
    }

    try {
      const recognition = new SpeechRecognition();
      recognition.continuous = true;
      recognition.interimResults = true;
      recognition.lang = 'en-US';
      recognition.onresult = (event: any) => {
        if (voiceSessionRef.current !== sessionId || cancelRequestedRef.current) return;
        for (let index = 0; index < event.results.length; index += 1) {
          const result = event.results[index];
          liveSegmentsRef.current[index] = {
            text: result[0]?.transcript ?? '',
            isFinal: Boolean(result.isFinal),
          };
        }
        refreshLiveTranscript();
      };
      recognition.onerror = (event: any) => {
        if (voiceSessionRef.current !== sessionId || cancelRequestedRef.current) return;
        const recoverable = event.error === 'network' || event.error === 'service-not-allowed' || event.error === 'audio-capture';
        if (recoverable && !stopRequestedRef.current) {
          recognition.abort();
          const fallbackSessionId = voiceSessionRef.current + 1;
          voiceSessionRef.current = fallbackSessionId;
          void startRecordedSession(fallbackSessionId);
          return;
        }
        if (event.error === 'no-speech') {
          setVoiceStatus('No speech detected yet. Keep speaking or stop to review.');
          return;
        }
        if (event.error === 'not-allowed') {
          setVoiceState('error');
          setVoiceError('Microphone permission was denied. Allow microphone access or type your plan instead.');
          setVoiceStatus('');
          return;
        }
        setVoiceState('error');
        setVoiceError('Live transcription stopped unexpectedly. Try again or type your plan instead.');
        setVoiceStatus('');
      };
      recognition.onend = () => {
        if (voiceSessionRef.current !== sessionId || cancelRequestedRef.current) return;
        recognitionRef.current = null;
        finishLiveReview(sessionId);
      };
      recognitionRef.current = recognition;
      setVoiceMode('live');
      setVoiceState('listening');
      setVoiceStatus('Speak naturally. Your transcript will be reviewable before submission.');
      recognition.start();
    } catch {
      recognitionRef.current = null;
      await startRecordedSession(sessionId);
    }
  };

  const stopListening = () => {
    if (voiceState !== 'listening') return;
    stopRequestedRef.current = true;
    setVoiceStatus('Finishing your recording…');

    if (voiceMode === 'live') {
      setVoiceState('processing');
      recognitionRef.current?.stop();
      return;
    }

    mediaRecorderRef.current?.stop();
  };

  const cancelVoice = () => {
    cancelRequestedRef.current = true;
    voiceSessionRef.current += 1;
    recognitionRef.current?.abort();
    if (mediaRecorderRef.current && mediaRecorderRef.current.state !== 'inactive') {
      mediaRecorderRef.current.stop();
    }
    cleanupMediaStream();
    setNotes(voiceBaseNotesRef.current);
    setTranscript('');
    setVoiceState('idle');
    setVoiceMode(null);
    setVoiceStatus('Voice input canceled.');
    setVoiceError('');
    setRecordingSeconds(0);
  };

  useEffect(() => {
    if (voiceState !== 'listening') return;
    const timer = window.setInterval(() => {
      setRecordingSeconds((current) => {
        const next = current + 1;
        if (next >= MAX_RECORDING_MS / 1000 && !durationStopRequestedRef.current) {
          durationStopRequestedRef.current = true;
          window.setTimeout(stopListening, 0);
        }
        return next;
      });
    }, 1000);
    return () => window.clearInterval(timer);
  }, [voiceState]);

  useEffect(() => () => {
    recognitionRef.current?.abort();
    cleanupMediaStream();
  }, []);

  const openAdd = () => {
    form.reset({
      title: '',
      timeBlock: '',
      priority: 'medium',
    });
    setDialogOpen(true);
  };

  const onSubmit = (data: PlanFormValues) => {
    createPlan.mutate({
      data: { ...data, date: dateStr }
    }, {
      onSuccess: () => {
        qc.invalidateQueries({ queryKey: getListDailyPlansQueryKey() });
        qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
        setDialogOpen(false);
      }
    });
  };

  const handleDelete = (id: number) => {
    deletePlan.mutate({ id }, {
      onSuccess: () => {
        qc.invalidateQueries({ queryKey: getListDailyPlansQueryKey() });
        qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
      }
    });
  };

  const toggleComplete = (item: DailyPlan) => {
    // optimistic
    qc.setQueryData([...getListDailyPlansQueryKey(), dateStr], (old: DailyPlan[] | undefined) => {
      if (!old) return old;
      return old.map(p => p.id === item.id ? { ...p, completed: !item.completed } : p);
    });

    updatePlan.mutate({
      id: item.id,
      data: { completed: !item.completed }
    }, {
      onSuccess: () => {
        qc.invalidateQueries({ queryKey: getListDailyPlansQueryKey() });
        qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
      }
    });
  };

  // Sort: incomplete first, then by priority (high->medium->low)
  const sortedPlans = plans ? [...plans].sort((a, b) => {
    if (a.completed !== b.completed) return a.completed ? 1 : -1;
    const pScore = { high: 0, medium: 1, low: 2 };
    return pScore[a.priority] - pScore[b.priority];
  }) : [];
  const voiceIsBusy = voiceState === 'starting' || voiceState === 'listening' || voiceState === 'processing';
  const isListening = voiceState === 'listening';

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
        
        {/* Date Selector */}
        <div className="flex bg-card/50 p-1 rounded-xl border border-border/50 backdrop-blur-sm self-start sm:self-auto">
          <Button 
            variant="ghost" 
            size="sm"
            onClick={() => setSelectedDate(subDays(new Date(), 1))}
            className={cn("rounded-lg px-4 h-9", dateStr === format(subDays(new Date(), 1), 'yyyy-MM-dd') ? 'bg-primary text-primary-foreground hover:bg-primary hover:text-primary-foreground shadow-sm' : 'text-muted-foreground')}
          >
            Yesterday
          </Button>
          <Button 
            variant="ghost" 
            size="sm"
            onClick={() => setSelectedDate(new Date())}
            className={cn("rounded-lg px-4 h-9", dateStr === format(new Date(), 'yyyy-MM-dd') ? 'bg-primary text-primary-foreground hover:bg-primary hover:text-primary-foreground shadow-sm' : 'text-muted-foreground')}
          >
            Today
          </Button>
          <Button 
            variant="ghost" 
            size="sm"
            onClick={() => setSelectedDate(addDays(new Date(), 1))}
            className={cn("rounded-lg px-4 h-9", dateStr === format(addDays(new Date(), 1), 'yyyy-MM-dd') ? 'bg-primary text-primary-foreground hover:bg-primary hover:text-primary-foreground shadow-sm' : 'text-muted-foreground')}
          >
            Tomorrow
          </Button>
          <div className="w-px bg-border/50 mx-1 my-1" />
          <div className="relative">
            <input 
              type="date" 
              className="opacity-0 absolute inset-0 w-full h-full cursor-pointer z-10"
              value={dateStr}
              onChange={(e) => {
                if (e.target.value) setSelectedDate(new Date(e.target.value + 'T12:00:00'));
              }}
            />
            <Button variant="ghost" size="icon" className="h-9 w-9 rounded-lg text-muted-foreground">
              <Calendar className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </header>

      <div className="flex-1 min-h-0 grid grid-cols-1 lg:grid-cols-2 gap-6 pb-6">
        
        {/* LEFT PANEL - AI Input */}
        <Card className="flex flex-col border-border bg-card/30 backdrop-blur-sm h-full overflow-hidden shadow-sm">
          <CardHeader className="pb-3 border-b border-border/50 shrink-0 bg-card/50">
            <CardTitle className="text-lg font-display flex items-center gap-2">
              <Sparkles className="h-5 w-5 text-primary" />
              What's on your mind?
            </CardTitle>
          </CardHeader>
          <CardContent className="flex-1 flex flex-col p-4 min-h-0">
            <Textarea
              className="flex-1 resize-none bg-black/20 border-white/5 focus-visible:ring-1 focus-visible:ring-primary/50 text-lg leading-relaxed placeholder:text-muted-foreground/40 p-4"
              placeholder={`Write your intentions, tasks, meetings, anything on your mind...\n\nExample:\n"I have a marketing sync at 10am, need to buy groceries for dinner, finish the Q3 report by EOD, and squeeze in a 30min run."`}
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
            />
            <div className="pt-4 shrink-0 flex items-center justify-between">
              <span className="text-xs text-muted-foreground flex items-center gap-1.5">
                <AlertCircle className="h-3.5 w-3.5" />
                AI will extract tasks, estimate priorities, and suggest time blocks.
              </span>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="icon"
                  onClick={isListening ? stopListening : startListening}
                  disabled={voiceToPlan.isPending}
                  className={cn(
                    'border-border',
                    isListening && 'bg-rose-500/10 text-rose-500 border-rose-500/30 animate-pulse',
                  )}
                  title={isListening ? 'Stop recording' : 'Record voice note'}
                  data-testid="button-voice-record"
                >
                  {isListening ? <MicOff className="h-4 w-4" /> : <Mic className="h-4 w-4" />}
                </Button>
                <Button 
                  onClick={generateWithAI} 
                  disabled={!notes.trim() || isGenerating || isListening}
                  className="bg-primary/20 hover:bg-primary/30 text-primary border border-primary/30"
                >
                  {isGenerating ? 'Thinking...' : 'Generate Plan with AI'}
                  <Sparkles className="h-4 w-4 ml-2" />
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* RIGHT PANEL - Structured Plan */}
        <Card className="flex flex-col border-border bg-card/50 backdrop-blur-sm h-full overflow-hidden shadow-sm">
          <CardHeader className="pb-3 border-b border-border/50 shrink-0 flex flex-row items-center justify-between bg-card/50">
            <CardTitle className="text-lg font-display flex items-center gap-2">
              <ListTodo className="h-5 w-5 text-foreground" />
              Structured Plan
              <span className="ml-2 text-xs font-medium text-muted-foreground bg-black/20 px-2 py-0.5 rounded-md border border-white/5">
                {format(selectedDate, 'MMM d')}
              </span>
            </CardTitle>
            <Button variant="ghost" size="icon" onClick={openAdd} className="h-8 w-8 text-muted-foreground hover:text-foreground">
              <Plus className="h-4 w-4" />
            </Button>
          </CardHeader>
          <div className="flex-1 overflow-y-auto p-4 space-y-3">
            {isLoading ? (
              <div className="space-y-3">
                {[...Array(5)].map((_, i) => (
                  <div key={i} className="h-16 w-full bg-white/5 animate-pulse rounded-xl" />
                ))}
              </div>
            ) : isGenerating ? (
              <div className="space-y-3">
                {[...Array(3)].map((_, i) => (
                  <div key={i} className="h-16 w-full bg-primary/5 border border-primary/10 animate-pulse rounded-xl" />
                ))}
              </div>
            ) : sortedPlans.length > 0 ? (
              sortedPlans.map((plan) => (
                <div 
                  key={plan.id} 
                  className={cn(
                    "group flex items-center justify-between p-3.5 rounded-xl border transition-all duration-200",
                    plan.completed 
                      ? "bg-black/20 border-white/5 opacity-60" 
                      : "bg-card hover:bg-card/80 border-white/10 hover:border-white/20 shadow-sm"
                  )}
                >
                  <div className="flex items-center gap-4 flex-1 overflow-hidden">
                    <button
                      onClick={() => toggleComplete(plan)}
                      className={cn(
                        "shrink-0 h-6 w-6 rounded-full border flex items-center justify-center transition-colors",
                        plan.completed ? "bg-primary border-primary text-primary-foreground" : "border-muted-foreground/40 hover:border-primary/50"
                      )}
                    >
                      {plan.completed && <CheckCircle2 className="h-4 w-4" />}
                    </button>
                    
                    <div className={cn("w-1 h-8 rounded-full shrink-0", priorityColors[plan.priority].split(' ')[0], priorityColors[plan.priority].split(' ')[1])} />
                    
                    <div className="flex-1 min-w-0">
                      <p className={cn("font-medium truncate transition-colors", plan.completed ? "line-through text-muted-foreground" : "text-foreground")}>
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
                    className="shrink-0 h-8 w-8 opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground hover:text-destructive hover:bg-destructive/10"
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

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-[425px]">
          <DialogHeader>
            <DialogTitle>Add to Plan</DialogTitle>
          </DialogHeader>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="title">Task or Intent</Label>
              <Input 
                id="title" 
                placeholder="e.g. Weekly review meeting" 
                {...form.register('title')} 
              />
              {form.formState.errors.title && (
                <p className="text-xs text-destructive">{form.formState.errors.title.message}</p>
              )}
            </div>
            
            <div className="space-y-2">
              <Label htmlFor="timeBlock">Time Block (optional)</Label>
              <Input 
                id="timeBlock" 
                placeholder="e.g. 10:00 AM - 11:00 AM" 
                {...form.register('timeBlock')} 
              />
            </div>
            
            <div className="space-y-2">
              <Label>Priority</Label>
              <div className="flex gap-4">
                <label className="flex items-center gap-2 text-sm cursor-pointer">
                  <input type="radio" value="high" {...form.register('priority')} className="accent-rose-500" />
                  <span className="text-rose-500 font-medium">High</span>
                </label>
                <label className="flex items-center gap-2 text-sm cursor-pointer">
                  <input type="radio" value="medium" {...form.register('priority')} className="accent-amber-500" />
                  <span className="text-amber-500 font-medium">Medium</span>
                </label>
                <label className="flex items-center gap-2 text-sm cursor-pointer">
                  <input type="radio" value="low" {...form.register('priority')} className="accent-blue-500" />
                  <span className="text-blue-500 font-medium">Low</span>
                </label>
              </div>
            </div>

            <DialogFooter className="pt-4">
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={form.formState.isSubmitting}>
                {form.formState.isSubmitting ? 'Adding...' : 'Add Item'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </PageTransition>
  );
}
