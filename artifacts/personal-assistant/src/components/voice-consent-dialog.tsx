import { AIConsentDialog } from '@/components/settings/ai-privacy-center';

export function VoiceConsentDialog({ open, onOpenChange, onConfirm, saving, error }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
  saving: boolean;
  error?: string;
}) {
  return (
    <AIConsentDialog
      purpose="recorded-voice"
      open={open}
      redactionConfirmed={true}
      saving={saving}
      error={error}
      onOpenChange={onOpenChange}
      onConfirm={onConfirm}
    />
  );
}