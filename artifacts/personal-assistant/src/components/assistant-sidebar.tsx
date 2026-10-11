import { useEffect, useRef, useState } from 'react';
import { motion, AnimatePresence, useReducedMotion } from 'framer-motion';
import { useForm } from 'react-hook-form';
import { useQueryClient } from '@tanstack/react-query';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import {
  Send,
  Sparkles,
  User,
  Bot,
  X,
  Maximize2,
  Minimize2,
  MessageSquare,
  Check,
  Loader2,
  AudioLines,
} from 'lucide-react';
import {
  confirmAssistantRun,
  cancelAssistantRun,
  useGetAssistantConversation,
  getGetAssistantConversationQueryKey,
  getGetDashboardSummaryQueryKey,
  getListActionItemsQueryKey,
  type AssistantRun,
  useGetTranscriptionPreferences,
  useUpdateTranscriptionPreferences,
  getGetTranscriptionPreferencesQueryKey,
  getGetAIPrivacyPreferencesQueryKey,
  useGetAIPrivacyPreferences,
  useUpdateAIPrivacyPreferences,
  useGetVoiceOutputPreferences,
  useUpdateVoiceOutputPreferences,
  getGetVoiceOutputPreferencesQueryKey,
} from '@workspace/api-client-react';
import { Button } from '@/components/ui/button';
import { Form, FormControl, FormField, FormItem } from '@/components/ui/form';
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from '@/components/ui/input-group';
import { Bubble, BubbleContent } from '@/components/ui/bubble';
import { Message, MessageAvatar, MessageContent, MessageFooter, MessageHeader } from '@/components/ui/message';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { useAssistantState, type ChatMessage } from '@/contexts/assistant-context';
import { useSpaceScope } from '@/contexts/space-context';
import { createAssistantRunOverWebSocket } from '@/lib/assistant-run-websocket';
import { creditApi, creditErrorMessage, formatUsdMicros, newCreditIdempotencyKey, type CreditEstimate } from '@/lib/credit-api';
import { useVoiceTranscription } from '@/hooks/use-voice-transcription';
import { useLocalVoiceActivity } from '@/hooks/use-local-voice-activity';
import {
  VoiceCaptureButton,
  VoiceCaptureFeedback,
  VoiceTranscriptReview,
} from '@/components/voice/voice-session-ui';
import {
  canAutoSubmitAssistantVoiceTranscript,
  getReviewedVoiceValue,
} from '@/lib/voice-flow';
import { VoiceConsentDialog } from '@/components/voice-consent-dialog';
import { CURRENT_VOICE_CONSENT_VERSION } from '@/lib/voice-consent';
import { getVoiceConsentErrorMessage } from '@/lib/voice-consent-errors';
import { useLocale } from '@/contexts/locale-context';
import { useAssistantSpeech } from '@/hooks/use-assistant-speech';
import { AIConsentDialog } from '@/components/settings/ai-privacy-center';
import { CURRENT_VOICE_OUTPUT_CONSENT_VERSION } from '@/lib/voice-output-consent';
import { useToast } from '@/hooks/use-toast';
import { VoiceAgentControls } from '@/components/voice-agent-controls';
import { VoiceChatControls } from '@/components/assistant/voice-chat-controls';

const messageSchema = z.object({ text: z.string().min(1) });
type MessageForm = z.infer<typeof messageSchema>;

export function AssistantSidebar({ fullPage = false }: { fullPage?: boolean }) {
  const {
    isOpen,
    isFull,
    messages,
    setMessages,
    toggle,
    close,
    toggleFull,
    draft,
    clearDraft,
  } = useAssistantState();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const bottomRef = useRef<HTMLDivElement>(null);
  const followConversationRef = useRef(true);
  const planningRequestRef = useRef<AbortController | null>(null);
  const reducedMotion = useReducedMotion();
  const { direction, locale } = useLocale();
  const { selectedSpaceId } = useSpaceScope();
  const hiddenOffset = direction === 'rtl' ? '-100%' : '100%';
  const [isThinking, setIsThinking] = useState(false);
  const [pendingActionId, setPendingActionId] = useState<string | null>(null);
  const [assistantError, setAssistantError] = useState('');
  const [assistantEstimate, setAssistantEstimate] = useState<CreditEstimate | null>(null);
  const [voiceConsentOpen, setVoiceConsentOpen] = useState(false);
  const [voiceConsentSaving, setVoiceConsentSaving] = useState(false);
  const [voiceConsentError, setVoiceConsentError] = useState('');
  const [voiceOutputConsentOpen, setVoiceOutputConsentOpen] = useState(false);
  const [voiceOutputConsentSaving, setVoiceOutputConsentSaving] = useState(false);
  const [voiceOutputConsentError, setVoiceOutputConsentError] = useState('');
  const [assistantConsentOpen, setAssistantConsentOpen] = useState(false);
  const [assistantConsentSaving, setAssistantConsentSaving] = useState(false);
  const [assistantConsentError, setAssistantConsentError] = useState('');
  const [assistantRedactionConfirmed, setAssistantRedactionConfirmed] = useState(false);
  const [voiceReviewText, setVoiceReviewText] = useState('');
  const [voiceApplied, setVoiceApplied] = useState(false);
  const [assistantMode, setAssistantMode] = useState<'chat' | 'voice-chat'>('chat');
  const [liveCallActive, setLiveCallActive] = useState(false);
  const [voiceChatError, setVoiceChatError] = useState('');
  const [voiceChatStatus, setVoiceChatStatus] = useState('');
  const [voiceChatSpeaking, setVoiceChatSpeaking] = useState(false);
  const [voiceChatSpeechError, setVoiceChatSpeechError] = useState(false);
  const assistantVoiceAutoSendRef = useRef(false);
  const assistantConsentApprovedRef = useRef(false);
  const pendingAssistantTextRef = useRef<string | null>(null);
  const pendingAssistantVoiceChatRef = useRef(false);
  const pendingVoiceChatStartRef = useRef(false);
  const pendingVoiceChatTranscriptsRef = useRef<string[]>([]);
  const voiceChatTurnRef = useRef(false);
  const assistantModeRef = useRef(assistantMode);
  assistantModeRef.current = assistantMode;
  const voiceChatActiveRef = useRef(false);
  const voiceChatCurrentRunIdRef = useRef('');
  const voiceChatIgnoredSpeechRunIdsRef = useRef(new Set<string>());
  const voiceChatAudioQueueRef = useRef<Array<{ runId: string; mediaType: string; data: string }>>([]);
  const voiceChatAudioRef = useRef<HTMLAudioElement | null>(null);
  const voiceChatAudioUrlRef = useRef<string | null>(null);
  const voiceChatSpeakingRef = useRef(false);
  const playNextVoiceChatAudioRef = useRef<() => void>(() => undefined);
  const voiceActivityStartRef = useRef<(
    stream: MediaStream,
    preRollPcm: Int16Array,
    takeBufferedPcm: () => { samples: Int16Array; overflow: boolean },
  ) => void>(() => undefined);
  const voiceActivityEndRef = useRef<() => void>(() => undefined);
  const cleanupVoiceChatOnCloseRef = useRef<() => void>(() => undefined);
  const speechOutput = useAssistantSpeech();
  const { data: voicePreferences } = useGetTranscriptionPreferences();
  const updateVoicePreferences = useUpdateTranscriptionPreferences();
  const { data: aiPrivacyPreferences } = useGetAIPrivacyPreferences();
  const updateAIPrivacyPreferences = useUpdateAIPrivacyPreferences();
  const { data: voiceOutputPreferences } = useGetVoiceOutputPreferences();
  const updateVoiceOutputPreferences = useUpdateVoiceOutputPreferences();
  const voice = useVoiceTranscription();
  const localVoiceActivity = useLocalVoiceActivity({
    onSpeechStart: (stream, preRollPcm, takeBufferedPcm) =>
      voiceActivityStartRef.current(stream, preRollPcm, takeBufferedPcm),
    onSpeechEnd: () => voiceActivityEndRef.current(),
  });
  voiceChatActiveRef.current = localVoiceActivity.active;

  const clearVoiceChatAudio = () => {
    voiceChatAudioQueueRef.current = [];
    const audio = voiceChatAudioRef.current;
    voiceChatAudioRef.current = null;
    if (audio) {
      audio.onended = null;
      audio.onerror = null;
      audio.pause();
      audio.removeAttribute('src');
      audio.load();
    }
    if (voiceChatAudioUrlRef.current) {
      URL.revokeObjectURL(voiceChatAudioUrlRef.current);
      voiceChatAudioUrlRef.current = null;
    }
    voiceChatSpeakingRef.current = false;
    setVoiceChatSpeaking(false);
  };

  const playNextVoiceChatAudio = () => {
    if (voiceChatAudioRef.current) return;
    const next = voiceChatAudioQueueRef.current.shift();
    if (!next || assistantModeRef.current !== 'voice-chat' || !voiceChatActiveRef.current) {
      if (!next) {
        voiceChatSpeakingRef.current = false;
        setVoiceChatSpeaking(false);
      }
      return;
    }
    if (voiceChatIgnoredSpeechRunIdsRef.current.has(next.runId)) {
      playNextVoiceChatAudioRef.current();
      return;
    }
    try {
      const binary = window.atob(next.data);
      const bytes = new Uint8Array(binary.length);
      for (let index = 0; index < binary.length; index += 1) {
        bytes[index] = binary.charCodeAt(index);
      }
      const url = URL.createObjectURL(new Blob([bytes], { type: next.mediaType }));
      const audio = new Audio(url);
      audio.preload = 'auto';
      voiceChatAudioRef.current = audio;
      voiceChatAudioUrlRef.current = url;
      voiceChatCurrentRunIdRef.current = next.runId;
      voiceChatSpeakingRef.current = true;
      setVoiceChatSpeaking(true);
      const completeChunk = () => {
        if (voiceChatAudioRef.current !== audio) return;
        voiceChatAudioRef.current = null;
        if (voiceChatAudioUrlRef.current === url) {
          URL.revokeObjectURL(url);
          voiceChatAudioUrlRef.current = null;
        }
        if (voiceChatAudioQueueRef.current.length > 0) {
          playNextVoiceChatAudioRef.current();
        } else {
          voiceChatSpeakingRef.current = false;
          setVoiceChatSpeaking(false);
          setVoiceChatStatus('');
        }
      };
      audio.onended = completeChunk;
      audio.onerror = () => {
        if (voiceChatAudioRef.current !== audio) return;
        clearVoiceChatAudio();
        setVoiceChatSpeechError(true);
        setVoiceChatStatus('Spoken reply playback failed. The full reply is available to read or retry.');
      };
      void audio.play().catch(() => {
        if (voiceChatAudioRef.current !== audio) return;
        clearVoiceChatAudio();
        setVoiceChatSpeechError(true);
        setVoiceChatStatus('Spoken reply playback failed. The full reply is available to read or retry.');
      });
    } catch {
      setVoiceChatSpeechError(true);
      setVoiceChatStatus('Spoken reply playback failed. The full reply is available to read or retry.');
    }
  };
  playNextVoiceChatAudioRef.current = playNextVoiceChatAudio;

  const enqueueVoiceChatAudio = (runId: string, mediaType: string, data: string) => {
    if (
      assistantModeRef.current !== 'voice-chat' ||
      !voiceChatActiveRef.current ||
      voiceChatIgnoredSpeechRunIdsRef.current.has(runId)
    ) return;
    voiceChatCurrentRunIdRef.current = runId;
    voiceChatAudioQueueRef.current.push({ runId, mediaType, data });
    setVoiceChatSpeechError(false);
    playNextVoiceChatAudioRef.current();
  };

  const interruptVoiceChatSpeech = () => {
    const runId = voiceChatCurrentRunIdRef.current;
    if (runId) voiceChatIgnoredSpeechRunIdsRef.current.add(runId);
    clearVoiceChatAudio();
    speechOutput.stop();
    setVoiceChatStatus('Spoken reply interrupted. Continue speaking when ready.');
  };
  const assistantVisible = fullPage || isOpen;
  const conversationQuery = useGetAssistantConversation({
    query: {
      queryKey: getGetAssistantConversationQueryKey(),
      enabled: assistantVisible,
      staleTime: 0,
      refetchOnWindowFocus: true,
    },
  });

  const form = useForm<MessageForm>({
    resolver: zodResolver(messageSchema),
    defaultValues: { text: '' },
  });

  useEffect(() => {
    if (assistantVisible && messages.length > 0 && followConversationRef.current) {
      bottomRef.current?.scrollIntoView({ behavior: reducedMotion ? 'auto' : 'smooth', block: 'end' });
    }
  }, [messages, isThinking, assistantVisible, reducedMotion]);

  useEffect(() => {
    if (
      voiceOutputPreferences &&
      (!voiceOutputPreferences.consentGiven ||
        voiceOutputPreferences.consentVersion !== CURRENT_VOICE_OUTPUT_CONSENT_VERSION)
    ) {
      speechOutput.stop();
    }
  }, [speechOutput.stop, voiceOutputPreferences]);

  useEffect(() => () => planningRequestRef.current?.abort(), []);

  useEffect(() => {
    if (!assistantVisible) speechOutput.stop();
  }, [assistantVisible, speechOutput.stop]);

  useEffect(() => {
    if (!assistantVisible) {
      cleanupVoiceChatOnCloseRef.current();
      if (voice.state !== 'idle') voice.reset();
    }
  }, [assistantVisible, voice.state]);

  useEffect(() => {
    if (!assistantVisible || fullPage) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') close();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [close, assistantVisible, fullPage]);

  useEffect(() => {
    if (!conversationQuery.data) return;
    setMessages(
      conversationQuery.data.messages.length > 0
        ? conversationQuery.data.messages
        : [],
    );
  }, [conversationQuery.data, setMessages]);

  useEffect(() => {
    if (!draft) return;
    form.reset({ text: draft });
    clearDraft();
  }, [draft, clearDraft, form]);

  const incorporateRun = (run: AssistantRun) => {
    setMessages((previous) => {
      const updated = previous.map((message) =>
        message.runId === run.id
          ? {
              ...message,
              content: run.message || message.content,
              state: run.state,
              intent: run.intent ?? message.intent,
              intentSha256: run.intentSha256 ?? message.intentSha256,
              requiresConfirmation: run.requiresConfirmation,
              confirmationExpiresAt: run.confirmationExpiresAt ?? message.confirmationExpiresAt,
              result: run.result ?? message.result,
            }
          : message,
      );
      if (!run.message || updated.some((message) => message.runId === run.id && message.content === run.message)) {
        return updated;
      }
      return [
        ...updated,
        {
          id: `run-${run.id}-${run.updatedAt}`,
          role: 'assistant',
          content: run.message,
          runId: run.id,
          state: run.state,
          intent: run.intent,
          intentSha256: run.intentSha256,
          requiresConfirmation: run.requiresConfirmation,
          confirmationExpiresAt: run.confirmationExpiresAt,
          result: run.result,
        },
      ];
    });
  };

  useEffect(() => {
    const onVoiceAgentAction = (event: Event) => {
      const run = (event as CustomEvent<AssistantRun>).detail;
      if (!run || typeof run.id !== 'string' || typeof run.state !== 'string') return;
      incorporateRun(run);
      if (!assistantVisible) toggle();
    };
    window.addEventListener('askolo:voice-agent-action', onVoiceAgentAction);
    return () => window.removeEventListener('askolo:voice-agent-action', onVoiceAgentAction);
  }, [incorporateRun, assistantVisible, toggle]);

  const onSubmit = async (data: MessageForm, options?: { voiceChat?: boolean }) => {
    const text = data.text.trim();
    if (!text || isThinking || pendingActionId !== null || voice.isBusy) return;
    const assistantConsentCurrent = Boolean(
      aiPrivacyPreferences?.assistantProcessingConsentGiven &&
      aiPrivacyPreferences.redactionLocation === 'app',
    );
    if (!assistantConsentCurrent && !assistantConsentApprovedRef.current) {
      pendingAssistantTextRef.current = text;
      pendingAssistantVoiceChatRef.current = options?.voiceChat === true;
      setAssistantConsentError('');
      setAssistantRedactionConfirmed(false);
      setAssistantConsentOpen(true);
      return;
    }
    assistantConsentApprovedRef.current = false;

    setMessages((previous) => [
      ...previous,
      { role: 'user', content: text, id: `local-${Date.now()}` },
    ]);
    form.reset();
    setIsThinking(true);
    setAssistantError('');
    const controller = new AbortController();
    planningRequestRef.current = controller;

    try {
      const estimate = await creditApi.estimate('assistant', 1);
      setAssistantEstimate(estimate);
      if (controller.signal.aborted) {
        setAssistantError('Request stopped. No action was taken.');
        await queryClient.invalidateQueries({ queryKey: getGetAssistantConversationQueryKey() });
        return;
      }
      if (!estimate.canReserve) {
        throw new Error('There are not enough AI credits for this request.');
      }
      const run = await createAssistantRunOverWebSocket({
        workspaceId: selectedSpaceId ?? undefined,
        allSpaces: !selectedSpaceId,
        conversationId: conversationQuery.data?.conversationId,
        transcript: text,
        idempotencyKey: newCreditIdempotencyKey(),
        policyVersion: estimate.policyVersion,
        voiceChat: options?.voiceChat,
      }, controller.signal, {
        onReplyDelta: (runId, delta) => {
          voiceChatCurrentRunIdRef.current = runId;
          setMessages((previous) => {
            const index = previous.findIndex((message) => message.runId === runId);
            if (index < 0) {
              return [
                ...previous,
                { id: `stream-${runId}`, role: 'assistant', content: delta, runId },
              ];
            }
            return previous.map((message, messageIndex) => messageIndex === index
              ? { ...message, content: message.content + delta }
              : message);
          });
        },
        onSpeechChunk: options?.voiceChat ? enqueueVoiceChatAudio : undefined,
        onSpeechError: options?.voiceChat
          ? (runId) => {
              if (voiceChatIgnoredSpeechRunIdsRef.current.has(runId)) return;
              setVoiceChatSpeechError(true);
              setVoiceChatStatus('Spoken reply could not be generated. The full reply is available to read or retry.');
            }
          : undefined,
      });
      incorporateRun(run);
      await queryClient.invalidateQueries({ queryKey: getGetAssistantConversationQueryKey() });
    } catch (error) {
      if (controller.signal.aborted) {
        setAssistantError('Request stopped. No action was taken.');
        await queryClient.invalidateQueries({ queryKey: getGetAssistantConversationQueryKey() });
      } else {
        setAssistantError(
          creditErrorMessage(error, 'I could not complete that request. Please try again.'),
        );
        await queryClient.invalidateQueries({ queryKey: getGetAssistantConversationQueryKey() });
      }
    } finally {
      if (planningRequestRef.current === controller) planningRequestRef.current = null;
      setIsThinking(false);
    }
  };

  const onSubmitRef = useRef(onSubmit);
  onSubmitRef.current = onSubmit;
  const resetVoiceRef = useRef(voice.reset);
  resetVoiceRef.current = voice.reset;

  const continueVoiceChatStart = (accepted?: {
    voiceInput?: boolean;
    assistantProcessing?: boolean;
    azureOutput?: boolean;
  }) => {
    if (!pendingVoiceChatStartRef.current) return;
    const hasVoiceInputConsent = accepted?.voiceInput === true || Boolean(
      voicePreferences?.consentGiven &&
      voicePreferences.consentVersion === CURRENT_VOICE_CONSENT_VERSION,
    );
    if (!hasVoiceInputConsent) {
      setVoiceConsentError('');
      setVoiceConsentOpen(true);
      return;
    }
    const hasAssistantProcessingConsent = accepted?.assistantProcessing === true || Boolean(
      aiPrivacyPreferences?.assistantProcessingConsentGiven &&
      aiPrivacyPreferences.redactionLocation === 'app',
    );
    if (!hasAssistantProcessingConsent) {
      setAssistantConsentError('');
      setAssistantRedactionConfirmed(false);
      setAssistantConsentOpen(true);
      return;
    }
    const hasAzureConsent = accepted?.azureOutput === true || Boolean(
      voiceOutputPreferences?.consentGiven &&
      voiceOutputPreferences.consentVersion === CURRENT_VOICE_OUTPUT_CONSENT_VERSION,
    );
    if (!hasAzureConsent) {
      setVoiceOutputConsentError('');
      setVoiceOutputConsentOpen(true);
      return;
    }
    pendingVoiceChatStartRef.current = false;
    setVoiceChatError('');
    setVoiceChatSpeechError(false);
    setVoiceChatStatus('Requesting microphone permission…');
    void localVoiceActivity.start();
  };

  const startVoiceChat = () => {
    if (liveCallActive) {
      setVoiceChatError('End Live Mode before starting Voice Chat.');
      return;
    }
    if (voice.isBusy || voice.state === 'review') {
      setVoiceChatError('Finish or cancel the current voice recording before starting Voice Chat.');
      return;
    }
    if (localVoiceActivity.active || localVoiceActivity.starting) return;
    setAssistantMode('voice-chat');
    setVoiceChatError('');
    setVoiceChatSpeechError(false);
    pendingVoiceChatStartRef.current = true;
    continueVoiceChatStart();
  };

  const takeQueuedVoiceMessages = () => {
    const pending = pendingVoiceChatTranscriptsRef.current.splice(0);
    if (pending.length === 0) return;
    const current = form.getValues('text');
    form.setValue('text', [current.trim(), ...pending].filter(Boolean).join('\n'), {
      shouldValidate: true,
    });
  };

  const endVoiceChat = () => {
    pendingVoiceChatStartRef.current = false;
    voiceChatActiveRef.current = false;
    takeQueuedVoiceMessages();
    localVoiceActivity.stop();
    if (voice.state !== 'idle') voice.cancel();
    voiceChatTurnRef.current = false;
    clearVoiceChatAudio();
    speechOutput.stop();
    setVoiceChatStatus('Voice Chat ended.');
  };

  const switchToNormalChat = () => {
    endVoiceChat();
    assistantModeRef.current = 'chat';
    setAssistantMode('chat');
    setVoiceChatError('');
    setVoiceChatSpeechError(false);
  };
  cleanupVoiceChatOnCloseRef.current = () => {
    pendingVoiceChatStartRef.current = false;
    voiceChatActiveRef.current = false;
    takeQueuedVoiceMessages();
    localVoiceActivity.stop();
    voiceChatTurnRef.current = false;
    clearVoiceChatAudio();
    speechOutput.stop();
    assistantModeRef.current = 'chat';
    setAssistantMode('chat');
  };

  voiceActivityStartRef.current = (stream, preRollPcm, takeBufferedPcm) => {
    if (assistantModeRef.current !== 'voice-chat') return;
    if (voiceChatSpeakingRef.current || speechOutput.status === 'speaking') {
      interruptVoiceChatSpeech();
    }
    if (voice.state === 'starting' || voice.state === 'listening' || voice.state === 'processing') return;
    setVoiceChatError('');
    setVoiceChatSpeechError(false);
    voiceChatTurnRef.current = true;
    setVoiceChatStatus('Speech detected. Connecting to AssemblyAI for this utterance…');
    void voice.start({
      voiceChat: true,
      stream,
      preRollPcm,
      takeBufferedPcm,
    });
  };
  voiceActivityEndRef.current = () => {
    if (voice.state === 'starting' || voice.state === 'listening') {
      voice.stop();
    }
  };

  const startAssistantVoice = async () => {
    if (!voicePreferences?.consentGiven || voicePreferences.consentVersion !== CURRENT_VOICE_CONSENT_VERSION) {
      setVoiceConsentError('');
      setVoiceConsentOpen(true);
      return;
    }
    setVoiceReviewText('');
    setVoiceApplied(false);
    assistantVoiceAutoSendRef.current = true;
    await voice.start();
  };

  useEffect(() => {
    if (voice.state === 'idle') {
      assistantVoiceAutoSendRef.current = false;
      return;
    }
    if (voice.state === 'error' && voiceChatTurnRef.current) {
      voiceChatTurnRef.current = false;
      setVoiceChatError(voice.error || 'Voice input could not be transcribed. Try again or switch to text chat.');
      return;
    }
    if (voice.state !== 'review' || !voice.transcript) return;

    if (voiceChatTurnRef.current) {
      voiceChatTurnRef.current = false;
      const transcript = voice.transcript.trim();
      resetVoiceRef.current();
      if (!transcript) {
        setVoiceChatError('No speech was detected. Try again or switch to text chat.');
        return;
      }
      if (isThinking || pendingActionId !== null) {
        if (pendingVoiceChatTranscriptsRef.current.length >= 3) {
        setVoiceChatError('Voice Chat can hold three spoken messages while the assistant is busy. Wait for a reply before speaking again.');
          return;
        }
        pendingVoiceChatTranscriptsRef.current.push(transcript);
      setVoiceChatStatus('Your spoken message is queued until the assistant finishes the current reply.');
        return;
      }
      setVoiceChatError('');
      void onSubmitRef.current({ text: transcript }, { voiceChat: true });
      return;
    }

    setVoiceReviewText(voice.transcript);
    setVoiceApplied(false);
    if (!assistantVoiceAutoSendRef.current) return;
    assistantVoiceAutoSendRef.current = false;

    if (!canAutoSubmitAssistantVoiceTranscript(
      voice.state,
      voice.transcript,
      voice.reviewSignals.length,
    )) return;

    const transcript = voice.transcript;
    resetVoiceRef.current();
    void onSubmitRef.current({ text: transcript });
  }, [isThinking, pendingActionId, voice.error, voice.reviewSignals, voice.state, voice.transcript]);

  useEffect(() => {
    if (
      assistantMode !== 'voice-chat' ||
      !localVoiceActivity.active ||
      isThinking ||
      voiceChatSpeaking ||
      voiceChatAudioQueueRef.current.length > 0 ||
      pendingActionId !== null
    ) return;
    const next = pendingVoiceChatTranscriptsRef.current.shift();
    if (!next) return;
    setVoiceChatStatus('Sending your queued voice message…');
    void onSubmitRef.current({ text: next }, { voiceChat: true });
  }, [assistantMode, isThinking, localVoiceActivity.active, pendingActionId, voiceChatSpeaking]);

  const applyAssistantTranscript = () => {
    if (!voiceReviewText.trim() || voiceApplied) return;
    const nextValue = getReviewedVoiceValue(form.getValues('text'), voiceReviewText, 'append');
    if (nextValue === null) return;
    form.setValue('text', nextValue, { shouldValidate: true });
    setVoiceApplied(true);
  };

  const sendReviewedAssistantTranscript = () => {
    const text = voiceReviewText.trim();
    if (!text || isThinking || pendingActionId !== null) return;
    resetVoiceRef.current();
    void onSubmitRef.current({ text });
  };

  const saveVoiceConsent = async () => {
    setVoiceConsentSaving(true);
    try {
      const updated = await updateVoicePreferences.mutateAsync({ data: { consent: true } });
      queryClient.setQueryData(getGetTranscriptionPreferencesQueryKey(), updated);
      setVoiceConsentOpen(false);
      continueVoiceChatStart({ voiceInput: true });
    } catch (error) {
      setVoiceConsentError(getVoiceConsentErrorMessage(error));
    } finally {
      setVoiceConsentSaving(false);
    }
  };

  const saveAssistantProcessingConsent = async () => {
    setAssistantConsentSaving(true);
    setAssistantConsentError('');
    try {
      const updated = await updateAIPrivacyPreferences.mutateAsync({
        data: { assistantProcessing: true, redactionLocation: 'app' },
      });
      queryClient.setQueryData(getGetAIPrivacyPreferencesQueryKey(), updated);
      assistantConsentApprovedRef.current = true;
      setAssistantConsentOpen(false);
      if (pendingVoiceChatStartRef.current) {
        continueVoiceChatStart({ assistantProcessing: true });
        return;
      }
      const pendingText = pendingAssistantTextRef.current;
      const pendingVoiceChat = pendingAssistantVoiceChatRef.current;
      pendingAssistantTextRef.current = null;
      pendingAssistantVoiceChatRef.current = false;
      if (pendingText) {
        void onSubmitRef.current({ text: pendingText }, pendingVoiceChat ? { voiceChat: true } : undefined);
      }
    } catch {
      setAssistantConsentError('Assistant processing permission could not be saved. Please try again.');
    } finally {
      setAssistantConsentSaving(false);
    }
  };

  const handleSpeechRequest = (message: ChatMessage) => {
    if (!message.runId) return;
    if (
      !voiceOutputPreferences?.consentGiven ||
      voiceOutputPreferences.consentVersion !== CURRENT_VOICE_OUTPUT_CONSENT_VERSION
    ) {
      setVoiceOutputConsentError('');
      setVoiceOutputConsentOpen(true);
      return;
    }
    void speechOutput.speak(message.id, message.content, { runId: message.runId, locale });
  };

  const saveVoiceOutputConsent = async () => {
    setVoiceOutputConsentSaving(true);
    try {
      const updated = await updateVoiceOutputPreferences.mutateAsync({
        data: { consent: true, autoSpeakEnabled: false },
      });
      queryClient.setQueryData(getGetVoiceOutputPreferencesQueryKey(), updated);
      setVoiceOutputConsentOpen(false);
      if (pendingVoiceChatStartRef.current) {
        continueVoiceChatStart({ azureOutput: true });
        return;
      }
      toast({
        title: locale === 'ar' ? 'تم حفظ موافقة الإخراج الصوتي' : 'Speech output permission saved',
        description: locale === 'ar'
          ? 'اختر «إعادة المحاولة» مرة أخرى لبدء التشغيل.'
          : 'Choose Retry speech again to start playback.',
      });
    } catch (error) {
      setVoiceOutputConsentError(getVoiceConsentErrorMessage(error));
    } finally {
      setVoiceOutputConsentSaving(false);
    }
  };

  const handleConfirm = async (message: ChatMessage) => {
    if (!message.runId || !message.intentSha256 || pendingActionId) return;
    setPendingActionId(message.runId);
    setAssistantError('');
    try {
      const run = await confirmAssistantRun(message.runId, {
        expectedIntentSHA256: message.intentSha256,
      });
      incorporateRun(run);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: getGetAssistantConversationQueryKey() }),
        queryClient.invalidateQueries({ queryKey: getListActionItemsQueryKey() }),
        queryClient.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() }),
      ]);
    } catch (error) {
      setAssistantError(
        creditErrorMessage(error, 'That action could not be confirmed. Review it and try again.'),
      );
      await queryClient.invalidateQueries({ queryKey: getGetAssistantConversationQueryKey() });
    } finally {
      setPendingActionId(null);
    }
  };

  const handleCancel = async (message: ChatMessage) => {
    if (!message.runId || !message.intentSha256 || pendingActionId) return;
    setPendingActionId(message.runId);
    setAssistantError('');
    try {
      const run = await cancelAssistantRun(message.runId, {
        expectedIntentSHA256: message.intentSha256,
      });
      incorporateRun(run);
      await queryClient.invalidateQueries({ queryKey: getGetAssistantConversationQueryKey() });
    } catch (error) {
      setAssistantError(
        creditErrorMessage(error, 'That pending action could not be dismissed. Try again.'),
      );
      await queryClient.invalidateQueries({ queryKey: getGetAssistantConversationQueryKey() });
    } finally {
      setPendingActionId(null);
    }
  };

  return (
    <>
      {!fullPage && <button
        onClick={toggle}
        aria-label={isOpen ? 'Close assistant' : 'Open assistant'}
        aria-expanded={isOpen}
        aria-controls="assistant-panel"
        className={cn(
          'fixed bottom-6 end-6 z-50 h-12 w-12 rounded-full shadow-lg flex items-center justify-center transition-colors',
          isOpen
            ? 'bg-primary/20 border border-primary/40 text-primary hover:bg-primary/30'
            : 'bg-primary text-primary-foreground hover:bg-primary/90',
        )}
      >
        {isOpen ? <X className="h-5 w-5" /> : <MessageSquare className="h-5 w-5" />}
      </button>}

      {/* Overlay backdrop for fullscreen */}
      <AnimatePresence>
        {!fullPage && isOpen && isFull && (
          <motion.div
            key="backdrop"
            initial={reducedMotion ? false : { opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={reducedMotion ? { duration: 0 } : undefined}
            className="fixed inset-0 z-40 bg-background/80"
            onClick={close}
          />
        )}
      </AnimatePresence>

      {/* Sidebar panel */}
      <AnimatePresence>
        {(fullPage || isOpen) && (
          <motion.aside
            key={fullPage ? 'workspace' : 'sidebar'}
            id="assistant-panel"
            aria-label="Assistant"
            initial={fullPage || reducedMotion ? { opacity: 1 } : { x: hiddenOffset }}
            animate={{ x: 0 }}
            exit={fullPage || reducedMotion ? { opacity: 0 } : { x: hiddenOffset }}
            transition={reducedMotion ? { duration: 0 } : { type: 'spring', damping: 28, stiffness: 300 }}
            className={cn(
              fullPage
                ? 'relative flex min-h-[min(72dvh,760px)] w-full flex-1 flex-col overflow-hidden bg-transparent'
                : cn(
                    'fixed top-0 end-0 bottom-0 z-50 flex flex-col bg-card border-s border-border shadow-2xl',
                    isFull ? 'w-full' : 'w-[380px] max-w-full',
                  ),
            )}
          >
            {/* Header */}
            <div className={cn('flex items-center justify-between border-b border-border shrink-0', fullPage ? 'px-1 py-3 md:px-2' : 'h-14 px-4')}>
              <div className="flex items-center gap-2">
                <div className={cn('rounded-lg bg-primary/10 border border-primary/20 flex items-center justify-center text-primary', fullPage ? 'h-9 w-9' : 'h-7 w-7')}>
                  <Sparkles className="h-3.5 w-3.5" />
                </div>
                <div>
                  <span className={cn('font-display font-semibold', fullPage ? 'text-lg' : 'text-sm')}>Assistant</span>
                </div>
              </div>
              {!fullPage && <div className="flex items-center gap-1">
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-10 w-10 text-muted-foreground hover:text-foreground"
                  onClick={toggleFull}
                  aria-label={isFull ? 'Collapse' : 'Expand to full screen'}
                >
                  {isFull ? <Minimize2 className="h-4 w-4" /> : <Maximize2 className="h-4 w-4" />}
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-10 w-10 text-muted-foreground hover:text-foreground"
                  onClick={close}
                  aria-label="Close assistant"
                >
                  <X className="h-4 w-4" />
                </Button>
              </div>}
            </div>

            {/* Messages */}
             <div
               onScroll={(event) => {
                 const element = event.currentTarget;
                 followConversationRef.current = element.scrollHeight - element.scrollTop - element.clientHeight < 96;
               }}
               className={cn('flex-1 overflow-y-auto', fullPage ? 'px-1 py-4 md:px-2 md:py-6' : 'p-4')}
               role="log"
               aria-label="Conversation"
               aria-live="polite"
               aria-relevant="additions text"
             >
              {conversationQuery.isError && (
                <p className="text-xs text-destructive" role="status">
                  Saved assistant history could not be loaded. New messages can still be sent.
                </p>
              )}
              <div className={cn('flex min-h-full flex-col gap-4', messages.length > 0 && 'justify-end')}>
              {messages.map((msg, idx) => (
                <motion.div
                  key={msg.id}
                  initial={reducedMotion ? false : { opacity: 0, y: 8 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={reducedMotion ? { duration: 0 } : { delay: idx === 0 ? 0 : 0.04 }}
                  className="w-full"
                >
                  <Message align={msg.role === 'user' ? 'end' : 'start'} className="gap-3">
                    <MessageAvatar className="size-8 w-8 self-start bg-transparent">
                      <Avatar className="size-8 border">
                        <AvatarFallback className={msg.role === 'user' ? 'bg-primary/15 text-primary' : 'bg-muted text-foreground'}>
                          {msg.role === 'user' ? <User className="size-4" aria-hidden="true" /> : <Bot className="size-4" aria-hidden="true" />}
                        </AvatarFallback>
                      </Avatar>
                    </MessageAvatar>
                    <MessageContent className={cn('max-w-[min(84%,760px)] gap-2.5', fullPage && 'md:max-w-[min(78%,820px)]')}>
                      <MessageHeader className="gap-1.5">
                        <span>{msg.role === 'user' ? 'You' : 'Assistant'}</span>
                        {msg.state && <><span aria-hidden="true" className="size-1 rounded-full bg-muted-foreground/40" /><span>{msg.state === 'planning' ? 'Preparing' : msg.state === 'completed' ? 'Ready' : ''}</span></>}
                      </MessageHeader>
                      <Bubble variant={msg.role === 'user' ? 'default' : 'muted'} className="max-w-full">
                        <BubbleContent><p dir="auto" className="whitespace-pre-wrap">{msg.content}</p>
                    {msg.role === 'assistant' &&
                      msg.state === 'needs_confirmation' &&
                      ['create_action_item', 'send_gmail', 'create_calendar_event'].includes(
                        msg.intent?.tool ?? '',
                      ) &&
                      msg.runId &&
                      msg.intentSha256 && (
                        <Card className="mt-3 border-primary/30 bg-background/80 shadow-none">
                          <CardContent className="space-y-2 p-3">
                          <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                            {msg.intent?.tool === 'send_gmail'
                              ? 'Email to send'
                              : msg.intent?.tool === 'create_calendar_event'
                                ? 'Calendar event to create'
                                : 'Action item to add'}
                          </p>
                          {msg.intent?.tool === 'send_gmail' ? (
                            <div className="space-y-2 text-sm">
                              <p className="break-words">
                                <span className="font-medium">Google account:</span>{' '}
                                {msg.intent.accountEmail || 'Connected Google account'}
                              </p>
                              <p dir="auto" className="break-words">
                                <span className="font-medium">To:</span> {msg.intent.to}
                              </p>
                              <p dir="auto" className="break-words">
                                <span className="font-medium">Subject:</span> {msg.intent.subject}
                              </p>
                              <div
                                dir="auto"
                                tabIndex={0}
                                aria-label="Email message to review"
                                className="max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-md border bg-background p-2 text-sm"
                              >
                                {msg.intent.body}
                              </div>
                            </div>
                          ) : msg.intent?.tool === 'create_calendar_event' ? (
                            <div className="space-y-1 text-sm">
                              <p className="break-words">
                                <span className="font-medium">Google account:</span>{' '}
                                {msg.intent.accountEmail || 'Connected Google account'}
                              </p>
                              <p dir="auto" className="break-words">
                                <span className="font-medium">Event:</span> {msg.intent.title}
                              </p>
                              <p className="break-words">
                                <span className="font-medium">Start:</span> {msg.intent.start}
                              </p>
                              <p className="break-words">
                                <span className="font-medium">End:</span> {msg.intent.end}
                              </p>
                              <p className="text-xs text-muted-foreground">
                                Times include their UTC offset. The event will be added to the primary calendar without guests.
                              </p>
                            </div>
                          ) : (
                            <p dir="auto" className="text-sm font-medium break-words">{msg.intent?.title}</p>
                          )}
                          {msg.confirmationExpiresAt &&
                          new Date(msg.confirmationExpiresAt).getTime() > Date.now() ? (
                            <div className="flex flex-wrap gap-2 pt-1">
                              <Button
                                type="button"
                                size="sm"
                                className="min-h-10"
                                disabled={pendingActionId !== null || isThinking}
                                onClick={() => void handleConfirm(msg)}
                                aria-label={
                                  msg.intent?.tool === 'send_gmail'
                                    ? 'Confirm sending this email'
                                    : msg.intent?.tool === 'create_calendar_event'
                                      ? 'Confirm creating this calendar event'
                                      : `Confirm adding ${msg.intent?.title ?? 'action item'}`
                                }
                              >
                                {pendingActionId === msg.runId ? (
                                  <Loader2 className="h-4 w-4 mr-1 animate-spin" />
                                ) : (
                                  <Check className="h-4 w-4 mr-1" />
                                )}
                                {msg.intent?.tool === 'send_gmail'
                                  ? 'Send email'
                                  : msg.intent?.tool === 'create_calendar_event'
                                    ? 'Create event'
                                    : 'Confirm add'}
                              </Button>
                              <Button
                                type="button"
                                size="sm"
                                variant="outline"
                                className="min-h-10"
                                disabled={pendingActionId !== null || isThinking}
                                onClick={() => void handleCancel(msg)}
                                aria-label={
                                  msg.intent?.tool === 'send_gmail'
                                    ? 'Dismiss sending this email'
                                    : msg.intent?.tool === 'create_calendar_event'
                                      ? 'Dismiss creating this calendar event'
                                      : `Dismiss adding ${msg.intent?.title ?? 'action item'}`
                                }
                              >
                                Not now
                              </Button>
                            </div>
                          ) : (
                            <p className="text-xs text-muted-foreground" role="status">
                              This confirmation has expired. Send the request again to review a new action.
                            </p>
                          )}
                          </CardContent>
                        </Card>
                      )}
                      </BubbleContent>
                      </Bubble>
                      {msg.role === 'assistant' && msg.runId && msg.state && <MessageFooter>{msg.state === 'needs_confirmation' ? 'Review required' : msg.state === 'completed' ? 'Complete' : ''}</MessageFooter>}
                    </MessageContent>
                  </Message>
                </motion.div>
              ))}
              {isThinking && (
                <motion.div
                  initial={reducedMotion ? false : { opacity: 0, y: 8 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={reducedMotion ? { duration: 0 } : undefined}
                  className="flex gap-2.5"
                >
                  <div className="h-7 w-7 rounded-full bg-muted border border-border flex items-center justify-center">
                    <Bot className="h-3.5 w-3.5" />
                  </div>
                  <div className="bg-muted border border-border rounded-2xl rounded-bl-sm px-3.5 py-2.5 flex items-center gap-1">
                    <span className="w-1.5 h-1.5 bg-muted-foreground rounded-full animate-bounce motion-reduce:animate-none" />
                    <span className="w-1.5 h-1.5 bg-muted-foreground rounded-full animate-bounce motion-reduce:animate-none [animation-delay:0.15s]" />
                    <span className="w-1.5 h-1.5 bg-muted-foreground rounded-full animate-bounce motion-reduce:animate-none [animation-delay:0.3s]" />
                  </div>
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    className="self-center min-h-10"
                    onClick={() => planningRequestRef.current?.abort()}
                  >
                    Stop
                  </Button>
                </motion.div>
              )}
              </div>
              <div ref={bottomRef} />
            </div>

            {/* Input */}
            <div className={cn('border-t border-border bg-card/50 shrink-0', fullPage ? 'p-4 md:px-7 md:py-5' : 'p-3')}>
              {liveCallActive && assistantMode !== 'voice-chat' && (
                <p className="mb-3 px-1 text-xs text-muted-foreground" role="status">
                  End Live Mode before starting Voice Chat.
                </p>
              )}
              {assistantMode === 'voice-chat' && (
                <div className="mb-3">
                  <VoiceChatControls
                    active={localVoiceActivity.active}
                    starting={localVoiceActivity.starting}
                    muted={localVoiceActivity.muted}
                    status={
                      voice.state === 'starting'
                        ? 'Connecting to AssemblyAI for this utterance…'
                        : voice.state === 'listening'
                          ? 'Transcribing this utterance…'
                          : voice.state === 'processing'
                            ? 'Finishing the transcript…'
                            : voiceChatSpeaking || speechOutput.status === 'speaking'
                                ? 'Assistant is speaking'
                              : isThinking
                                  ? 'Preparing a reply…'
                                : voiceChatStatus
                    }
                    error={voiceChatError || localVoiceActivity.error || (voice.state === 'error' ? voice.error : '')}
                    speechError={voiceChatSpeechError || speechOutput.status === 'error'}
                    speaking={voiceChatSpeaking || speechOutput.status === 'speaking'}
                    onStart={startVoiceChat}
                    onEnd={endVoiceChat}
                    onToggleMute={() => localVoiceActivity.setMuted(!localVoiceActivity.muted)}
                    onInterrupt={interruptVoiceChatSpeech}
                    onRetrySpeech={() => {
                      const lastAssistantMessage = [...messages].reverse().find((message) =>
                        message.role === 'assistant' &&
                        (message.state === 'completed' || message.state === 'needs_confirmation') &&
                        Boolean(message.runId) &&
                        Boolean(message.content.trim()),
                      );
                      if (!lastAssistantMessage) {
                        setVoiceChatSpeechError(true);
                        setVoiceChatStatus('There is no completed reply to retry yet.');
                        return;
                      }
                      setVoiceChatSpeechError(false);
                      handleSpeechRequest(lastAssistantMessage);
                    }}
                    onSwitchToChat={switchToNormalChat}
                  />
                </div>
              )}
              <Form {...form}>
                <form onSubmit={form.handleSubmit((data) => onSubmit(data))}>
                  <InputGroup className="h-14 rounded-full border-border bg-background p-1.5 shadow-sm">
                  <FormField
                    control={form.control}
                    name="text"
                    render={({ field }) => (
                      <FormItem className="min-w-0 flex-1">
                        <FormControl>
                          <InputGroupInput
                            placeholder="Write a message…"
                            aria-label="Message"
                            dir="auto"
                            className="h-11 min-w-0 px-2 text-sm"
                            {...field}
                            onKeyDown={(e) => {
                              if (e.key === 'Enter' && !e.shiftKey) {
                                e.preventDefault();
                                form.handleSubmit((data) => onSubmit(data))();
                              }
                            }}
                          />
                        </FormControl>
                      </FormItem>
                    )}
                  />
                  <InputGroupAddon align="inline-end" className="gap-1 border-0 pe-1">
                  {assistantMode === 'chat' && (
                    <VoiceCaptureButton
                      voice={voice}
                      onStart={startAssistantVoice}
                      startText="Press and hold or activate to record voice input"
                      stopText="Recording voice input; release or activate to stop"
                      variant="outline"
                      size="icon"
                      iconOnly
                      className="size-10 shrink-0 rounded-full"
                      disabled={voice.state === 'starting' || voice.state === 'processing' || isThinking || pendingActionId !== null}
                      data-testid="button-assistant-voice"
                    />
                  )}
                  <InputGroupButton
                    type="button"
                    size="icon-sm"
                    className={cn('size-10 rounded-full', assistantMode === 'voice-chat' && 'bg-primary/10 text-primary')}
                    aria-label={assistantMode === 'voice-chat' ? 'End Voice Chat' : 'Start Voice Chat'}
                    aria-pressed={assistantMode === 'voice-chat'}
                    disabled={voice.isBusy || voice.state === 'review' || liveCallActive}
                    onClick={() => assistantMode === 'voice-chat' && localVoiceActivity.active ? switchToNormalChat() : startVoiceChat()}
                  >
                    <AudioLines className="size-5" aria-hidden="true" />
                  </InputGroupButton>
                  {fullPage && <VoiceAgentControls
                    voiceChatSelected={assistantMode === 'voice-chat'}
                    onActiveChange={setLiveCallActive}
                    compact
                    hideProcessingLocationNotice
                  />}
                  <Button
                    type="submit"
                    size="icon"
                    className="size-10 shrink-0 rounded-full"
                     disabled={isThinking || pendingActionId !== null || voice.isBusy}
                    aria-label="Send message"
                  >
                    <Send className="h-4 w-4" />
                  </Button>
                  </InputGroupAddon>
                  </InputGroup>
                </form>
              </Form>
              {!fullPage && <div className="mt-2">
                <VoiceAgentControls
                  voiceChatSelected={assistantMode === 'voice-chat'}
                  onActiveChange={setLiveCallActive}
                />
              </div>}
              {assistantMode === 'chat' && (voice.state !== 'idle' || voice.status || voice.error || voice.recording) && (
                <div className="mt-2 space-y-2 px-1">
                  <VoiceCaptureFeedback voice={voice} />
                  <VoiceTranscriptReview
                    id="assistant-voice-transcript"
                    state={voice.state}
                    transcript={voice.transcript}
                    value={voiceReviewText}
                    onChange={setVoiceReviewText}
                    label="Editable message transcript"
                  >
                    {voice.reviewSignals.length > 0 && (
                      <p className="text-xs text-amber-700 dark:text-amber-300" role="status">
                        Transcription flagged possible uncertainty. Review and correct the text before sending.
                      </p>
                    )}
                    {voice.reviewSignals.length > 0 && (
                      <Button
                        type="button"
                        size="sm"
                        onClick={sendReviewedAssistantTranscript}
                        disabled={!voiceReviewText.trim() || isThinking || pendingActionId !== null}
                      >
                        Send corrected transcript
                      </Button>
                    )}
                    <Button type="button" size="sm" onClick={applyAssistantTranscript} disabled={!voiceReviewText.trim() || voiceApplied}>
                      {voiceApplied ? 'Added to message' : 'Use transcript in message'}
                    </Button>
                    <Button type="button" size="sm" variant="ghost" onClick={voice.reset}>
                      Discard transcript
                    </Button>
                  </VoiceTranscriptReview>
                </div>
              )}
              {assistantError && (
                <p className="mt-2 px-1 text-xs text-destructive" role="alert">
                  {assistantError}
                </p>
              )}
              {assistantEstimate && !isThinking && (
                <p className="mt-2 px-1 text-[11px] text-muted-foreground" data-testid="text-assistant-usage-estimate">
                  This request is estimated at <bdi dir="ltr">{formatUsdMicros(assistantEstimate.estimatedUsdMicros)}</bdi>, with a maximum reservation of <bdi dir="ltr">{formatUsdMicros(assistantEstimate.hardCapUsdMicros)}</bdi>. Available: <bdi dir="ltr">{formatUsdMicros(assistantEstimate.availableUsdMicros)}</bdi>.
                </p>
              )}
            </div>
          </motion.aside>
        )}
      </AnimatePresence>
      <VoiceConsentDialog
        open={voiceConsentOpen}
        onOpenChange={(open) => {
          setVoiceConsentOpen(open);
          if (!open && pendingVoiceChatStartRef.current) {
            pendingVoiceChatStartRef.current = false;
            setVoiceChatError('AssemblyAI microphone permission is required for Voice Chat. You can switch to text chat instead.');
          }
        }}
        onConfirm={() => void saveVoiceConsent()}
        saving={voiceConsentSaving}
        error={voiceConsentError}
      />
      <AIConsentDialog
        purpose={assistantConsentOpen ? 'assistant-processing' : null}
        open={assistantConsentOpen}
        redactionConfirmed={assistantRedactionConfirmed}
        saving={assistantConsentSaving}
        error={assistantConsentError}
        onOpenChange={(open) => {
          setAssistantConsentOpen(open);
          if (!open) {
            pendingAssistantTextRef.current = null;
            pendingAssistantVoiceChatRef.current = false;
            if (pendingVoiceChatStartRef.current) {
              pendingVoiceChatStartRef.current = false;
              setVoiceChatError('Assistant processing permission is required for Voice Chat. You can switch to text chat instead.');
            }
          }
        }}
        onRedactionConfirmedChange={setAssistantRedactionConfirmed}
        onConfirm={() => void saveAssistantProcessingConsent()}
      />
      <AIConsentDialog
        purpose={voiceOutputConsentOpen ? 'azure-speech' : null}
        open={voiceOutputConsentOpen}
        onOpenChange={(open) => {
          setVoiceOutputConsentOpen(open);
          if (!open && pendingVoiceChatStartRef.current) {
            pendingVoiceChatStartRef.current = false;
            setVoiceChatError('Azure speech permission was not granted. Voice Chat did not start; switch to text chat.');
          }
        }}
        redactionConfirmed={true}
        onConfirm={() => void saveVoiceOutputConsent()}
        saving={voiceOutputConsentSaving}
        error={voiceOutputConsentError}
      />
    </>
  );
}
