import { useEffect, useRef, useState } from 'react';
import { useTranscribeAudio } from '@workspace/api-client-react';

export type VoiceState = 'idle' | 'starting' | 'listening' | 'processing' | 'review' | 'error';
export type VoiceMode = 'live' | 'recorded';

export interface UseVoiceTranscriptionOptions {
  language?: string;
  maxRecordingMs?: number;
  maxAudioBytes?: number;
}

export interface VoiceTranscriptionResult {
  state: VoiceState;
  mode: VoiceMode | null;
  status: string;
  error: string;
  transcript: string;
  liveText: string;
  recordingSeconds: number;
  isBusy: boolean;
  isListening: boolean;
  start: () => Promise<void>;
  stop: () => void;
  cancel: () => void;
  reset: () => void;
}

const DEFAULT_MAX_RECORDING_MS = 2 * 60 * 1000;
const DEFAULT_MAX_AUDIO_BYTES = 8 * 1024 * 1024;

function normalizeSpeech(value: string): string {
  return value.replace(/\s+/g, ' ').trim();
}

function removeRepeatedTail(value: string): string {
  const words = normalizeSpeech(value).split(' ').filter(Boolean);
  const maxRepeatedWords = Math.min(12, Math.floor(words.length / 2));

  for (let size = maxRepeatedWords; size >= 5; size -= 1) {
    const previous = words.slice(words.length - size * 2, words.length - size);
    const last = words.slice(words.length - size);
    if (previous.length === size && previous.join(' ') === last.join(' ')) {
      return words.slice(0, words.length - size).join(' ');
    }
  }

  return words.join(' ');
}

function getRecorderMimeType(): string | null {
  if (typeof MediaRecorder === 'undefined') return null;
  const supportedTypes = [
    'audio/webm;codecs=opus',
    'audio/webm',
    'audio/mp4',
    'audio/ogg;codecs=opus',
    'audio/mpeg',
  ];
  return supportedTypes.find((type) => MediaRecorder.isTypeSupported(type)) ?? null;
}

async function blobToBase64(blob: Blob): Promise<string> {
  const buffer = await blob.arrayBuffer();
  const bytes = new Uint8Array(buffer);
  let binary = '';
  const chunkSize = 0x8000;
  for (let index = 0; index < bytes.length; index += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(index, index + chunkSize));
  }
  return btoa(binary);
}

export function useVoiceTranscription({
  language = 'en-US',
  maxRecordingMs = DEFAULT_MAX_RECORDING_MS,
  maxAudioBytes = DEFAULT_MAX_AUDIO_BYTES,
}: UseVoiceTranscriptionOptions = {}): VoiceTranscriptionResult {
  const transcribeAudio = useTranscribeAudio();
  const [state, setState] = useState<VoiceState>('idle');
  const [mode, setMode] = useState<VoiceMode | null>(null);
  const [status, setStatus] = useState('');
  const [error, setError] = useState('');
  const [transcript, setTranscript] = useState('');
  const [liveText, setLiveText] = useState('');
  const [recordingSeconds, setRecordingSeconds] = useState(0);

  const stateRef = useRef<VoiceState>('idle');
  const modeRef = useRef<VoiceMode | null>(null);
  const recognitionRef = useRef<any>(null);
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const mediaStreamRef = useRef<MediaStream | null>(null);
  const audioChunksRef = useRef<Blob[]>([]);
  const liveSegmentsRef = useRef<Array<{ text: string; isFinal: boolean }>>([]);
  const sessionRef = useRef(0);
  const cancelRequestedRef = useRef(false);
  const stopRequestedRef = useRef(false);
  const durationStopRequestedRef = useRef(false);

  const updateState = (next: VoiceState) => {
    stateRef.current = next;
    setState(next);
  };

  const updateMode = (next: VoiceMode | null) => {
    modeRef.current = next;
    setMode(next);
  };

  const cleanupMediaStream = () => {
    mediaStreamRef.current?.getTracks().forEach((track) => track.stop());
    mediaStreamRef.current = null;
    mediaRecorderRef.current = null;
    audioChunksRef.current = [];
  };

  const refreshLiveText = () => {
    const finalText = removeRepeatedTail(
      liveSegmentsRef.current.filter((segment) => segment.isFinal).map((segment) => segment.text).join(' '),
    );
    const interimText = normalizeSpeech(
      liveSegmentsRef.current.filter((segment) => !segment.isFinal).map((segment) => segment.text).join(' '),
    );
    setTranscript(finalText);
    setLiveText(normalizeSpeech([finalText, interimText].filter(Boolean).join(' ')));
  };

  const finishLiveReview = (sessionId: number) => {
    if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
    const finalText = removeRepeatedTail(
      liveSegmentsRef.current.filter((segment) => segment.isFinal).map((segment) => segment.text).join(' '),
    );
    if (!finalText) {
      updateState('error');
      setError('No speech was detected. Try again or type instead.');
      setStatus('');
      return;
    }
    setTranscript(finalText);
    setLiveText(finalText);
    updateState('review');
    setStatus('Review the transcript before submitting it.');
  };

  const startRecordedSession = async (sessionId: number) => {
    if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') {
      updateState('error');
      setError('This browser cannot record audio. You can type instead.');
      setStatus('');
      return;
    }

    const mimeType = getRecorderMimeType();
    if (!mimeType) {
      updateState('error');
      setError('This browser has no supported audio recording format.');
      setStatus('');
      return;
    }

    updateMode('recorded');
    updateState('starting');
    setStatus('Requesting microphone permission…');

    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      if (sessionRef.current !== sessionId || cancelRequestedRef.current) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }

      const recorder = new MediaRecorder(stream, { mimeType });
      mediaStreamRef.current = stream;
      mediaRecorderRef.current = recorder;
      audioChunksRef.current = [];

      recorder.ondataavailable = (event) => {
        if (event.data.size > 0) audioChunksRef.current.push(event.data);
      };
      recorder.onerror = () => {
        cleanupMediaStream();
        updateState('error');
        setError('The browser could not record this voice note. Try again or type instead.');
        setStatus('');
      };
      recorder.onstop = async () => {
        const canceled = cancelRequestedRef.current || sessionRef.current !== sessionId;
        const blob = new Blob(audioChunksRef.current, { type: recorder.mimeType || mimeType });
        cleanupMediaStream();
        if (canceled) return;

        if (blob.size === 0) {
          updateState('error');
          setError('No audio was recorded. Check microphone permission and try again.');
          setStatus('');
          return;
        }
        if (blob.size > maxAudioBytes) {
          updateState('error');
          setError('This recording is too large. Keep voice notes under 2 minutes.');
          setStatus('');
          return;
        }

        updateState('processing');
        setStatus('Transcribing your recording…');
        try {
          const audioBase64 = await blobToBase64(blob);
          if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
          transcribeAudio.mutate(
            {
              data: {
                audioBase64,
                mimeType: (blob.type || mimeType).split(';')[0] as
                  'audio/webm' | 'audio/mp4' | 'audio/m4a' | 'audio/wav' | 'audio/ogg' | 'audio/mpeg',
              },
            },
            {
              onSuccess: (data) => {
                if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
                const spokenText = removeRepeatedTail(data.transcript);
                if (!spokenText) {
                  updateState('error');
                  setError('No speech was detected. Try again or type instead.');
                  setStatus('');
                  return;
                }
                setTranscript(spokenText);
                setLiveText(spokenText);
                updateState('review');
                setStatus('Review the transcript before submitting it.');
              },
              onError: (mutationError) => {
                if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
                const message = mutationError instanceof Error ? mutationError.message : '';
                updateState('error');
                setError(message || 'Voice transcription failed. Try again or type instead.');
                setStatus('');
              },
            },
          );
        } catch {
          updateState('error');
          setError('The recording could not be prepared. Try again or type instead.');
          setStatus('');
        }
      };

      recorder.start(1000);
      updateState('listening');
      setStatus('Speak naturally. Your transcript will be reviewable before submission.');
    } catch (captureError) {
      const denied = captureError instanceof DOMException &&
        (captureError.name === 'NotAllowedError' || captureError.name === 'SecurityError');
      updateState('error');
      setError(
        denied
          ? 'Microphone permission was denied. Allow microphone access or type instead.'
          : 'The microphone could not be started. Check browser permissions and try again.',
      );
      setStatus('');
    }
  };

  const start = async () => {
    if (stateRef.current === 'starting' || stateRef.current === 'listening' || stateRef.current === 'processing') {
      return;
    }

    const sessionId = sessionRef.current + 1;
    sessionRef.current = sessionId;
    cancelRequestedRef.current = false;
    stopRequestedRef.current = false;
    durationStopRequestedRef.current = false;
    liveSegmentsRef.current = [];
    audioChunksRef.current = [];
    setTranscript('');
    setLiveText('');
    setRecordingSeconds(0);
    setError('');
    setStatus('');

    const SpeechRecognition = (window as any).SpeechRecognition || (window as any).webkitSpeechRecognition;
    if (!SpeechRecognition) {
      await startRecordedSession(sessionId);
      return;
    }

    try {
      const recognition = new SpeechRecognition();
      recognition.continuous = true;
      recognition.interimResults = true;
      recognition.lang = language;
      recognition.onresult = (event: any) => {
        if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
        for (let index = 0; index < event.results.length; index += 1) {
          const result = event.results[index];
          liveSegmentsRef.current[index] = {
            text: result[0]?.transcript ?? '',
            isFinal: Boolean(result.isFinal),
          };
        }
        refreshLiveText();
      };
      recognition.onerror = (event: any) => {
        if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
        const recoverable = event.error === 'network' ||
          event.error === 'service-not-allowed' ||
          event.error === 'audio-capture';
        if (recoverable && !stopRequestedRef.current) {
          recognition.abort();
          const fallbackSessionId = sessionRef.current + 1;
          sessionRef.current = fallbackSessionId;
          void startRecordedSession(fallbackSessionId);
          return;
        }
        if (event.error === 'no-speech') {
          setStatus('No speech detected yet. Keep speaking or stop to review.');
          return;
        }
        if (event.error === 'not-allowed') {
          updateState('error');
          setError('Microphone permission was denied. Allow microphone access or type instead.');
          setStatus('');
          return;
        }
        updateState('error');
        setError('Live transcription stopped unexpectedly. Try again or type instead.');
        setStatus('');
      };
      recognition.onend = () => {
        if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
        recognitionRef.current = null;
        finishLiveReview(sessionId);
      };
      recognitionRef.current = recognition;
      updateMode('live');
      updateState('listening');
      setStatus('Speak naturally. Your transcript will be reviewable before submission.');
      recognition.start();
    } catch {
      recognitionRef.current = null;
      await startRecordedSession(sessionId);
    }
  };

  const stop = () => {
    if (stateRef.current !== 'listening') return;
    stopRequestedRef.current = true;
    setStatus('Finishing your recording…');
    if (modeRef.current === 'live') {
      updateState('processing');
      recognitionRef.current?.stop();
      return;
    }
    mediaRecorderRef.current?.stop();
  };

  const cancel = () => {
    cancelRequestedRef.current = true;
    sessionRef.current += 1;
    recognitionRef.current?.abort();
    if (mediaRecorderRef.current && mediaRecorderRef.current.state !== 'inactive') {
      mediaRecorderRef.current.stop();
    }
    cleanupMediaStream();
    setTranscript('');
    setLiveText('');
    updateState('idle');
    updateMode(null);
    setStatus('Voice input canceled.');
    setError('');
    setRecordingSeconds(0);
  };

  const reset = () => {
    cancelRequestedRef.current = true;
    sessionRef.current += 1;
    recognitionRef.current?.abort();
    if (mediaRecorderRef.current && mediaRecorderRef.current.state !== 'inactive') {
      mediaRecorderRef.current.stop();
    }
    cleanupMediaStream();
    setTranscript('');
    setLiveText('');
    updateState('idle');
    updateMode(null);
    setStatus('');
    setError('');
    setRecordingSeconds(0);
  };

  useEffect(() => {
    stateRef.current = state;
  }, [state]);

  useEffect(() => {
    if (state !== 'listening') return;
    const timer = window.setInterval(() => {
      setRecordingSeconds((current) => {
        const next = current + 1;
        if (next >= maxRecordingMs / 1000 && !durationStopRequestedRef.current) {
          durationStopRequestedRef.current = true;
          window.setTimeout(stop, 0);
        }
        return next;
      });
    }, 1000);
    return () => window.clearInterval(timer);
  }, [maxRecordingMs, state]);

  useEffect(() => () => {
    recognitionRef.current?.abort();
    cleanupMediaStream();
  }, []);

  const isBusy = state === 'starting' || state === 'listening' || state === 'processing';
  return {
    state,
    mode,
    status,
    error,
    transcript,
    liveText,
    recordingSeconds,
    isBusy,
    isListening: state === 'listening',
    start,
    stop,
    cancel,
    reset,
  };
}