import { useState, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import {
  getGetAIPrivacyPreferencesQueryKey,
  getGetTranscriptionPreferencesQueryKey,
  getGetVoiceOutputPreferencesQueryKey,
  useGetAIPrivacyPreferences,
  useGetVoiceOutputPreferences,
  useUpdateAIPrivacyPreferences,
  useUpdateVoiceOutputPreferences,
} from '@workspace/api-client-react';
import { ShieldCheck } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';

type ConsentPurpose = 'recorded-voice' | 'assistant-processing' | 'assemblyai-live' | 'azure-speech';

const purposeCopy: Record<ConsentPurpose, { title: string; description: string }> = {
  'recorded-voice': {
    title: 'Recorded voice transcription',
    description:
      'AssemblyAI receives recorded audio and returns the complete, editable transcript without provider-side redaction. Askolo requests deletion of the provider transcript, but that does not confirm provider audio deletion. Assistant processing, Live Mode, and Azure speech each require separate permission; app-side redaction can miss sensitive details.',
  },
  'assistant-processing': {
    title: 'Assistant processing and app-side redaction',
    description:
      'When you submit a message or voice transcript, Askolo sends it to its configured AI provider. Before sending, Askolo filters detected contact, financial and account, government-ID, credential, birth-date, and other high-risk values in the app. Names and project or client labels remain only after you grant this separate consent. Redaction can miss sensitive details.',
  },
  'assemblyai-live': {
    title: 'AssemblyAI Live Mode',
    description:
      'AssemblyAI’s managed Voice Agent processes live audio and produces spoken replies. The processing location and EU-only processing are unverified; account-wide model-improvement opt-out and Voice Agent retention coverage have not been verified. Askolo requests provider soft-delete when a session ends, but that does not prove physical erasure. Development Live Mode may proceed with your separate permission while these provider checks remain pending.',
  },
  'azure-speech': {
    title: 'Azure speech output',
    description:
      'When you choose Listen, Assistant response text is sent to Microsoft Azure Speech and generated audio is streamed to your device, not saved in Askolo. Automatic spoken replies are a separate setting and are enabled only if you choose them.',
  },
};

export function AIPrivacyCenter() {
  const queryClient = useQueryClient();
  const privacyQuery = useGetAIPrivacyPreferences();
  const outputQuery = useGetVoiceOutputPreferences();
  const privacyMutation = useUpdateAIPrivacyPreferences();
  const outputMutation = useUpdateVoiceOutputPreferences();
  const [purpose, setPurpose] = useState<ConsentPurpose | null>(null);
  const [redactionConfirmed, setRedactionConfirmed] = useState(false);
  const [autoSpeakAfterConsent, setAutoSpeakAfterConsent] = useState(false);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);

  const privacy = privacyQuery.data;
  const output = outputQuery.data;
  const outputConsentCurrent = Boolean(output?.consentGiven && output.consentVersion === 'azure-tts-v1');
  const openConsent = (nextPurpose: ConsentPurpose, autoSpeak = false) => {
    setError('');
    setRedactionConfirmed(false);
    setAutoSpeakAfterConsent(autoSpeak);
    setPurpose(nextPurpose);
  };
  const closeConsent = (open: boolean) => {
    if (!open && !saving) {
      setPurpose(null);
      setAutoSpeakAfterConsent(false);
    }
  };
  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: getGetAIPrivacyPreferencesQueryKey() }),
      queryClient.invalidateQueries({ queryKey: getGetTranscriptionPreferencesQueryKey() }),
      queryClient.invalidateQueries({ queryKey: getGetVoiceOutputPreferencesQueryKey() }),
    ]);
  };

  const updatePrivacy = async (body: {
    recordedVoiceInput?: boolean;
    assistantProcessing?: boolean;
    assemblyAiLive?: boolean;
    redactionLocation?: 'app';
  }) => {
    await privacyMutation.mutateAsync({ data: body });
    await refresh();
  };
  const updateOutput = async (body: { consent?: boolean; autoSpeakEnabled?: boolean }) => {
    await outputMutation.mutateAsync({ data: body });
    await refresh();
  };

  const confirmConsent = async () => {
    if (!purpose || (purpose === 'assistant-processing' && !redactionConfirmed)) return;
    setSaving(true);
    setError('');
    try {
      if (purpose === 'recorded-voice') {
        await updatePrivacy({ recordedVoiceInput: true });
      } else if (purpose === 'assistant-processing') {
        await updatePrivacy({ assistantProcessing: true, redactionLocation: 'app' });
      } else if (purpose === 'assemblyai-live') {
        await updatePrivacy({ assemblyAiLive: true });
      } else {
        await updateOutput({ consent: true, ...(autoSpeakAfterConsent ? { autoSpeakEnabled: true } : {}) });
      }
      setPurpose(null);
      setAutoSpeakAfterConsent(false);
    } catch {
      setError('Your privacy setting could not be saved. Please try again.');
    } finally {
      setSaving(false);
    }
  };

  const revokePrivacy = async (key: 'recordedVoiceInput' | 'assistantProcessing' | 'assemblyAiLive') => {
    setError('');
    try {
      await updatePrivacy({ [key]: false });
    } catch {
      setError('Your privacy setting could not be updated. Please try again.');
    }
  };
  const toggleAutoSpeak = async () => {
    if (!outputConsentCurrent) {
      openConsent('azure-speech', true);
      return;
    }
    setError('');
    try {
      await updateOutput({ autoSpeakEnabled: !output?.autoSpeakEnabled });
    } catch {
      setError('Automatic spoken replies could not be updated. Please try again.');
    }
  };
  const toggleOutputConsent = async () => {
    if (!outputConsentCurrent) {
      openConsent('azure-speech');
      return;
    }
    setError('');
    try {
      await updateOutput({ consent: false });
    } catch {
      setError('Azure speech consent could not be revoked. Please try again.');
    }
  };

  const statusBadge = (enabled: boolean) => (
    <Badge variant={enabled ? 'default' : 'secondary'}>{enabled ? 'Enabled' : 'Not enabled'}</Badge>
  );

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldCheck className="h-4 w-4 text-primary" /> Voice &amp; AI privacy
          </CardTitle>
          <CardDescription>
            These permissions are independent. You can revoke them here at any time.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          {(privacyQuery.isError || outputQuery.isError) && (
            <p role="alert" className="text-sm text-destructive">Privacy settings could not be loaded.</p>
          )}
          <section className="space-y-4" aria-label="AI and voice permissions">
            <PreferenceRow
              title="Recorded voice transcription"
              description="Allows AssemblyAI to transcribe voice input in Assistant, Planning, Notes, and uploads."
              enabled={Boolean(privacy?.recordedVoiceInputConsentGiven)}
              onEnable={() => openConsent('recorded-voice')}
              onRevoke={() => void revokePrivacy('recordedVoiceInput')}
              busy={privacyMutation.isPending}
              statusBadge={statusBadge}
            />
            <PreferenceRow
              title="Assistant processing"
              description="Allows user-submitted text and approved voice transcripts to reach Askolo’s AI provider. Workspace changes still require separate confirmation."
              enabled={Boolean(privacy?.assistantProcessingConsentGiven)}
              onEnable={() => openConsent('assistant-processing')}
              onRevoke={() => void revokePrivacy('assistantProcessing')}
              busy={privacyMutation.isPending}
              statusBadge={statusBadge}
            />
            <div className="border-t pt-4">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <h3 className="text-sm font-medium">Transcript redaction location</h3>
                  <p className="text-sm text-muted-foreground">
                    Only app-side filtering is available. Full transcripts remain editable; detected high-risk values are filtered before Assistant processing.
                  </p>
                  <p className="text-xs font-medium" role="status">
                    {privacy?.redactionLocation === 'app' ? 'App-side redaction selected' : 'Choose app-side redaction before enabling Assistant processing'}
                  </p>
                </div>
                {privacy?.redactionLocation !== 'app' && (
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={privacyMutation.isPending || privacyQuery.isLoading}
                    onClick={() => void updatePrivacy({ redactionLocation: 'app' }).catch(() => setError('App-side redaction could not be selected. Please try again.'))}
                  >
                    Select app-side
                  </Button>
                )}
              </div>
            </div>
            <PreferenceRow
              title="AssemblyAI Live Mode"
              description="Separate permission for live microphone audio and managed spoken responses. Development Live Mode may proceed while provider privacy checks remain pending; your separate permission is still required."
              enabled={Boolean(privacy?.assemblyAiLiveConsentGiven)}
              onEnable={() => openConsent('assemblyai-live')}
              onRevoke={() => void revokePrivacy('assemblyAiLive')}
              busy={privacyMutation.isPending}
              statusBadge={(enabled) => (
                <Badge variant={enabled && privacy?.assemblyAiLiveAvailable ? 'default' : 'secondary'}>
                  {enabled
                    ? privacy?.assemblyAiLiveAvailable ? 'Enabled' : 'Permission saved; unavailable'
                    : 'Not enabled'}
                </Badge>
              )}
            />
            <div className="border-t pt-4">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="text-sm font-medium">Azure speech output</h3>
                    {statusBadge(outputConsentCurrent)}
                  </div>
                  <p className="text-sm text-muted-foreground">
                    Sends Assistant replies to Azure when you choose Listen. Automatic spoken replies are a separate opt-in.
                  </p>
                  <Button
                    type="button"
                    size="sm"
                    variant="link"
                    className="h-auto p-0"
                    disabled={outputMutation.isPending}
                    onClick={() => void toggleOutputConsent()}
                  >
                    {outputConsentCurrent ? 'Revoke Azure speech consent' : 'Review Azure speech consent'}
                  </Button>
                </div>
                <div className="flex items-center gap-3">
                  <Label htmlFor="assistant-auto-speak" className="text-sm">Speak completed replies automatically</Label>
                  <Checkbox
                    id="assistant-auto-speak"
                    checked={Boolean(output?.autoSpeakEnabled)}
                    disabled={outputMutation.isPending || outputQuery.isLoading}
                    onCheckedChange={() => void toggleAutoSpeak()}
                    aria-describedby="assistant-auto-speak-help"
                  />
                </div>
              </div>
              <p id="assistant-auto-speak-help" className="mt-2 text-xs text-muted-foreground">
                Turn this off to stop automatic playback. You can still choose Listen for individual replies.
              </p>
            </div>
          </section>
          {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        </CardContent>
      </Card>

      <AIConsentDialog
        purpose={purpose}
        open={purpose !== null}
        redactionConfirmed={redactionConfirmed}
        autoSpeakAfterConsent={autoSpeakAfterConsent}
        saving={saving}
        error={error}
        onOpenChange={closeConsent}
        onRedactionConfirmedChange={setRedactionConfirmed}
        onConfirm={() => void confirmConsent()}
      />
    </>
  );
}

function PreferenceRow({
  title,
  description,
  enabled,
  onEnable,
  onRevoke,
  busy,
  statusBadge,
}: {
  title: string;
  description: string;
  enabled: boolean;
  onEnable: () => void;
  onRevoke: () => void;
  busy: boolean;
  statusBadge: (enabled: boolean) => ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3 border-b pb-4 last:border-b-0 last:pb-0 sm:flex-row sm:items-start sm:justify-between">
      <div className="space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="text-sm font-medium">{title}</h3>
          {statusBadge(enabled)}
        </div>
        <p className="max-w-3xl text-sm text-muted-foreground">{description}</p>
      </div>
      <Button type="button" size="sm" variant="outline" disabled={busy} onClick={enabled ? onRevoke : onEnable}>
        {enabled ? 'Revoke permission' : 'Review permission'}
      </Button>
    </div>
  );
}

export function AIConsentDialog({
  purpose,
  open,
  redactionConfirmed,
  autoSpeakAfterConsent,
  saving,
  error,
  onOpenChange,
  onRedactionConfirmedChange,
  onConfirm,
}: {
  purpose: ConsentPurpose | null;
  open: boolean;
  redactionConfirmed: boolean;
  autoSpeakAfterConsent?: boolean;
  saving: boolean;
  error?: string;
  onOpenChange: (open: boolean) => void;
  onRedactionConfirmedChange?: (confirmed: boolean) => void;
  onConfirm: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{purpose ? purposeCopy[purpose].title : 'Privacy permission'}</DialogTitle>
          <DialogDescription>{purpose ? purposeCopy[purpose].description : ''}</DialogDescription>
        </DialogHeader>
        {purpose === 'assistant-processing' && (
          <div className="flex items-start gap-3 rounded-md border p-3">
            <Checkbox
              id="confirm-app-side-redaction"
              checked={redactionConfirmed}
              onCheckedChange={(checked) => onRedactionConfirmedChange?.(checked === true)}
            />
            <Label htmlFor="confirm-app-side-redaction" className="text-sm leading-relaxed">
              Use app-side transcript redaction before sending content to the Assistant provider.
            </Label>
          </div>
        )}
        {purpose === 'azure-speech' && autoSpeakAfterConsent && (
          <p className="text-sm font-medium">
            You also chose to automatically send each completed Assistant reply to Azure for speech.
          </p>
        )}
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <DialogFooter>
          <Button type="button" variant="outline" disabled={saving} onClick={() => onOpenChange(false)}>Not now</Button>
          <Button
            type="button"
            disabled={saving || (purpose === 'assistant-processing' && !redactionConfirmed)}
            onClick={onConfirm}
          >
            {saving ? 'Saving…' : 'I understand and continue'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
