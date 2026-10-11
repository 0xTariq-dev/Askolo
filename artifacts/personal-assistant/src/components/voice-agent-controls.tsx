import { useEffect, useRef, useState } from 'react';
import { Loader2, Mic, PhoneCall, Square } from 'lucide-react';
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
import { useToast } from '@/hooks/use-toast';
import { useVoiceAgent } from '@/hooks/use-voice-agent';
import { formatUsdMicros } from '@/lib/credit-api';

export function VoiceAgentControls({
  voiceChatSelected = false,
  onActiveChange,
  compact = false,
  hideProcessingLocationNotice = false,
  freshConversation = false,
}: {
  voiceChatSelected?: boolean;
  onActiveChange?: (active: boolean) => void;
  compact?: boolean;
  hideProcessingLocationNotice?: boolean;
  freshConversation?: boolean;
}) {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const conversationQuery = useGetAssistantConversation({
    query: { queryKey: getGetAssistantConversationQueryKey(), enabled: !freshConversation },
  });
  const { data: privacyPreferences } = useGetAIPrivacyPreferences();
  const updatePrivacyPreferences = useUpdateAIPrivacyPreferences();
  const voiceAgent = useVoiceAgent();
  const [consentOpen, setConsentOpen] = useState(false);
  const [consentSaving, setConsentSaving] = useState(false);
  const [consentError, setConsentError] = useState('');
  const [startAfterConsent, setStartAfterConsent] = useState(false);
  const previousLiveConsentRef = useRef<boolean | null>(null);
  const liveVoiceLocales: Record<string, { locale: string; language: string }> = {
    michael: { locale: 'en', language: 'US English' },
    mary: { locale: 'en', language: 'US English' },
    paul: { locale: 'en', language: 'UK English' },
    vera: { locale: 'en', language: 'UK English' },
    giovanni: { locale: 'it', language: 'Italian' },
    lola: { locale: 'es', language: 'Spanish' },
    juergen: { locale: 'de', language: 'German' },
    rafael: { locale: 'pt', language: 'Portuguese' },
    estelle: { locale: 'fr', language: 'French' },
  };
  const selectedVoice = privacyPreferences?.assemblyAiLiveVoice
    ? liveVoiceLocales[privacyPreferences.assemblyAiLiveVoice]
    : undefined;
  const voiceUnavailable = !selectedVoice;
  const liveModeUnavailable = !privacyPreferences?.assemblyAiLiveAvailable;
  const isBusy = voiceAgent.status !== 'idle';

  useEffect(() => {
    onActiveChange?.(isBusy);
  }, [isBusy, onActiveChange]);

  useEffect(() => {
    const consentGiven = privacyPreferences?.assemblyAiLiveConsentGiven;
    if (consentGiven === undefined) return;

    const wasGranted = previousLiveConsentRef.current;
    previousLiveConsentRef.current = consentGiven;
    if (
      wasGranted === true &&
      !consentGiven &&
      voiceAgent.status !== 'idle' &&
      voiceAgent.status !== 'stopping'
    ) {
      voiceAgent.stop();
      toast({
        title: 'Live Mode ended',
        description: 'Your AssemblyAI Live Mode permission was revoked.',
      });
    }
  }, [privacyPreferences?.assemblyAiLiveConsentGiven, toast, voiceAgent.status, voiceAgent.stop]);

  const startLiveMode = () => {
    if (voiceChatSelected || !selectedVoice || isBusy || liveModeUnavailable) return;
    if (!privacyPreferences?.assemblyAiLiveConsentGiven) {
      setConsentError('');
      setStartAfterConsent(true);
      setConsentOpen(true);
      return;
    }
    void voiceAgent.start(freshConversation ? undefined : conversationQuery.data?.conversationId, selectedVoice.locale);
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
        if (voiceChatSelected) {
            toast({
              title: 'Switch modes to start Live Mode',
              description: 'End Voice Chat before starting Live Mode.',
            });
          return;
        }
        if (selectedVoice) {
          void voiceAgent.start(freshConversation ? undefined : conversationQuery.data?.conversationId, selectedVoice.locale);
        }
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
        ? voiceAgent.notice || 'Live Mode is on. Speak naturally.'
        : '';

  return (
    <section className={compact ? 'flex items-center justify-end' : 'rounded-xl border border-border bg-card/70 p-3'} aria-label="Live voice conversation">
      <div className="flex flex-wrap items-center gap-2">
        {isBusy ? (
          <Button type="button" variant="destructive" size={compact ? 'icon' : 'sm'} className={compact ? 'h-9 w-9 rounded-full' : undefined} onClick={() => voiceAgent.stop()} disabled={voiceAgent.status === 'stopping'} aria-label={compact ? 'End Live Mode' : undefined}>
            {voiceAgent.status === 'stopping' ? <Loader2 className={compact ? 'size-4 animate-spin' : 'mr-2 h-4 w-4 animate-spin'} aria-hidden="true" /> : <Square className={compact ? 'size-3.5' : 'mr-2 h-3.5 w-3.5'} aria-hidden="true" />}
            {!compact && (voiceAgent.status === 'stopping' ? 'Ending Live Mode…' : 'End Live Mode')}
          </Button>
        ) : (
          <Button
            type="button"
            variant="outline"
            size={compact ? 'icon' : 'sm'}
            className={compact ? 'h-9 w-9 rounded-full' : undefined}
            onClick={startLiveMode}
            disabled={voiceUnavailable || liveModeUnavailable || voiceChatSelected}
            aria-label={compact ? 'Start Live Mode' : undefined}
            title={compact ? 'Live Mode' : undefined}
          >
            {compact ? <PhoneCall className="size-4" aria-hidden="true" /> : <><Mic className="mr-2 h-4 w-4" aria-hidden="true" />Start Live Mode</>}
          </Button>
        )}
        {!compact && <p className="min-w-0 flex-1 text-xs text-muted-foreground">
          {voiceChatSelected
            ? 'End Voice Chat before starting Live Mode.'
            : liveModeUnavailable
            ? 'Live Mode is currently unavailable in this environment.'
            : voiceUnavailable
              ? 'Choose a supported Live Mode voice in Settings before starting.'
              : `Speak in ${selectedVoice.language}. Your app language does not change this voice.`}
        </p>}
      </div>

      {statusText && <p className={compact ? 'sr-only' : 'mt-2 text-xs text-muted-foreground'} role="status" aria-live="polite">{statusText}</p>}
      {voiceAgent.error && <p className={compact ? 'sr-only' : 'mt-2 text-xs text-destructive'} role="alert">{voiceAgent.error}</p>}
      {voiceAgent.notice && voiceAgent.status !== 'live' && <p className={compact ? 'sr-only' : 'mt-2 text-xs text-muted-foreground'} role="status">{voiceAgent.notice}</p>}
      {!hideProcessingLocationNotice && <p className="mt-2 text-xs text-muted-foreground">
        AssemblyAI’s processing location and EU-only processing have not been verified.
      </p>}

      {!compact && voiceAgent.voiceEstimate && (
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
                <span className="font-medium">{transcript.role === 'user' ? 'You' : 'Assistant'}:</span>{' '}
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