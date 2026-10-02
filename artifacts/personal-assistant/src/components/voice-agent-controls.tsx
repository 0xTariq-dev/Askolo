import { useState } from 'react';
import { Loader2, Mic, Square } from 'lucide-react';
import {
  getGetAssistantConversationQueryKey,
  getGetTranscriptionPreferencesQueryKey,
  useGetAssistantConversation,
  useGetTranscriptionPreferences,
  useUpdateTranscriptionPreferences,
} from '@workspace/api-client-react';
import { useQueryClient } from '@tanstack/react-query';
import { Button } from '@/components/ui/button';
import { VoiceConsentDialog } from '@/components/voice-consent-dialog';
import { useLocale } from '@/contexts/locale-context';
import { useToast } from '@/hooks/use-toast';
import { useVoiceAgent } from '@/hooks/use-voice-agent';
import { CURRENT_VOICE_CONSENT_VERSION } from '@/lib/voice-consent';
import { getVoiceConsentErrorMessage } from '@/lib/voice-consent-errors';
import { formatUsdMicros } from '@/lib/credit-api';

export function VoiceAgentControls() {
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const conversationQuery = useGetAssistantConversation({
    query: { queryKey: getGetAssistantConversationQueryKey() },
  });
  const { data: transcriptionPreferences } = useGetTranscriptionPreferences();
  const updateTranscriptionPreferences = useUpdateTranscriptionPreferences();
  const voiceAgent = useVoiceAgent();
  const [consentOpen, setConsentOpen] = useState(false);
  const [consentSaving, setConsentSaving] = useState(false);
  const [consentError, setConsentError] = useState('');
  const [startAfterConsent, setStartAfterConsent] = useState(false);
  const englishOnly = !locale.toLowerCase().startsWith('en');
  const languageNotice = locale.toLowerCase().startsWith('ar')
    ? 'يدعم الوضع المباشر التحدث باللغة الإنجليزية فقط حاليًا. غيّر لغة التطبيق إلى الإنجليزية لاستخدامه.'
    : 'Live Mode currently supports English speech only. Switch the app language to English to use it.';
  const isBusy = voiceAgent.status !== 'idle';

  const startLiveMode = () => {
    if (englishOnly || isBusy) return;
    if (
      !transcriptionPreferences?.consentGiven ||
      transcriptionPreferences.consentVersion !== CURRENT_VOICE_CONSENT_VERSION
    ) {
      setConsentError('');
      setStartAfterConsent(true);
      setConsentOpen(true);
      return;
    }
    void voiceAgent.start(conversationQuery.data?.conversationId);
  };

  const saveConsent = async () => {
    setConsentSaving(true);
    setConsentError('');
    try {
      const updated = await updateTranscriptionPreferences.mutateAsync({ data: { consent: true } });
      queryClient.setQueryData(getGetTranscriptionPreferencesQueryKey(), updated);
      setConsentOpen(false);
      if (startAfterConsent) {
        setStartAfterConsent(false);
        void voiceAgent.start(conversationQuery.data?.conversationId);
      }
    } catch (error) {
      setConsentError(getVoiceConsentErrorMessage(error));
      toast({ title: 'Voice consent could not be saved', description: getVoiceConsentErrorMessage(error), variant: 'destructive' });
    } finally {
      setConsentSaving(false);
    }
  };

  const statusText = voiceAgent.status === 'connecting'
    ? 'Connecting to Live Mode…'
    : voiceAgent.status === 'stopping'
      ? 'Ending the session and requesting provider deletion…'
      : voiceAgent.status === 'live'
        ? voiceAgent.notice || 'Live Mode is on. Speak naturally; your audio is not saved in Askolo.'
        : '';

  return (
    <section className="rounded-xl border border-border bg-card/70 p-3" aria-label="Live voice conversation">
      <div className="flex flex-wrap items-center gap-2">
        {isBusy ? (
          <Button type="button" variant="destructive" size="sm" onClick={() => voiceAgent.stop()} disabled={voiceAgent.status === 'stopping'}>
            {voiceAgent.status === 'stopping' ? <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" /> : <Square className="mr-2 h-3.5 w-3.5" aria-hidden="true" />}
            {voiceAgent.status === 'stopping' ? 'Ending Live Mode…' : 'End Live Mode'}
          </Button>
        ) : (
          <Button type="button" variant="outline" size="sm" onClick={startLiveMode} disabled={englishOnly}>
            <Mic className="mr-2 h-4 w-4" aria-hidden="true" />
            Start Live Mode
          </Button>
        )}
        <p className="min-w-0 flex-1 text-xs text-muted-foreground">
          {englishOnly
            ? languageNotice
            : 'Talk with Askolo. Workspace changes still require your confirmation.'}
        </p>
      </div>

      {statusText && <p className="mt-2 text-xs text-muted-foreground" role="status" aria-live="polite">{statusText}</p>}
      {voiceAgent.error && <p className="mt-2 text-xs text-destructive" role="alert">{voiceAgent.error}</p>}
      {voiceAgent.notice && voiceAgent.status !== 'live' && <p className="mt-2 text-xs text-muted-foreground" role="status">{voiceAgent.notice}</p>}

      {voiceAgent.voiceEstimate && (
        <p className="mt-2 text-[11px] leading-relaxed text-muted-foreground">
          Maximum 180-second voice reservation: <bdi dir="ltr">{formatUsdMicros(voiceAgent.voiceEstimate.hardCapUsdMicros)}</bdi>
          {' '}(available: <bdi dir="ltr">{formatUsdMicros(voiceAgent.voiceEstimate.availableUsdMicros)}</bdi>).
          {voiceAgent.assistantEstimate
            ? <> Action preparation is separate, estimated up to <bdi dir="ltr">{formatUsdMicros(voiceAgent.assistantEstimate.hardCapUsdMicros)}</bdi>.</>
            : voiceAgent.assistantEstimateUnavailable
              ? ' Action preparation uses a separate credit rate and may be unavailable.'
              : null}
        </p>
      )}

      {voiceAgent.transcripts.length > 0 && (
        <ol className="mt-3 max-h-40 space-y-2 overflow-y-auto rounded-lg bg-muted/40 p-2 text-sm" aria-label="Live conversation transcript" aria-live="polite">
          {voiceAgent.transcripts.map((transcript, index) => (
            <li key={`${index}-${transcript.role}`} className="break-words">
              <span className="font-medium">{transcript.role === 'user' ? 'You' : 'Askolo'}:</span>{' '}
              <span>{transcript.text}</span>
            </li>
          ))}
        </ol>
      )}

      <VoiceConsentDialog
        open={consentOpen}
        onOpenChange={(open) => {
          setConsentOpen(open);
          if (!open) setStartAfterConsent(false);
        }}
        onConfirm={() => void saveConsent()}
        saving={consentSaving}
        error={consentError}
      />
    </section>
  );
}