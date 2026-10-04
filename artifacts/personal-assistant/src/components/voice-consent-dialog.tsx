import { ShieldCheck } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';

export function VoiceConsentDialog({ open, onOpenChange, onConfirm, saving, error }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
  saving: boolean;
  error?: string;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle><ShieldCheck className="mr-2 inline h-5 w-5 text-primary" />Before you use voice input</DialogTitle>
          <DialogDescription>Please review how Askolo handles voice notes and AI processing.</DialogDescription>
        </DialogHeader>
        <div className="space-y-3 text-sm text-muted-foreground">
          <p>
            Voice audio is sent through Askolo’s server connection to AssemblyAI for live or recorded transcription. Askolo does not store audio or live transcripts. AssemblyAI processes voice data under its own retention and model-improvement settings. For recorded transcription, Askolo requests deletion of the provider transcript and reports whether deletion is confirmed; that confirmation does not mean provider audio was deleted.
          </p>
          <p>
            We request automatic redaction of detected personal information before AssemblyAI returns a transcript. Redaction can miss details. In Assistant chat, a clear transcript is sent as a message when you stop recording; if transcription flags uncertainty, you can review and correct it before sending. In Notes and planning, transcripts remain available for review before insertion.
          </p>
          <p>
            Live transcription sessions end after 180 seconds at most. Voice is optional and requires your consent. Assistant actions are still only added after you confirm them.
          </p>
        </div>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>Not now</Button>
          <Button onClick={onConfirm} disabled={saving}>{saving ? 'Saving…' : 'I understand and continue'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}