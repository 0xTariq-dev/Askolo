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
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Badge } from '@/components/ui/badge';
import { useVoiceTranscription } from '@/hooks/use-voice-transcription';
import {
  VoiceCaptureButton,
  VoiceCaptureFeedback,
  VoiceTranscriptReview,
} from '@/components/voice/voice-session-ui';
import { getReviewedVoiceValue } from '@/lib/voice-flow';
import { VoiceConsentDialog } from '@/components/voice-consent-dialog';
import { CURRENT_VOICE_CONSENT_VERSION } from '@/lib/voice-consent';
import { getVoiceConsentErrorMessage } from '@/lib/voice-consent-errors';

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
  const { data: voicePreferences } = useGetTranscriptionPreferences();
  const updateVoicePreferences = useUpdateTranscriptionPreferences();
  const voice = useVoiceTranscription();
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
    if (!voicePreferences?.consentGiven || voicePreferences.consentVersion !== CURRENT_VOICE_CONSENT_VERSION) {
      setConsentError('');
      setConsentOpen(true);
      return;
    }
    await voice.start();
  };

  const startNoteVoice = async () => {
    if (!voicePreferences?.consentGiven || voicePreferences.consentVersion !== CURRENT_VOICE_CONSENT_VERSION) {
      setConsentError('');
      setConsentOpen(true);
      return;
    }
    await noteVoice.start();
  };

  const canUseVoice = Boolean(
    voicePreferences?.consentGiven &&
      voicePreferences.consentVersion === CURRENT_VOICE_CONSENT_VERSION,
  );

  const saveConsent = async () => {
    setConsentSaving(true);
    try {
      const updated = await updateVoicePreferences.mutateAsync({ data: { consent: true } });
      qc.setQueryData(getGetTranscriptionPreferencesQueryKey(), updated);
      setConsentOpen(false);
    } catch (error) {
      setConsentError(getVoiceConsentErrorMessage(error));
    } finally {
      setConsentSaving(false);
    }
  };

  const applyVoiceTranscript = () => {
    const reviewed = getReviewedVoiceValue(meetingNotes, meetingReviewText, 'replace');
    if (reviewed !== null) setMeetingNotes(reviewed);
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
    const reviewed = getReviewedVoiceValue(form.getValues('content'), noteReviewText, 'replace');
    if (reviewed !== null) form.setValue('content', reviewed, { shouldValidate: true });
  };

  return (
    <PageTransition surface={false} className="max-w-5xl mx-auto space-y-6 pb-10">
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
              <VoiceCaptureFeedback voice={voice} />
              <div className="flex flex-wrap gap-2">
                <VoiceCaptureButton
                  voice={voice}
                  onStart={startVoice}
                  startText="Press and hold or activate to record meeting notes"
                  stopText="Recording meeting notes; release or activate to stop"
                  variant="outline"
                  size="sm"
                  disabled={voice.state === 'starting' || voice.state === 'processing'}
                  data-testid="button-notes-voice"
                />
                <input
                  id="meeting-audio-file"
                  type="file"
                  accept="audio/webm,audio/mp4,audio/m4a,audio/wav,audio/ogg,audio/mpeg"
                  className="sr-only"
                  tabIndex={-1}
                  aria-hidden="true"
                  disabled={voice.isBusy}
                  onClick={(event) => {
                    if (!canUseVoice) {
                      event.preventDefault();
                      setConsentOpen(true);
                    }
                  }}
                  onChange={(event) => {
                    const file = event.target.files?.[0];
                    if (file) void voice.transcribeFile(file);
                    event.currentTarget.value = '';
                  }}
                />
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={voice.isBusy}
                  onClick={() => document.getElementById('meeting-audio-file')?.click()}
                  data-testid="button-upload-meeting-audio"
                >
                  <Upload aria-hidden="true" /> Upload audio
                </Button>
              </div>
              <VoiceTranscriptReview
                id="meeting-voice-transcript"
                state={voice.state}
                transcript={voice.transcript}
                value={meetingReviewText}
                onChange={setMeetingReviewText}
                label="Editable meeting transcript"
              >
                <Button type="button" size="sm" onClick={applyVoiceTranscript}>
                  Insert into meeting notes
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    const reviewed = getReviewedVoiceValue(meetingNotes, meetingReviewText, 'replace');
                    if (reviewed === null) return;
                    setMeetingNotes(reviewed);
                    meetingExtract.mutate({ data: { notes: reviewed } });
                  }}
                  disabled={!meetingReviewText.trim() || meetingExtract.isPending}
                >
                  Use for extraction
                </Button>
                <Button type="button" size="sm" variant="ghost" onClick={voice.reset}>
                  Discard transcript
                </Button>
              </VoiceTranscriptReview>
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
                <VoiceCaptureButton
                  voice={noteVoice}
                  onStart={startNoteVoice}
                  startText="Press and hold or activate to record note audio"
                  stopText="Recording note audio; release or activate to stop"
                  variant="outline"
                  size="sm"
                  disabled={noteVoice.state === 'starting' || noteVoice.state === 'processing'}
                  data-testid="button-note-voice"
                />
                <input
                  id="note-audio-file"
                  type="file"
                  accept="audio/webm,audio/mp4,audio/m4a,audio/wav,audio/ogg,audio/mpeg"
                  className="sr-only"
                  tabIndex={-1}
                  aria-hidden="true"
                  disabled={noteVoice.isBusy}
                  onClick={(event) => {
                    if (!canUseVoice) {
                      event.preventDefault();
                      setConsentOpen(true);
                    }
                  }}
                  onChange={(event) => {
                    const file = event.target.files?.[0];
                    if (file) void noteVoice.transcribeFile(file);
                    event.currentTarget.value = '';
                  }}
                />
                <Button type="button" variant="outline" size="sm" disabled={noteVoice.isBusy} onClick={() => document.getElementById('note-audio-file')?.click()}>
                  <Upload className="mr-2 h-4 w-4" /> Upload audio
                </Button>
              </div>
              <VoiceCaptureFeedback voice={noteVoice} />
              <VoiceTranscriptReview
                id="note-transcript-review"
                state={noteVoice.state}
                transcript={noteVoice.transcript}
                value={noteReviewText}
                onChange={setNoteReviewText}
                label="Editable note transcript"
              >
                <Button type="button" size="sm" onClick={applyNoteTranscript}>
                  Insert into note
                </Button>
                <Button type="button" size="sm" variant="ghost" onClick={noteVoice.reset}>
                  Discard transcript
                </Button>
              </VoiceTranscriptReview>
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
