import { useEffect, useState } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import {
  Plus,
  Trash2,
  Edit2,
  Save,
  X,
  FileText,
  Wand2,
  CheckCircle2,
  ListTodo,
  Sparkles,
  Mic,
  MicOff,
  Upload,
} from 'lucide-react';
import {
  useListNotes,
  useCreateNote,
  useUpdateNote,
  useDeleteNote,
  useMeetingExtract,
  getListNotesQueryKey,
  getListActionItemsQueryKey,
  getGetDashboardSummaryQueryKey,
  Note,
  useGetTranscriptionPreferences,
  useUpdateTranscriptionPreferences,
  getGetTranscriptionPreferencesQueryKey,
} from '@workspace/api-client-react';
import { useQueryClient } from '@tanstack/react-query';
import { PageTransition } from '@/components/ui/page-transition';
import { Card, CardContent, CardHeader, CardTitle } from '@workspace/askolo-design-system/components/ui/card';
import { Button } from '@workspace/askolo-design-system/components/ui/button';
import { Input } from '@workspace/askolo-design-system/components/ui/input';
import { Textarea } from '@workspace/askolo-design-system/components/ui/textarea';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@workspace/askolo-design-system/components/ui/dialog';
import { Label } from '@workspace/askolo-design-system/components/ui/label';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@workspace/askolo-design-system/components/ui/tabs';
import { Badge } from '@workspace/askolo-design-system/components/ui/badge';
import { useVoiceTranscription } from '@/hooks/use-voice-transcription';
import { VoiceConsentDialog } from '@/components/voice-consent-dialog';

const noteSchema = z.object({
  title: z.string().min(1, 'Title is required'),
  content: z.string().min(1, 'Note is required'),
  tags: z.string().optional(),
});

type NoteForm = z.infer<typeof noteSchema>;

export function NotesPage() {
  const qc = useQueryClient();
  const { data: notes, isLoading } = useListNotes();
  const createNote = useCreateNote();
  const updateNote = useUpdateNote();
  const deleteNote = useDeleteNote();
  const meetingExtract = useMeetingExtract();

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingNote, setEditingNote] = useState<Note | null>(null);
  const [meetingNotes, setMeetingNotes] = useState('');
  const [meetingReviewText, setMeetingReviewText] = useState('');
  const [noteReviewText, setNoteReviewText] = useState('');
  const [meetingResult, setMeetingResult] = useState<{
    summary: string;
    decisions: string[];
    actionItems: { id: number; title: string; dueDate: string | null; completed: boolean }[];
  } | null>(null);
  const [activeTab, setActiveTab] = useState('notes');
  const [consentOpen, setConsentOpen] = useState(false);
  const [consentError, setConsentError] = useState('');
  const [consentSaving, setConsentSaving] = useState(false);
  const [liveVoice, setLiveVoice] = useState(false);
  const { data: voicePreferences } = useGetTranscriptionPreferences();
  const updateVoicePreferences = useUpdateTranscriptionPreferences();
  const voice = useVoiceTranscription({ realtime: liveVoice });
  const noteVoice = useVoiceTranscription();

  const form = useForm<NoteForm>({
    resolver: zodResolver(noteSchema),
    defaultValues: { title: '', content: '', tags: '' },
  });

  const openAdd = () => {
    setEditingNote(null);
    form.reset({ title: '', content: '', tags: '' });
    setDialogOpen(true);
  };

  const openEdit = (note: Note) => {
    setEditingNote(note);
    form.reset({
      title: note.title,
      content: note.content,
      tags: note.tags || '',
    });
    setDialogOpen(true);
  };

  const onSubmit = (data: NoteForm) => {
    if (noteVoice.isBusy) return;
    const payload = {
      title: data.title,
      content: data.content,
      tags: data.tags || undefined,
    };

    if (editingNote) {
      updateNote.mutate(
        { id: editingNote.id, data: payload },
        {
          onSuccess: () => {
            qc.invalidateQueries({ queryKey: getListNotesQueryKey() });
            setDialogOpen(false);
          },
        },
      );
    } else {
      createNote.mutate(
        { data: payload },
        {
          onSuccess: () => {
            qc.invalidateQueries({ queryKey: getListNotesQueryKey() });
            setDialogOpen(false);
          },
        },
      );
    }
  };

  const handleDelete = (id: number) => {
    if (confirm('Delete this note?')) {
      deleteNote.mutate(
        { id },
        {
          onSuccess: () => qc.invalidateQueries({ queryKey: getListNotesQueryKey() }),
        },
      );
    }
  };

  const extractMeeting = async () => {
    if (!meetingNotes.trim()) return;
    meetingExtract.mutate(
      { data: { notes: meetingNotes } },
      {
        onSuccess: (res) => {
          if (res) {
            setMeetingResult({
              summary: res.summary,
              decisions: res.decisions,
              actionItems: (res.actionItems as any[]).map((a) => ({
                id: a.id,
                title: a.title,
                dueDate: a.dueDate || null,
                completed: a.completed,
              })),
            });
            qc.invalidateQueries({ queryKey: getListActionItemsQueryKey() });
            qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
          }
        },
      },
    );
  };

  const startVoice = async () => {
    if (!voicePreferences?.consentGiven || voicePreferences.consentVersion !== 'voice-v3') {
      setConsentError('');
      setConsentOpen(true);
      return;
    }
    await voice.start();
  };

  const startNoteVoice = async () => {
    if (!voicePreferences?.consentGiven || voicePreferences.consentVersion !== 'voice-v3') {
      setConsentError('');
      setConsentOpen(true);
      return;
    }
    await noteVoice.start();
  };

  const canUseVoice = Boolean(voicePreferences?.consentGiven && voicePreferences.consentVersion === 'voice-v3');

  const saveConsent = async () => {
    setConsentSaving(true);
    try {
      const updated = await updateVoicePreferences.mutateAsync({ data: { consent: true } });
      qc.setQueryData(getGetTranscriptionPreferencesQueryKey(), updated);
      setConsentOpen(false);
    } catch {
      setConsentError('Consent could not be saved. Please try again.');
    } finally {
      setConsentSaving(false);
    }
  };

  const applyVoiceTranscript = () => {
    if (meetingReviewText.trim()) setMeetingNotes(meetingReviewText.trim());
  };

  useEffect(() => {
    if (voice.state === 'review') setMeetingReviewText(voice.transcript);
  }, [voice.state, voice.transcript]);
  useEffect(() => {
    if (noteVoice.state === 'review') setNoteReviewText(noteVoice.transcript);
  }, [noteVoice.state, noteVoice.transcript]);
  useEffect(() => {
    if (activeTab !== 'meetings' && voice.state !== 'idle') voice.reset();
  }, [activeTab, voice.state]);
  useEffect(() => {
    if (!dialogOpen && noteVoice.state !== 'idle') noteVoice.reset();
  }, [dialogOpen, noteVoice.state]);

  const applyNoteTranscript = () => {
    if (noteReviewText.trim()) form.setValue('content', noteReviewText.trim(), { shouldValidate: true });
  };

  return (
    <PageTransition className="max-w-5xl mx-auto space-y-6 pb-10">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-display font-bold tracking-tight">Notes & Meetings</h1>
          <p className="text-muted-foreground mt-1">Capture ideas and turn meeting chaos into clear next steps.</p>
        </div>
        <Button onClick={openAdd} className="gap-2" data-testid="button-add-note">
          <Plus className="h-4 w-4" /> New Note
        </Button>
      </header>

      <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
        <TabsList className="bg-card border border-border">
          <TabsTrigger value="notes" className="gap-2">
            <FileText className="h-4 w-4" /> My Notes
          </TabsTrigger>
          <TabsTrigger value="meetings" className="gap-2">
            <Sparkles className="h-4 w-4" /> Meeting Intelligence
          </TabsTrigger>
        </TabsList>

        <TabsContent value="notes" className="mt-6">
          {isLoading ? (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {[...Array(4)].map((_, i) => (
                <div key={i} className="h-40 bg-card rounded-xl animate-pulse" />
              ))}
            </div>
          ) : notes && notes.length > 0 ? (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <AnimatePresence>
                {notes.map((note) => (
                  <motion.div
                    key={note.id}
                    layout
                    initial={{ opacity: 0, scale: 0.98 }}
                    animate={{ opacity: 1, scale: 1 }}
                    exit={{ opacity: 0, scale: 0.98 }}
                  >
                    <Card className="border-border bg-card/50 backdrop-blur-sm hover:bg-card/70 transition-colors group h-full flex flex-col">
                      <CardHeader className="pb-3">
                        <div className="flex items-start justify-between gap-3">
                          <CardTitle className="text-lg font-display leading-tight">{note.title}</CardTitle>
                          <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8"
                              onClick={() => openEdit(note)}
                              data-testid={`button-edit-note-${note.id}`}
                            >
                              <Edit2 className="h-4 w-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8 text-destructive hover:text-destructive"
                              onClick={() => handleDelete(note.id)}
                              data-testid={`button-delete-note-${note.id}`}
                            >
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          </div>
                        </div>
                        {note.tags && (
                          <div className="flex flex-wrap gap-1.5 mt-2">
                            {note.tags.split(',').map((tag) => (
                              <Badge key={tag} variant="outline" className="text-xs">
                                {tag.trim()}
                              </Badge>
                            ))}
                          </div>
                        )}
                      </CardHeader>
                      <CardContent className="flex-1">
                        <p className="text-sm text-muted-foreground whitespace-pre-wrap line-clamp-6">
                          {note.content}
                        </p>
                      </CardContent>
                    </Card>
                  </motion.div>
                ))}
              </AnimatePresence>
            </div>
          ) : (
            <Card className="border-dashed border-border bg-card/30">
              <CardContent className="py-16 text-center">
                <FileText className="h-10 w-10 text-muted-foreground/50 mx-auto mb-4" />
                <p className="text-muted-foreground">No notes yet.</p>
                <Button variant="link" className="text-primary mt-2" onClick={openAdd}>
                  Create your first note
                </Button>
              </CardContent>
            </Card>
          )}
        </TabsContent>

        <TabsContent value="meetings" className="mt-6 space-y-6">
          <Card className="border-border bg-card/50 backdrop-blur-sm">
            <CardHeader>
              <CardTitle className="text-lg font-display flex items-center gap-2">
                <Wand2 className="h-5 w-5 text-primary" />
                Extract meeting intelligence
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <Textarea
                placeholder="Paste raw meeting notes here — who said what, decisions, action items, deadlines..."
                className="min-h-[200px] bg-background border-border resize-none"
                value={meetingNotes}
                onChange={(e) => setMeetingNotes(e.target.value)}
                data-testid="meeting-notes-input"
              />
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border bg-muted/30 p-3">
                <div className="text-xs text-muted-foreground" aria-live="polite">
                  {voice.state === 'listening' ? `Recording ${voice.recordingSeconds}s…` : voice.state === 'review' ? 'Transcript ready for review.' : 'Add audio only when it helps.'}
                </div>
                {voice.state === 'listening' && liveVoice && voice.liveText && (
                  <p aria-live="off" className="w-full whitespace-pre-wrap break-words rounded-md border border-border bg-background p-2 text-sm text-muted-foreground">
                    <span className="sr-only">Live transcript preview: </span>{voice.liveText}
                  </p>
                )}
                <div className="flex flex-wrap gap-2">
                  <Button type="button" variant="ghost" size="sm" onClick={() => setLiveVoice((value) => !value)} disabled={voice.isBusy} aria-pressed={liveVoice} data-testid="button-toggle-notes-live-voice">{liveVoice ? 'Recorded mode' : 'Live mode'}</Button>
                  <Button type="button" variant="outline" size="sm" onPointerDown={(event) => { event.currentTarget.setPointerCapture?.(event.pointerId); void startVoice(); }} onPointerUp={voice.stop} onPointerCancel={voice.cancel} onBlur={voice.cancel} onKeyDown={(event) => { if ((event.key === ' ' || event.key === 'Enter') && !event.repeat) { event.preventDefault(); void startVoice(); } }} onKeyUp={(event) => { if (event.key === ' ' || event.key === 'Enter') { event.preventDefault(); voice.stop(); } }} onClick={(event) => event.preventDefault()} disabled={voice.state === 'processing'} aria-label={voice.isListening ? 'Release to stop recording' : 'Press and hold to record meeting notes'} data-testid="button-notes-voice">
                    {voice.isListening ? <MicOff className="mr-2 h-4 w-4" /> : <Mic className="mr-2 h-4 w-4" />} {voice.isListening ? 'Release to stop' : 'Record'}
                  </Button>
                  <input id="meeting-audio-file" type="file" accept="audio/webm,audio/mp4,audio/m4a,audio/wav,audio/ogg,audio/mpeg" className="sr-only" tabIndex={-1} aria-hidden="true" disabled={voice.isBusy} onClick={(event) => { if (!canUseVoice) { event.preventDefault(); setConsentOpen(true); } }} onChange={(event) => { const file = event.target.files?.[0]; if (file) void voice.transcribeFile(file); event.currentTarget.value = ''; }} />
                  <Button type="button" variant="outline" size="sm" disabled={voice.isBusy} onClick={() => document.getElementById('meeting-audio-file')?.click()} data-testid="button-upload-meeting-audio">
                    <Upload className="mr-2 h-4 w-4" /> Upload audio
                  </Button>
                </div>
                {voice.error && <p className="w-full text-xs text-destructive" role="alert">{voice.error}</p>}
                {(voice.isBusy || voice.state === 'error') && <div className="flex w-full gap-2"><Button type="button" size="sm" variant="ghost" onClick={voice.cancel}>{voice.isBusy ? 'Cancel voice input' : 'Discard'}</Button>{voice.state === 'error' && <Button type="button" size="sm" variant="outline" onClick={() => void voice.retry()} disabled={!voice.recording}>Retry</Button>}</div>}
                {voice.reviewSignals.length > 0 && (
                  <div className="w-full rounded-md border border-warning/30 bg-warning/10 p-2 text-xs text-warning">
                    <p className="font-medium">Please verify these low-confidence details:</p>
                    <ul className="mt-1 list-disc pl-4">
                      {voice.reviewSignals.map((signal) => <li key={`${signal.startMs ?? 'unknown'}-${signal.text}`}>{signal.text} ({Math.round(signal.confidence * 100)}% confidence)</li>)}
                    </ul>
                  </div>
                )}
                {voice.deletionStatus && (
                  <p className="w-full text-xs text-muted-foreground" role={voice.deletionStatus.providerTranscript === 'deletion_failed' ? 'alert' : 'status'}>
                    {voice.deletionStatus.providerTranscript === 'deleted'
                      ? 'Provider transcript deletion was confirmed; this does not confirm provider audio deletion.'
                      : 'Provider transcript deletion could not be confirmed; provider retention may apply.'}
                  </p>
                )}
                {voice.state === 'review' && voice.transcript && (
                  <div className="w-full space-y-2 rounded-md border border-primary/20 bg-primary/5 p-3">
                    <p className="text-sm font-medium">Review transcript</p>
                    <Textarea aria-label="Editable meeting transcript" value={meetingReviewText} onChange={(event) => setMeetingReviewText(event.target.value)} className="min-h-24 bg-background" dir="auto" />
                    <div className="flex flex-wrap gap-2">
                      <Button type="button" size="sm" onClick={applyVoiceTranscript}>Insert into meeting notes</Button>
                      <Button type="button" size="sm" variant="outline" onClick={() => { if (meetingReviewText.trim()) { setMeetingNotes(meetingReviewText.trim()); meetingExtract.mutate({ data: { notes: meetingReviewText.trim() } }); } }}>Use for extraction</Button>
                      <Button type="button" size="sm" variant="ghost" onClick={voice.reset}>Discard</Button>
                    </div>
                  </div>
                )}
              </div>
              <div className="flex justify-end">
                  <Button
                  onClick={extractMeeting}
                   disabled={meetingExtract.isPending || !meetingNotes.trim() || voice.isBusy || noteVoice.isBusy}
                  className="gap-2"
                  data-testid="button-extract-meeting"
                >
                  {meetingExtract.isPending ? (
                    <>
                      <div className="h-4 w-4 border-2 border-current border-t-transparent rounded-full animate-spin" />
                      Analyzing...
                    </>
                  ) : (
                    <>
                      <Sparkles className="h-4 w-4" /> Extract summary & actions
                    </>
                  )}
                </Button>
              </div>
            </CardContent>
          </Card>

          {meetingResult && (
            <motion.div
              initial={{ opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              className="grid grid-cols-1 lg:grid-cols-2 gap-6"
            >
              <Card className="border-border bg-card/50 backdrop-blur-sm">
                <CardHeader>
                  <CardTitle className="text-base font-display flex items-center gap-2">
                    <FileText className="h-4 w-4 text-primary" /> Summary
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  <p className="text-sm text-foreground leading-relaxed">{meetingResult.summary}</p>
                  {meetingResult.decisions.length > 0 && (
                    <div>
                      <h4 className="text-sm font-medium mb-2">Decisions made</h4>
                      <ul className="space-y-2">
                        {meetingResult.decisions.map((decision, idx) => (
                          <li key={idx} className="flex gap-2 text-sm text-muted-foreground">
                            <CheckCircle2 className="h-4 w-4 text-success shrink-0 mt-0.5" />
                            {decision}
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}
                </CardContent>
              </Card>

              <Card className="border-border bg-card/50 backdrop-blur-sm">
                <CardHeader>
                  <CardTitle className="text-base font-display flex items-center gap-2">
                    <ListTodo className="h-4 w-4 text-primary" /> Action items created
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  {meetingResult.actionItems.length > 0 ? (
                    <ul className="space-y-3">
                      {meetingResult.actionItems.map((item) => (
                        <li
                          key={item.id}
                          className="flex items-start gap-3 p-3 rounded-lg bg-muted/40 border border-border"
                        >
                          <div className="mt-0.5 h-4 w-4 rounded border border-muted-foreground/50 shrink-0" />
                          <div className="flex-1">
                            <p className="text-sm font-medium">{item.title}</p>
                            {item.dueDate && (
                              <p className="text-xs text-muted-foreground mt-1">Due {item.dueDate}</p>
                            )}
                          </div>
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p className="text-sm text-muted-foreground">No clear action items detected.</p>
                  )}
                </CardContent>
              </Card>
            </motion.div>
          )}
        </TabsContent>
      </Tabs>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="bg-card border-border max-w-lg">
          <DialogHeader>
            <DialogTitle className="font-display">
              {editingNote ? 'Edit note' : 'Create note'}
            </DialogTitle>
          </DialogHeader>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4 py-2">
            <div className="space-y-2">
              <Label htmlFor="title">Title</Label>
              <Input id="title" {...form.register('title')} placeholder="Note title" />
              {form.formState.errors.title && (
                <p className="text-xs text-destructive">{form.formState.errors.title.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="content">Content</Label>
              <Textarea
                id="content"
                {...form.register('content')}
                placeholder="Write your thoughts..."
                className="min-h-[160px] bg-background border-border resize-none"
              />
              <div className="flex flex-wrap items-center gap-2">
                <Button type="button" variant="outline" size="sm" onPointerDown={(event) => { event.currentTarget.setPointerCapture?.(event.pointerId); void startNoteVoice(); }} onPointerUp={noteVoice.stop} onPointerCancel={noteVoice.cancel} onBlur={noteVoice.cancel} onKeyDown={(event) => { if ((event.key === ' ' || event.key === 'Enter') && !event.repeat) { event.preventDefault(); void startNoteVoice(); } }} onKeyUp={(event) => { if (event.key === ' ' || event.key === 'Enter') { event.preventDefault(); noteVoice.stop(); } }} onClick={(event) => event.preventDefault()} disabled={noteVoice.state === 'processing'} aria-label={noteVoice.isListening ? 'Release to stop note recording' : 'Press and hold to record note audio'} data-testid="button-note-voice">{noteVoice.isListening ? <MicOff className="mr-2 h-4 w-4" /> : <Mic className="mr-2 h-4 w-4" />} {noteVoice.isListening ? 'Release to stop' : 'Record audio'}</Button>
                <input id="note-audio-file" type="file" accept="audio/webm,audio/mp4,audio/m4a,audio/wav,audio/ogg,audio/mpeg" className="sr-only" tabIndex={-1} aria-hidden="true" disabled={noteVoice.isBusy} onClick={(event) => { if (!canUseVoice) { event.preventDefault(); setConsentOpen(true); } }} onChange={(event) => { const file = event.target.files?.[0]; if (file) void noteVoice.transcribeFile(file); event.currentTarget.value = ''; }} />
                <Button type="button" variant="outline" size="sm" disabled={noteVoice.isBusy} onClick={() => document.getElementById('note-audio-file')?.click()}>
                  <Upload className="mr-2 h-4 w-4" /> Upload audio
                </Button>
                <span className="text-xs text-muted-foreground" aria-live="polite">{noteVoice.isListening ? `Recording ${noteVoice.recordingSeconds}s…` : noteVoice.status || 'Review before inserting; saving the note remains explicit.'}</span>
              </div>
              {noteVoice.error && <p role="alert" className="text-xs text-destructive">{noteVoice.error}</p>}
              {noteVoice.isBusy && <Button type="button" variant="ghost" size="sm" onClick={noteVoice.cancel}>Cancel voice input</Button>}
              {noteVoice.reviewSignals.length > 0 && (
                <div className="rounded-md border border-warning/30 bg-warning/10 p-2 text-xs text-warning">
                  <p className="font-medium">Please verify these low-confidence details:</p>
                  <ul className="mt-1 list-disc pl-4">
                    {noteVoice.reviewSignals.map((signal) => <li key={`${signal.startMs ?? 'unknown'}-${signal.text}`}>{signal.text} ({Math.round(signal.confidence * 100)}% confidence)</li>)}
                  </ul>
                </div>
              )}
              {noteVoice.deletionStatus && <p className="text-xs text-muted-foreground">{noteVoice.deletionStatus.providerTranscript === 'deleted' ? 'Provider transcript deletion confirmed; this does not confirm provider audio deletion.' : 'Provider transcript deletion could not be confirmed; provider retention may apply.'}</p>}
              {noteVoice.state === 'error' && (
                <div className="flex flex-wrap gap-2">
                  <Button type="button" size="sm" variant="outline" onClick={() => void noteVoice.retry()} disabled={!noteVoice.recording}>Retry transcription</Button>
                  <Button type="button" size="sm" variant="ghost" onClick={noteVoice.reset}>Discard</Button>
                </div>
              )}
              {noteVoice.state === 'review' && noteVoice.transcript && <div className="space-y-2 rounded-md border border-primary/20 bg-primary/5 p-3"><Label htmlFor="note-transcript-review">Editable transcript</Label><Textarea id="note-transcript-review" value={noteReviewText} onChange={(event) => setNoteReviewText(event.target.value)} className="min-h-24 bg-background" /><div className="flex gap-2"><Button type="button" size="sm" onClick={applyNoteTranscript}>Insert into note</Button><Button type="button" size="sm" variant="outline" onClick={noteVoice.reset}>Discard</Button><Button type="button" size="sm" variant="outline" onClick={() => void noteVoice.retry()} disabled={!noteVoice.recording}>Retry</Button></div></div>}
              {form.formState.errors.content && (
                <p className="text-xs text-destructive">{form.formState.errors.content.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="tags">Tags (comma separated)</Label>
              <Input id="tags" {...form.register('tags')} placeholder="ideas, work, personal" />
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setDialogOpen(false)}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={createNote.isPending || updateNote.isPending}>
                <Save className="h-4 w-4 mr-2" />
                {editingNote ? 'Save changes' : 'Create note'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <VoiceConsentDialog open={consentOpen} onOpenChange={setConsentOpen} onConfirm={() => void saveConsent()} saving={consentSaving} error={consentError} />
    </PageTransition>
  );
}
