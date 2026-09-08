import { useEffect, useRef, useState } from 'react';
import { StreamingTranscriber, type TurnEvent } from 'assemblyai';
import {
  useCreateRealtimeTranscriptionToken,
  useTranscribeAudio,
  type TranscriptionReviewSignal,
} from '@workspace/api-client-react';

export type VoiceState = 'idle' | 'starting' | 'listening' | 'processing' | 'review' | 'error';
export type VoiceMode = 'live' | 'recorded';

export interface UseVoiceTranscriptionOptions {
  language?: string;
  maxRecordingMs?: number;
  maxAudioBytes?: number;
  realtime?: boolean;
}

export interface CompletedVoiceRecording {
  blob: Blob;
  mimeType: string;
  durationMs: number;
  transcript: string;
}

export interface VoiceTranscriptionResult {
  state: VoiceState;
  mode: VoiceMode | null;
  status: string;
  error: string;
  transcript: string;
  liveText: string;
  reviewSignals: TranscriptionReviewSignal[];
  recordingSeconds: number;
  audioLevel: number;
  recording: CompletedVoiceRecording | null;
  isBusy: boolean;
  isListening: boolean;
  start: () => Promise<void>;
  stop: () => void;
  cancel: () => void;
  reset: () => void;
  retry: () => Promise<void>;
  clearRecording: () => void;
}

const DEFAULT_MAX_RECORDING_MS = 2 * 60 * 1000;
const DEFAULT_MAX_AUDIO_BYTES = 8 * 1024 * 1024;
const MIN_RECORDED_SPEECH_MS = 700;
const MIN_ACTIVE_AUDIO_MS = 140;
const ACTIVE_AUDIO_RMS_THRESHOLD = 0.008;
const LOW_CONFIDENCE_THRESHOLD = 0.78;

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

function isCriticalEntity(text: string): boolean {
  return /[\d@#$%]|(?:am|pm)$/i.test(text);
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

async function validateRecordedSpeech(blob: Blob, durationMs: number): Promise<boolean> {
  if (durationMs < MIN_RECORDED_SPEECH_MS) return false;

  const AudioContextCtor = window.AudioContext ??
    (window as Window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
  if (!AudioContextCtor) return false;

  let context: AudioContext | null = null;
  try {
    context = new AudioContextCtor();
    const audioBuffer = await context.decodeAudioData(await blob.arrayBuffer());
    const channel = audioBuffer.getChannelData(0);
    const windowSize = Math.max(1, Math.floor(audioBuffer.sampleRate * 0.02));
    let activeSamples = 0;
    let peakRms = 0;

    for (let offset = 0; offset < channel.length; offset += windowSize) {
      const end = Math.min(channel.length, offset + windowSize);
      let sumSquares = 0;
      for (let index = offset; index < end; index += 1) {
        sumSquares += channel[index] ** 2;
      }
      const rms = Math.sqrt(sumSquares / Math.max(1, end - offset));
      peakRms = Math.max(peakRms, rms);
      if (rms >= ACTIVE_AUDIO_RMS_THRESHOLD) activeSamples += end - offset;
    }

    const activeDurationMs = (activeSamples / audioBuffer.sampleRate) * 1000;
    return peakRms >= ACTIVE_AUDIO_RMS_THRESHOLD && activeDurationMs >= MIN_ACTIVE_AUDIO_MS;
  } catch {
    // Do not send an unvalidated recording if this browser cannot decode it.
    return false;
  } finally {
    if (context && context.state !== 'closed') await context.close();
  }
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

function downsampleToPcm16(input: Float32Array, inputRate: number, outputRate = 16_000): Int16Array {
  if (inputRate === outputRate) {
    const output = new Int16Array(input.length);
    for (let index = 0; index < input.length; index += 1) {
      const sample = Math.max(-1, Math.min(1, input[index]));
      output[index] = sample < 0 ? sample * 0x8000 : sample * 0x7fff;
    }
    return output;
  }

  const ratio = inputRate / outputRate;
  const outputLength = Math.floor(input.length / ratio);
  const output = new Int16Array(outputLength);
  for (let index = 0; index < outputLength; index += 1) {
    const sourceIndex = Math.floor(index * ratio);
    const sample = Math.max(-1, Math.min(1, input[sourceIndex]));
    output[index] = sample < 0 ? sample * 0x8000 : sample * 0x7fff;
  }
  return output;
}

export function useVoiceTranscription({
  language = 'en-US',
  maxRecordingMs = DEFAULT_MAX_RECORDING_MS,
  maxAudioBytes = DEFAULT_MAX_AUDIO_BYTES,
  realtime = false,
}: UseVoiceTranscriptionOptions = {}): VoiceTranscriptionResult {
  const transcribeAudio = useTranscribeAudio();
  const createRealtimeToken = useCreateRealtimeTranscriptionToken();
  const [state, setState] = useState<VoiceState>('idle');
  const [mode, setMode] = useState<VoiceMode | null>(null);
  const [status, setStatus] = useState('');
  const [error, setError] = useState('');
  const [transcript, setTranscript] = useState('');
  const [liveText, setLiveText] = useState('');
  const [reviewSignals, setReviewSignals] = useState<TranscriptionReviewSignal[]>([]);
  const [recordingSeconds, setRecordingSeconds] = useState(0);
  const [audioLevel, setAudioLevel] = useState(0);
  const [recording, setRecording] = useState<CompletedVoiceRecording | null>(null);

  const stateRef = useRef<VoiceState>('idle');
  const modeRef = useRef<VoiceMode | null>(null);
  const sessionRef = useRef(0);
  const cancelRequestedRef = useRef(false);
  const stopRequestedRef = useRef(false);
  const durationStopRequestedRef = useRef(false);
  const recordingStartedAtRef = useRef<number | null>(null);
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const mediaStreamRef = useRef<MediaStream | null>(null);
  const audioChunksRef = useRef<Blob[]>([]);
  const realtimeRef = useRef<StreamingTranscriber | null>(null);
  const audioContextRef = useRef<AudioContext | null>(null);
  const audioSourceRef = useRef<MediaStreamAudioSourceNode | null>(null);
  const audioProcessorRef = useRef<ScriptProcessorNode | null>(null);
  const analyserRef = useRef<AnalyserNode | null>(null);
  const visualizerFrameRef = useRef<number | null>(null);
  const finalTurnsRef = useRef<Map<number, string>>(new Map());
  const interimTurnRef = useRef('');
  const realtimeSignalsRef = useRef<TranscriptionReviewSignal[]>([]);

  const updateState = (next: VoiceState) => {
    stateRef.current = next;
    setState(next);
  };

  const updateMode = (next: VoiceMode | null) => {
    modeRef.current = next;
    setMode(next);
  };

  const cleanupMediaStream = () => {
    if (visualizerFrameRef.current !== null) {
      cancelAnimationFrame(visualizerFrameRef.current);
      visualizerFrameRef.current = null;
    }
    analyserRef.current = null;
    audioProcessorRef.current?.disconnect();
    audioSourceRef.current?.disconnect();
    audioProcessorRef.current = null;
    audioSourceRef.current = null;
    mediaStreamRef.current?.getTracks().forEach((track) => track.stop());
    mediaStreamRef.current = null;
    mediaRecorderRef.current = null;
    audioChunksRef.current = [];
    const context = audioContextRef.current;
    audioContextRef.current = null;
    if (context && context.state !== 'closed') void context.close();
    setAudioLevel(0);
  };

  const updateAudioLevel = () => {
    const analyser = analyserRef.current;
    if (!analyser) return;
    const samples = new Uint8Array(analyser.fftSize);
    analyser.getByteTimeDomainData(samples);
    let sumSquares = 0;
    for (const sample of samples) {
      const centered = (sample - 128) / 128;
      sumSquares += centered ** 2;
    }
    setAudioLevel(Math.min(1, Math.sqrt(sumSquares / samples.length) * 5));
    visualizerFrameRef.current = requestAnimationFrame(updateAudioLevel);
  };

  const refreshRealtimeText = () => {
    const finalText = removeRepeatedTail([...finalTurnsRef.current.values()].join(' '));
    setTranscript(finalText);
    setLiveText(normalizeSpeech([finalText, interimTurnRef.current].filter(Boolean).join(' ')));
    setReviewSignals(realtimeSignalsRef.current);
  };

  const collectRealtimeTurn = (turn: TurnEvent) => {
    const spokenText = normalizeSpeech(turn.transcript ?? '');
    if (!spokenText) return;
    if (turn.end_of_turn) {
      if (!finalTurnsRef.current.has(turn.turn_order)) {
        finalTurnsRef.current.set(turn.turn_order, spokenText);
      }
      interimTurnRef.current = '';
    } else {
      interimTurnRef.current = spokenText;
    }

    for (const word of turn.words ?? []) {
      const text = normalizeSpeech(word.text ?? '');
      if (text && word.confidence < LOW_CONFIDENCE_THRESHOLD && isCriticalEntity(text)) {
        const signal = {
          kind: 'low_confidence_entity' as const,
          text,
          confidence: word.confidence,
          startMs: word.start ?? null,
          endMs: word.end ?? null,
        };
        if (!realtimeSignalsRef.current.some((item) => item.startMs === signal.startMs && item.text === signal.text)) {
          realtimeSignalsRef.current = [...realtimeSignalsRef.current, signal].slice(0, 20);
        }
      }
    }
    refreshRealtimeText();
  };

  const finishReview = (sessionId: number) => {
    if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
    const spokenText = removeRepeatedTail(transcript || liveText);
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
  };

  const startRecordedSession = async (sessionId: number) => {
    if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') {
      updateState('error');
      setError('This browser cannot record audio. You can type instead.');
      return;
    }
    const mimeType = getRecorderMimeType();
    if (!mimeType) {
      updateState('error');
      setError('This browser has no supported audio recording format.');
      return;
    }

    updateMode('recorded');
    updateState('starting');
    setStatus('Requesting microphone permission…');
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
       if (sessionRef.current !== sessionId || cancelRequestedRef.current || stopRequestedRef.current) {
        stream.getTracks().forEach((track) => track.stop());
         updateState('idle');
         setStatus('');
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
      const AudioContextCtor = window.AudioContext ??
        (window as Window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
      if (AudioContextCtor) {
        const context = new AudioContextCtor();
        const source = context.createMediaStreamSource(stream);
        const analyser = context.createAnalyser();
        analyser.fftSize = 256;
        analyser.smoothingTimeConstant = 0.75;
        source.connect(analyser);
        audioContextRef.current = context;
        audioSourceRef.current = source;
        analyserRef.current = analyser;
        void context.resume();
        visualizerFrameRef.current = requestAnimationFrame(updateAudioLevel);
      }
      recorder.onstop = async () => {
        const canceled = cancelRequestedRef.current || sessionRef.current !== sessionId;
        const recordedDurationMs = Math.min(
          maxRecordingMs,
          Math.max(1, Date.now() - (recordingStartedAtRef.current ?? Date.now())),
        );
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
        if (!(await validateRecordedSpeech(blob, recordedDurationMs))) {
          updateState('error');
          setError('This recording is too short or contains no clear speech. Hold to record while speaking, then release.');
          setStatus('');
          return;
        }

        updateState('processing');
        setStatus('Transcribing with AssemblyAI…');
        try {
          const audioBase64 = await blobToBase64(blob);
          if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
          transcribeAudio.mutate(
            {
              data: {
                audioBase64,
                mimeType: (blob.type || mimeType).split(';')[0] as
                  'audio/webm' | 'audio/mp4' | 'audio/m4a' | 'audio/wav' | 'audio/ogg' | 'audio/mpeg',
                durationMs: recordedDurationMs,
                language,
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
                setReviewSignals(data.reviewSignals);
                 setRecording({
                   blob,
                   mimeType: blob.type || mimeType,
                   durationMs: recordedDurationMs,
                   transcript: spokenText,
                 });
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
      recordingStartedAtRef.current = Date.now();
      recorder.start(1000);
      updateState('listening');
      setStatus('Speak naturally. Your audio is sent to AssemblyAI for reviewable transcription.');
    } catch (captureError) {
      const denied = captureError instanceof DOMException &&
        (captureError.name === 'NotAllowedError' || captureError.name === 'SecurityError');
      updateState('error');
      setError(denied
        ? 'Microphone permission was denied. Allow microphone access or type instead.'
        : 'The microphone could not be started. Check browser permissions and try again.');
      setStatus('');
    }
  };

  const closeRealtime = async (waitForTermination: boolean) => {
    cleanupMediaStream();
    const transcriber = realtimeRef.current;
    realtimeRef.current = null;
    if (transcriber) {
      try {
        await transcriber.close(waitForTermination, 3_000);
      } catch {
        // The SDK closes the socket even when the provider does not send its
        // termination acknowledgement. Never leave an abandoned socket open.
      }
    }
  };

  const startRealtimeSession = async (sessionId: number) => {
    if (!navigator.mediaDevices?.getUserMedia || typeof AudioContext === 'undefined') {
      updateState('error');
      setError('Real-time transcription is not supported in this browser. Use recorded transcription instead.');
      return;
    }
    updateMode('live');
    updateState('starting');
    setStatus('Requesting microphone permission…');
    try {
      const token = await createRealtimeToken.mutateAsync();
      if (sessionRef.current !== sessionId || cancelRequestedRef.current || token.region !== 'us') return;
      const stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true } });
      if (sessionRef.current !== sessionId || cancelRequestedRef.current) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }
      const transcriber = new StreamingTranscriber({
        token: token.token,
        websocketBaseUrl: token.websocketUrl,
        speechModel: token.speechModel as 'universal-3-5-pro',
        sampleRate: 16_000,
        includePartialTurns: true,
        redactPii: true,
        maxConnectionRetries: 0,
      });
      transcriber.on('turn', collectRealtimeTurn);
      transcriber.on('error', () => {
        if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
        updateState('error');
        setError('Real-time transcription stopped unexpectedly. Try recorded transcription instead.');
        setStatus('');
        void closeRealtime(false);
      });
      transcriber.on('close', () => {
        if (sessionRef.current !== sessionId || cancelRequestedRef.current || stopRequestedRef.current) return;
        updateState('error');
        setError('The real-time transcription session ended unexpectedly.');
        setStatus('');
      });
      realtimeRef.current = transcriber;
      await transcriber.connect();

      const AudioContextCtor = window.AudioContext ??
        (window as Window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
      if (!AudioContextCtor) throw new Error('A Web Audio context is required.');
      const context = new AudioContextCtor();
      const source = context.createMediaStreamSource(stream);
      const processor = context.createScriptProcessor(4096, 1, 1);
      processor.onaudioprocess = (event) => {
        if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
        const input = event.inputBuffer.getChannelData(0);
        let sumSquares = 0;
        for (const sample of input) sumSquares += sample ** 2;
        setAudioLevel(Math.min(1, Math.sqrt(sumSquares / Math.max(1, input.length)) * 5));
        const pcm = downsampleToPcm16(input, context.sampleRate);
        if (pcm.length > 0) realtimeRef.current?.sendAudio(new Uint8Array(pcm).buffer);
      };
      source.connect(processor);
      processor.connect(context.destination);
      mediaStreamRef.current = stream;
      audioContextRef.current = context;
      audioSourceRef.current = source;
      audioProcessorRef.current = processor;
      updateState('listening');
      setStatus('Live US transcription is active. Review the final text before submitting it.');
    } catch (captureError) {
      await closeRealtime(false);
      if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
      updateState('error');
      setError(captureError instanceof Error
        ? captureError.message
        : 'Real-time transcription could not be started. Try recorded transcription instead.');
      setStatus('');
    }
  };

  const start = async () => {
    if (stateRef.current === 'starting' || stateRef.current === 'listening' || stateRef.current === 'processing') return;
    const sessionId = sessionRef.current + 1;
    sessionRef.current = sessionId;
    cancelRequestedRef.current = false;
    stopRequestedRef.current = false;
    durationStopRequestedRef.current = false;
    finalTurnsRef.current = new Map();
    interimTurnRef.current = '';
    realtimeSignalsRef.current = [];
    setTranscript('');
    setLiveText('');
    setReviewSignals([]);
    setRecordingSeconds(0);
    setAudioLevel(0);
    setRecording(null);
    setError('');
    setStatus('');
    if (realtime) {
      await startRealtimeSession(sessionId);
    } else {
      await startRecordedSession(sessionId);
    }
  };

  const stop = () => {
    if (stateRef.current === 'starting') {
      stopRequestedRef.current = true;
      setStatus('Canceling before the microphone opens…');
      return;
    }
    if (stateRef.current !== 'listening') return;
    stopRequestedRef.current = true;
    setStatus('Finishing your recording…');
    if (modeRef.current === 'live') {
      updateState('processing');
      void closeRealtime(true).then(() => finishReview(sessionRef.current));
      return;
    }
    mediaRecorderRef.current?.stop();
  };

  const cancel = () => {
    cancelRequestedRef.current = true;
    sessionRef.current += 1;
    if (realtimeRef.current) void closeRealtime(true);
    if (mediaRecorderRef.current && mediaRecorderRef.current.state !== 'inactive') {
      mediaRecorderRef.current.stop();
    }
    cleanupMediaStream();
    setTranscript('');
    setLiveText('');
    setReviewSignals([]);
    updateState('idle');
    updateMode(null);
    setStatus('Voice input canceled.');
    setError('');
    setRecordingSeconds(0);
    setAudioLevel(0);
  };

  const reset = () => {
    cancelRequestedRef.current = true;
    sessionRef.current += 1;
    if (realtimeRef.current) void closeRealtime(true);
    if (mediaRecorderRef.current && mediaRecorderRef.current.state !== 'inactive') {
      mediaRecorderRef.current.stop();
    }
    cleanupMediaStream();
    setTranscript('');
    setLiveText('');
    setReviewSignals([]);
    updateState('idle');
    updateMode(null);
    setStatus('');
    setError('');
    setRecordingSeconds(0);
    setAudioLevel(0);
    setRecording(null);
  };

  const retry = async () => {
    if (!recording) return;
    updateState('processing');
    setError('');
    setStatus('Transcribing the recording again…');
    try {
      const audioBase64 = await blobToBase64(recording.blob);
      const data = await transcribeAudio.mutateAsync({
        data: {
          audioBase64,
          mimeType: recording.mimeType.split(';')[0] as
            'audio/webm' | 'audio/mp4' | 'audio/m4a' | 'audio/wav' | 'audio/ogg' | 'audio/mpeg',
          durationMs: recording.durationMs,
          language,
        },
      });
      const spokenText = removeRepeatedTail(data.transcript);
      setTranscript(spokenText);
      setLiveText(spokenText);
      setReviewSignals(data.reviewSignals);
      setRecording({ ...recording, transcript: spokenText });
      updateState('review');
      setStatus('Review the transcript before submitting it.');
    } catch (retryError) {
      updateState('error');
      setError(retryError instanceof Error ? retryError.message : 'Voice transcription failed. Try again.');
      setStatus('');
    }
  };

  const clearRecording = () => setRecording(null);

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
    cancelRequestedRef.current = true;
    realtimeRef.current?.close(true, 3_000).catch(() => undefined);
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
    reviewSignals,
    recordingSeconds,
    audioLevel,
    recording,
    isBusy,
    isListening: state === 'listening',
    start,
    stop,
    cancel,
    reset,
    retry,
    clearRecording,
  };
}