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
          <DialogTitle><ShieldCheck className="mr-2 inline h-5 w-5 text-primary" />Before using Askolo voice features</DialogTitle>
          <DialogDescription>Review how voice recording, transcription, and Live Mode are handled.</DialogDescription>
        </DialogHeader>
        <div className="space-y-3 text-sm text-muted-foreground">
          <p>
            Voice audio is sent through Askolo’s server connection to AssemblyAI. Recorded transcription requests deletion of the provider transcript, but that does not confirm provider audio was deleted. Live Mode uses AssemblyAI’s managed Voice Agent, which may retain session audio and transcripts under provider settings. When a session ends, Askolo requests a provider soft-delete; the response does not confirm physical erasure.
          </p>
          <p>
            Askolo does not put Live Mode audio or raw provider transcripts in recovery storage or logs. If you ask Live Mode to prepare a workspace action, Askolo redacts detected personal information before sending the transcript to the Assistant planner; that redacted text may appear in your Assistant conversation. Redaction can miss details, so do not speak passwords, codes, or other secrets.
          </p>
          <p>
            AssemblyAI’s processing location is not verified, and Askolo cannot promise regional residency. Account-wide model-improvement opt-out and Voice Agent-specific retention coverage have not been verified. Managed Live Mode stays unavailable until provider privacy settings are verified and an administrator configures its credit rate. If enabled, sessions end after 180 seconds at most. English speech is supported; Arabic Live Mode is unavailable. Voice is optional, and workspace changes still require your confirmation.
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