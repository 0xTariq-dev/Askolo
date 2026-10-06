import { useState } from 'react';
import { Loader2, Mic, Square } from 'lucide-react';
import {
  getGetAssistantConversationQueryKey,
  getGetAIPrivacyPreferencesQueryKey,
  useGetAssistantConversation,
  useGetAIPrivacyPreferences,
  useUpdateAIPrivacyPreferences,
} from '@workspace/api-client-react';
import { useQueryClient } from '@tanstack/react-query';
import { Button } from '@/components/ui/button';
import { AIConsentDialog } from '@/components/settings/ai-privacy-center';
import { useLocale } from '@/contexts/locale-context';
import { useToast } from '@/hooks/use-toast';
import { useVoiceAgent } from '@/hooks/use-voice-agent';
import { formatUsdMicros } from '@/lib/credit-api';

export function VoiceAgentControls() {
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const conversationQuery = useGetAssistantConversation({
    query: { queryKey: getGetAssistantConversationQueryKey() },
  });
  const { data: privacyPreferences } = useGetAIPrivacyPreferences();
  const updatePrivacyPreferences = useUpdateAIPrivacyPreferences();
  const voiceAgent = useVoiceAgent();
  const [consentOpen, setConsentOpen] = useState(false);
  const [consentSaving, setConsentSaving] = useState(false);
  const [consentError, setConsentError] = useState('');
  const [startAfterConsent, setStartAfterConsent] = useState(false);
  const englishOnly = !locale.toLowerCase().startsWith('en');
  const liveModeUnavailable = !privacyPreferences?.assemblyAiLiveAvailable;
  const languageNotice = locale.toLowerCase().startsWith('ar')
    ? 'يدعم الوضع المباشر التحدث باللغة الإنجليزية فقط حاليًا. غيّر لغة التطبيق إلى الإنجليزية لاستخدامه.'
    : 'Live Mode currently supports English speech only. Switch the app language to English to use it.';
  const isBusy = voiceAgent.status !== 'idle';

  const startLiveMode = () => {
    if (englishOnly || isBusy || liveModeUnavailable) return;
    if (!privacyPreferences?.assemblyAiLiveConsentGiven) {
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
      const updated = await updatePrivacyPreferences.mutateAsync({ data: { assemblyAiLive: true } });
      queryClient.setQueryData(getGetAIPrivacyPreferencesQueryKey(), updated);
      setConsentOpen(false);
      if (startAfterConsent) {
        setStartAfterConsent(false);
        void voiceAgent.start(conversationQuery.data?.conversationId);
      }
    } catch (error) {
      const message = 'Live Mode permission could not be saved. Please try again.';
      setConsentError(message);
      toast({ title: 'Live Mode permission could not be saved', description: message, variant: 'destructive' });
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
          <Button type="button" variant="outline" size="sm" onClick={startLiveMode} disabled={englishOnly || liveModeUnavailable}>
            <Mic className="mr-2 h-4 w-4" aria-hidden="true" />
            Start Live Mode
          </Button>
        )}
        <p className="min-w-0 flex-1 text-xs text-muted-foreground">
          {englishOnly
            ? languageNotice
            : liveModeUnavailable
              ? 'Live Mode is currently unavailable in this environment.'
              : 'Talk with Askolo. Workspace changes still require your confirmation.'}
        </p>
      </div>

      {statusText && <p className="mt-2 text-xs text-muted-foreground" role="status" aria-live="polite">{statusText}</p>}
      {voiceAgent.error && <p className="mt-2 text-xs text-destructive" role="alert">{voiceAgent.error}</p>}
      {voiceAgent.notice && voiceAgent.status !== 'live' && <p className="mt-2 text-xs text-muted-foreground" role="status">{voiceAgent.notice}</p>}
      <p className="mt-2 text-xs text-muted-foreground">
        AssemblyAI’s processing location and EU-only processing have not been verified.
      </p>

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
          {voiceAgent.transcripts.map((transcript) => (
            <li key={transcript.key} className="break-words">
              <span className="font-medium">{transcript.role === 'user' ? 'You' : 'Askolo'}:</span>{' '}
              <span>{transcript.text}</span>
              {transcript.interrupted && <span className="ms-2 text-xs text-muted-foreground">(interrupted)</span>}
            </li>
          ))}
        </ol>
      )}

      <AIConsentDialog
        purpose={consentOpen ? 'assemblyai-live' : null}
        open={consentOpen}
        redactionConfirmed={true}
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