import { ShieldCheck } from 'lucide-react';
import { Button } from '@workspace/askolo-design-system/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@workspace/askolo-design-system/components/ui/dialog';

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
          <DialogDescription>Voice is optional and always requires review before it is used.</DialogDescription>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">Voice audio is sent through Askolo’s server connection to AssemblyAI for live or recorded transcription. Askolo does not store audio or live transcripts. AssemblyAI processes voice data under its own retention and model-improvement settings. For recorded transcription, Askolo requests deletion of the provider transcript and reports whether deletion is confirmed; this does not prove uploaded provider audio was deleted. Review the editable transcript before inserting it into notes or sending a message. Voice never creates or sends an action automatically.</p>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>Not now</Button>
          <Button onClick={onConfirm} disabled={saving}>{saving ? 'Saving…' : 'I understand and continue'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}