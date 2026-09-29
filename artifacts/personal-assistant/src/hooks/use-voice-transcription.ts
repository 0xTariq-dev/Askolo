import { useEffect, useRef, useState } from 'react';
import {
  transcribeAudio as transcribeAudioRequest,
  type TranscriptionReviewSignal,
} from '@workspace/api-client-react';
import {
  creditApi,
  newCreditIdempotencyKey,
  type CreditEstimate,
  type CreditReceiptDetails,
} from '@/lib/credit-api';
import {
  parsePublicWebSocketEnvelope,
  publicWebSocketURL,
  sendPublicWebSocketAudio,
  sendPublicWebSocketMessage,
  waitForPublicWebSocketEvent,
} from '@/lib/public-websocket';

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

export interface VoiceDeletionStatus {
  rawAudio: 'not_stored';
  providerTranscript: 'deleted' | 'deletion_failed';
  marker: string;
}

export interface VoiceTranscriptionResult {
  state: VoiceState;
  mode: VoiceMode | null;
  status: string;
  error: string;
  transcript: string;
  liveText: string;
  reviewSignals: TranscriptionReviewSignal[];
  deletionStatus: VoiceDeletionStatus | null;
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
  transcribeFile: (file: File) => Promise<void>;
}

const DEFAULT_MAX_RECORDING_MS = 2 * 60 * 1000;
const DEFAULT_MAX_AUDIO_BYTES = 8 * 1024 * 1024;
const MIN_RECORDED_SPEECH_MS = 700;
const MIN_ACTIVE_AUDIO_MS = 140;
const ACTIVE_AUDIO_RMS_THRESHOLD = 0.008;
const LOW_CONFIDENCE_THRESHOLD = 0.78;
const DEFAULT_REALTIME_MAX_SESSION_SECONDS = 180;
const SUPPORTED_FILE_TYPES = new Set(['audio/webm', 'audio/mp4', 'audio/m4a', 'audio/wav', 'audio/ogg', 'audio/mpeg']);

interface RealtimeWordEvent {
  text?: string;
  confidence: number;
  start?: number;
  end?: number;
}

interface RealtimeTurnEvent {
  type?: string;
  turn_order: number;
  transcript: string;
  end_of_turn: boolean;
  words?: RealtimeWordEvent[];
}

interface RealtimeServerEvent {
  type?: string;
  code?: string;
  message?: string;
  maxSessionDurationSeconds?: number;
  creditReceipt?: CreditReceiptDetails;
  turn_order?: number;
  transcript?: string;
  end_of_turn?: boolean;
  words?: RealtimeWordEvent[];
}

function normalizeSpeech(value: string): string {
  return value.replace(/\s+/g, ' ').trim();
}

function waitForWebSocketOpen(socket: WebSocket, timeoutMs = 20_000): Promise<void> {
  return new Promise((resolve, reject) => {
    const cleanup = () => {
      window.clearTimeout(timeout);
      socket.removeEventListener('open', onOpen);
      socket.removeEventListener('error', onError);
      socket.removeEventListener('close', onClose);
    };
    const onOpen = () => {
      cleanup();
      resolve();
    };
    const onError = () => {
      cleanup();
      reject(new Error('Real-time transcription could not be started. Try recorded transcription instead.'));
    };
    const onClose = () => {
      cleanup();
      reject(new Error('The live transcription connection closed before it was ready.'));
    };
    const timeout = window.setTimeout(() => {
      cleanup();
      reject(new Error('Real-time transcription took too long to connect. Try again.'));
    }, timeoutMs);
    socket.addEventListener('open', onOpen, { once: true });
    socket.addEventListener('error', onError, { once: true });
    socket.addEventListener('close', onClose, { once: true });
  });
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

function assertVoiceReservationAllowed(estimate: CreditEstimate) {
  if (!estimate.canReserve) {
    throw new Error(
      `This voice request needs up to ${estimate.hardCapCredits} credits; ${estimate.availableCredits} are available. Open AI Credits to review your balance.`,
    );
  }
}

async function prepareVoiceCreditRequest() {
  const estimate = await creditApi.estimate('voice', 1);
  assertVoiceReservationAllowed(estimate);
  return {
    estimate,
    headers: {
      'Idempotency-Key': newCreditIdempotencyKey(),
      'X-AI-Credit-Policy-Version': String(estimate.policyVersion),
    },
  };
}

function creditPostflightMessage(receipt: CreditReceiptDetails | undefined, estimate: CreditEstimate): string {
  if (!receipt || typeof receipt.settledCredits !== 'number' || typeof receipt.balance !== 'number') {
    throw new Error('The operation completed but its credit receipt was missing. Refresh AI Credits before retrying.');
  }
  const returned = Math.max(0, receipt.reservedCredits - receipt.settledCredits);
  return `Estimated ${estimate.estimatedCredits} credit${estimate.estimatedCredits === 1 ? '' : 's'}; used ${receipt.settledCredits}. ${returned} returned. Balance: ${receipt.balance}.`;
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
  const [state, setState] = useState<VoiceState>('idle');
  const [mode, setMode] = useState<VoiceMode | null>(null);
  const [status, setStatus] = useState('');
  const [error, setError] = useState('');
  const [transcript, setTranscript] = useState('');
  const [liveText, setLiveText] = useState('');
  const [reviewSignals, setReviewSignals] = useState<TranscriptionReviewSignal[]>([]);
  const [deletionStatus, setDeletionStatus] = useState<VoiceDeletionStatus | null>(null);
  const [recordingSeconds, setRecordingSeconds] = useState(0);
  const [audioLevel, setAudioLevel] = useState(0);
  const [recording, setRecording] = useState<CompletedVoiceRecording | null>(null);
  const realtimeMaxSessionSecondsRef = useRef(DEFAULT_REALTIME_MAX_SESSION_SECONDS);

  const stateRef = useRef<VoiceState>('idle');
  const modeRef = useRef<VoiceMode | null>(null);
  const sessionRef = useRef(0);
  const cancelRequestedRef = useRef(false);
  const stopRequestedRef = useRef(false);
  const durationStopRequestedRef = useRef(false);
  const transcriptionAbortRef = useRef<AbortController | null>(null);
  const voiceCreditPostflightRef = useRef('');
  const recordingStartedAtRef = useRef<number | null>(null);
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const mediaStreamRef = useRef<MediaStream | null>(null);
  const audioChunksRef = useRef<Blob[]>([]);
  const realtimeRef = useRef<WebSocket | null>(null);
  const realtimeClientSequenceRef = useRef(0);
  const realtimeServerSequenceRef = useRef(0);
  const realtimeHeartbeatRef = useRef<number | null>(null);
  const pendingRealtimePcmRef = useRef(new Int16Array(0));
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

  const collectRealtimeTurn = (turn: RealtimeTurnEvent) => {
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
      setStatus(voiceCreditPostflightRef.current);
      return;
    }
    setTranscript(spokenText);
    setLiveText(spokenText);
    updateState('review');
    setStatus(`${voiceCreditPostflightRef.current ? `${voiceCreditPostflightRef.current} ` : ''}Review the transcript before submitting it.`);
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
    setStatus('Checking the voice credit estimate…');
    try {
      const preflight = await creditApi.estimate('voice', 1);
      assertVoiceReservationAllowed(preflight);
      setStatus(`Estimate: ${preflight.estimatedCredits} credits; maximum ${preflight.hardCapCredits}. Requesting microphone permission…`);
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
          setError(`This recording is too large. Keep it under ${Number((maxAudioBytes / (1024 * 1024)).toFixed(1))} MiB.`);
          setStatus('');
          return;
        }
        if (!(await validateRecordedSpeech(blob, recordedDurationMs))) {
          updateState('error');
          setError('This recording is too short or contains no clear speech. Hold to record while speaking, then release.');
          setStatus('');
          return;
        }
        if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
        setRecording({
          blob,
          mimeType: blob.type || mimeType,
          durationMs: recordedDurationMs,
          transcript: '',
        });

        updateState('processing');
        setStatus('Checking the voice credit estimate…');
        let transcriptionController: AbortController | null = null;
        try {
          const audioBase64 = await blobToBase64(blob);
          if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
          const creditRequest = await prepareVoiceCreditRequest();
          if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
          setStatus(`Estimate: ${creditRequest.estimate.estimatedCredits} credits; maximum ${creditRequest.estimate.hardCapCredits}. Transcribing…`);
          transcriptionController = new AbortController();
          transcriptionAbortRef.current = transcriptionController;
          const data = await transcribeAudioRequest({
            audioBase64,
            mimeType: (blob.type || mimeType).split(';')[0] as
              'audio/webm' | 'audio/mp4' | 'audio/m4a' | 'audio/wav' | 'audio/ogg' | 'audio/mpeg',
            durationMs: recordedDurationMs,
            language,
          }, {
            credentials: 'include',
            headers: creditRequest.headers,
            signal: transcriptionController.signal,
          });
          if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
          const receipt = (data as typeof data & { creditReceipt?: CreditReceiptDetails }).creditReceipt;
          const postflight = creditPostflightMessage(receipt, creditRequest.estimate);
          const spokenText = removeRepeatedTail(data.transcript);
          if (!spokenText) {
            updateState('error');
            setError('No speech was detected. Try again or type instead.');
            setStatus(postflight);
            return;
          }
          setTranscript(spokenText);
          setLiveText(spokenText);
          setReviewSignals(data.reviewSignals);
          setDeletionStatus(data.deletion);
          setRecording({
            blob,
            mimeType: blob.type || mimeType,
            durationMs: recordedDurationMs,
            transcript: spokenText,
          });
          updateState('review');
          setStatus(`${postflight} Review the transcript before submitting it.`);
        } catch (transcriptionError) {
          if (transcriptionController?.signal.aborted || sessionRef.current !== sessionId || cancelRequestedRef.current) return;
          updateState('error');
          setError(transcriptionError instanceof Error ? transcriptionError.message : 'Voice transcription failed. Try again or type instead.');
          setStatus('');
        } finally {
          if (transcriptionAbortRef.current === transcriptionController) {
            transcriptionAbortRef.current = null;
          }
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
        : captureError instanceof Error
          ? captureError.message
          : 'The microphone could not be started. Check browser permissions and try again.');
      setStatus('');
    }
  };

  const closeRealtime = async (waitForTermination: boolean) => {
    cleanupMediaStream();
    if (realtimeHeartbeatRef.current !== null) {
      window.clearInterval(realtimeHeartbeatRef.current);
      realtimeHeartbeatRef.current = null;
    }
    const socket = realtimeRef.current;
    realtimeRef.current = null;
    if (!socket || socket.readyState === WebSocket.CLOSED) {
      pendingRealtimePcmRef.current = new Int16Array(0);
      return;
    }
    if (socket.readyState === WebSocket.CONNECTING) {
      pendingRealtimePcmRef.current = new Int16Array(0);
      socket.close();
      return;
    }
    if (waitForTermination && socket.readyState === WebSocket.OPEN) {
      const pending = pendingRealtimePcmRef.current;
      if (pending.length >= 800 && socket.bufferedAmount <= 512 * 1024) {
        sendPublicWebSocketAudio(socket, realtimeClientSequenceRef, pending.slice().buffer);
      }
      pendingRealtimePcmRef.current = new Int16Array(0);
      try {
        const voiceEnded = waitForPublicWebSocketEvent(socket, 'voice.ended');
        sendPublicWebSocketMessage(socket, realtimeClientSequenceRef, 'voice.stop');
        await voiceEnded;
        if (socket.readyState === WebSocket.OPEN) {
          const sessionClosed = waitForPublicWebSocketEvent(socket, 'session.closed');
          sendPublicWebSocketMessage(socket, realtimeClientSequenceRef, 'session.close', {
            reason: cancelRequestedRef.current ? 'cancelled' : 'completed',
          });
          await sessionClosed;
        }
      } catch {
        // The socket is closed below if the server cannot acknowledge shutdown.
      }
    }
    if (socket.readyState !== WebSocket.CLOSED) socket.close();
  };

  const startRealtimeSession = async (sessionId: number) => {
    if (!navigator.mediaDevices?.getUserMedia || typeof AudioContext === 'undefined') {
      updateState('error');
      setError('Real-time transcription is not supported in this browser. Use recorded transcription instead.');
      return;
    }
    updateMode('live');
    updateState('starting');
    realtimeMaxSessionSecondsRef.current = DEFAULT_REALTIME_MAX_SESSION_SECONDS;
    let postflight = '';
    let realtimeReadyTimeout: number | null = null;
    voiceCreditPostflightRef.current = '';
    setStatus('Checking the voice credit estimate…');
    try {
      const creditRequest = await prepareVoiceCreditRequest();
      setStatus(`Estimate: ${creditRequest.estimate.estimatedCredits} credits; maximum ${creditRequest.estimate.hardCapCredits}. Requesting microphone permission…`);
      const stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true } });
      if (sessionRef.current !== sessionId || cancelRequestedRef.current) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }
      mediaStreamRef.current = stream;
      setStatus('Starting the secure live session…');
      const socket = new WebSocket(publicWebSocketURL());
      realtimeRef.current = socket;
      realtimeClientSequenceRef.current = 0;
      realtimeServerSequenceRef.current = 0;
      let readySettled = false;
      let sessionStarted = false;
      let resolveReady: () => void = () => undefined;
      let rejectReady: (error: Error) => void = () => undefined;
      const ready = new Promise<void>((resolve, reject) => {
        resolveReady = resolve;
        rejectReady = reject;
      });
      void ready.catch(() => undefined);
      realtimeReadyTimeout = window.setTimeout(() => {
        if (readySettled) return;
        readySettled = true;
        rejectReady(new Error('The secure live session took too long to start. Try again.'));
      }, 30_000);
      const failRealtime = (message: string) => {
        updateState('error');
        setError(message);
        setStatus(postflight);
        if (!readySettled) {
          readySettled = true;
          if (realtimeReadyTimeout !== null) window.clearTimeout(realtimeReadyTimeout);
          rejectReady(new Error(message));
        }
        void closeRealtime(false);
      };
      socket.addEventListener('message', (event: MessageEvent) => {
        if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
        if (typeof event.data !== 'string') {
          failRealtime('The live transcription server sent an invalid response.');
          return;
        }
        const envelope = parsePublicWebSocketEnvelope<RealtimeServerEvent>(event.data);
        if (!envelope) {
          failRealtime('The live transcription server sent an invalid response.');
          return;
        }
        if (envelope.type === 'protocol.error') {
          failRealtime(envelope.payload?.message || 'Real-time transcription stopped unexpectedly. Try recorded transcription instead.');
          return;
        }
        if (envelope.sequence > 0) {
          if (envelope.sequence <= realtimeServerSequenceRef.current) return;
          if (realtimeServerSequenceRef.current > 0 && envelope.sequence !== realtimeServerSequenceRef.current + 1) {
            failRealtime('The live connection missed a server event. Reconnect and start a new voice session.');
            return;
          }
          realtimeServerSequenceRef.current = envelope.sequence;
        }
        if (envelope.type === 'session.ready') {
          if (sessionStarted) return;
          sessionStarted = true;
          try {
            sendPublicWebSocketMessage(socket, realtimeClientSequenceRef, 'voice.start', {
              idempotencyKey: creditRequest.headers['Idempotency-Key'],
              policyVersion: Number(creditRequest.headers['X-AI-Credit-Policy-Version']),
            });
          } catch {
            failRealtime('The secure live session could not start. Try again.');
          }
          return;
        }
        if (envelope.type === 'voice.ready') {
          if (readySettled) return;
          try {
            postflight = creditPostflightMessage(envelope.payload?.creditReceipt, creditRequest.estimate);
            if (typeof envelope.payload?.maxSessionDurationSeconds === 'number' && envelope.payload.maxSessionDurationSeconds > 0) {
              realtimeMaxSessionSecondsRef.current = Math.min(
                DEFAULT_REALTIME_MAX_SESSION_SECONDS,
                Math.max(1, Math.floor(envelope.payload.maxSessionDurationSeconds)),
              );
            }
            voiceCreditPostflightRef.current = postflight;
            setStatus(`${postflight} Connecting live transcription…`);
            readySettled = true;
            if (realtimeReadyTimeout !== null) window.clearTimeout(realtimeReadyTimeout);
            resolveReady();
          } catch (readyError) {
            failRealtime(readyError instanceof Error ? readyError.message : 'The live session credit receipt was invalid.');
          }
          return;
        }
        if (envelope.type === 'voice.failed') {
          failRealtime(envelope.payload?.message || 'Real-time transcription stopped unexpectedly. Try recorded transcription instead.');
          return;
        }
        if (envelope.type === 'voice.session_limit') {
          durationStopRequestedRef.current = true;
          stopRequestedRef.current = true;
          cleanupMediaStream();
          updateState('processing');
          setStatus(`The ${realtimeMaxSessionSecondsRef.current}-second live session limit was reached. Finishing the transcript…`);
          return;
        }
        if (envelope.type === 'voice.provider' && envelope.payload?.type === 'Termination') {
          if (stopRequestedRef.current || durationStopRequestedRef.current) return;
          durationStopRequestedRef.current = true;
          stopRequestedRef.current = true;
          cleanupMediaStream();
          updateState('processing');
          setStatus('The live transcription session ended. Finishing the transcript…');
          return;
        }
        if (envelope.type === 'voice.ended') {
          if (durationStopRequestedRef.current && socket.readyState === WebSocket.OPEN) {
            sendPublicWebSocketMessage(socket, realtimeClientSequenceRef, 'session.close', { reason: 'completed' });
          }
          return;
        }
        const message = envelope.type === 'voice.provider' ? envelope.payload : undefined;
        if (message?.type === 'Turn' &&
          typeof message.turn_order === 'number' &&
          typeof message.transcript === 'string' &&
          typeof message.end_of_turn === 'boolean') {
          collectRealtimeTurn({
            type: message.type,
            turn_order: message.turn_order,
            transcript: message.transcript,
            end_of_turn: message.end_of_turn,
            words: message.words,
          });
        }
      });
      socket.addEventListener('error', () => {
        if (sessionRef.current !== sessionId || cancelRequestedRef.current || stateRef.current === 'error') return;
        failRealtime('Real-time transcription could not connect. Try recorded transcription instead.');
      });
      socket.addEventListener('close', () => {
        if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
        if (stateRef.current === 'error') return;
        if (durationStopRequestedRef.current) {
          void finishReview(sessionId);
          return;
        }
        if (stopRequestedRef.current) return;
        failRealtime('The real-time transcription session ended unexpectedly.');
      });
      await waitForWebSocketOpen(socket);
      if (sessionRef.current !== sessionId || cancelRequestedRef.current) {
        stream.getTracks().forEach((track) => track.stop());
        socket.close();
        return;
      }
      mediaStreamRef.current = stream;
      sendPublicWebSocketMessage(socket, realtimeClientSequenceRef, 'session.start');
      await ready;
      if (sessionRef.current !== sessionId || cancelRequestedRef.current) {
        stream.getTracks().forEach((track) => track.stop());
        await closeRealtime(false);
        return;
      }
      realtimeHeartbeatRef.current = window.setInterval(() => {
        if (socket.readyState !== WebSocket.OPEN || socket.bufferedAmount > 512 * 1024) return;
        sendPublicWebSocketMessage(socket, realtimeClientSequenceRef, 'session.heartbeat');
      }, 15_000);

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
        if (pcm.length === 0 || socket.readyState !== WebSocket.OPEN) return;
        if (socket.bufferedAmount > 512 * 1024) {
          pendingRealtimePcmRef.current = new Int16Array(0);
          failRealtime('The live audio connection is too slow. Try again or use recorded transcription.');
          return;
        }
        const pending = pendingRealtimePcmRef.current;
        const combined = new Int16Array(pending.length + pcm.length);
        combined.set(pending);
        combined.set(pcm, pending.length);
        const frameSamples = 1_600;
        const sendLength = Math.floor(combined.length / frameSamples) * frameSamples;
        for (let offset = 0; offset < sendLength; offset += frameSamples) {
          sendPublicWebSocketAudio(socket, realtimeClientSequenceRef, combined.slice(offset, offset + frameSamples).buffer);
        }
        pendingRealtimePcmRef.current = combined.slice(sendLength);
      };
      source.connect(processor);
      processor.connect(context.destination);
      audioContextRef.current = context;
      audioSourceRef.current = source;
      audioProcessorRef.current = processor;
      updateState('listening');
      setStatus(`${postflight} Live transcription is active for up to ${realtimeMaxSessionSecondsRef.current} seconds. Review the final text before submitting it.`);
    } catch (captureError) {
      await closeRealtime(false);
      if (realtimeReadyTimeout !== null) window.clearTimeout(realtimeReadyTimeout);
      if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
      updateState('error');
      setError(captureError instanceof Error
        ? captureError.message
        : 'Real-time transcription could not be started. Try recorded transcription instead.');
      setStatus(postflight);
    }
  };

  const start = async () => {
    if (stateRef.current === 'starting' || stateRef.current === 'listening' || stateRef.current === 'processing') return;
    const sessionId = sessionRef.current + 1;
    sessionRef.current = sessionId;
    transcriptionAbortRef.current?.abort();
    transcriptionAbortRef.current = null;
    cancelRequestedRef.current = false;
    stopRequestedRef.current = false;
    durationStopRequestedRef.current = false;
    finalTurnsRef.current = new Map();
    interimTurnRef.current = '';
    realtimeSignalsRef.current = [];
    pendingRealtimePcmRef.current = new Int16Array(0);
    setTranscript('');
    setLiveText('');
    setReviewSignals([]);
    setDeletionStatus(null);
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
    transcriptionAbortRef.current?.abort();
    transcriptionAbortRef.current = null;
    if (realtimeRef.current) void closeRealtime(true);
    if (mediaRecorderRef.current && mediaRecorderRef.current.state !== 'inactive') {
      mediaRecorderRef.current.stop();
    }
    cleanupMediaStream();
    setTranscript('');
    setLiveText('');
    setReviewSignals([]);
    setDeletionStatus(null);
    setRecording(null);
    updateState('idle');
    updateMode(null);
    setStatus(`${voiceCreditPostflightRef.current ? `${voiceCreditPostflightRef.current} ` : ''}Voice input canceled.`);
    setError('');
    setRecordingSeconds(0);
    setAudioLevel(0);
  };

  const reset = () => {
    cancelRequestedRef.current = true;
    sessionRef.current += 1;
    transcriptionAbortRef.current?.abort();
    transcriptionAbortRef.current = null;
    if (realtimeRef.current) void closeRealtime(true);
    if (mediaRecorderRef.current && mediaRecorderRef.current.state !== 'inactive') {
      mediaRecorderRef.current.stop();
    }
    cleanupMediaStream();
    setTranscript('');
    setLiveText('');
    setReviewSignals([]);
    setDeletionStatus(null);
    updateState('idle');
    updateMode(null);
    setStatus('');
    voiceCreditPostflightRef.current = '';
    setError('');
    setRecordingSeconds(0);
    setAudioLevel(0);
    setRecording(null);
  };

  const retry = async () => {
    if (!recording) return;
    const sessionId = sessionRef.current + 1;
    sessionRef.current = sessionId;
    cancelRequestedRef.current = false;
    transcriptionAbortRef.current?.abort();
    transcriptionAbortRef.current = null;
    updateState('processing');
    setError('');
    setStatus('Checking the voice credit estimate…');
    setDeletionStatus(null);
    setTranscript('');
    setLiveText('');
    setReviewSignals([]);
    let transcriptionController: AbortController | null = null;
    try {
      const audioBase64 = await blobToBase64(recording.blob);
      if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
      const creditRequest = await prepareVoiceCreditRequest();
      if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
      setStatus(`Estimate: ${creditRequest.estimate.estimatedCredits} credits; maximum ${creditRequest.estimate.hardCapCredits}. Transcribing…`);
      transcriptionController = new AbortController();
      transcriptionAbortRef.current = transcriptionController;
      const data = await transcribeAudioRequest({
        audioBase64,
        mimeType: recording.mimeType.split(';')[0] as
          'audio/webm' | 'audio/mp4' | 'audio/m4a' | 'audio/wav' | 'audio/ogg' | 'audio/mpeg',
        durationMs: recording.durationMs,
        language,
      }, {
        credentials: 'include',
        headers: creditRequest.headers,
        signal: transcriptionController.signal,
      });
      if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
      const receipt = (data as typeof data & { creditReceipt?: CreditReceiptDetails }).creditReceipt;
      const postflight = creditPostflightMessage(receipt, creditRequest.estimate);
      const spokenText = removeRepeatedTail(data.transcript);
      setTranscript(spokenText);
      setLiveText(spokenText);
      setReviewSignals(data.reviewSignals);
      setDeletionStatus(data.deletion);
      setRecording({ ...recording, transcript: spokenText });
      updateState('review');
      setStatus(`${postflight} Review the transcript before submitting it.`);
    } catch (retryError) {
      if (transcriptionController?.signal.aborted || sessionRef.current !== sessionId || cancelRequestedRef.current) return;
      updateState('error');
      setError(retryError instanceof Error ? retryError.message : 'Voice transcription failed. Try again.');
      setStatus('');
    } finally {
      if (transcriptionAbortRef.current === transcriptionController) {
        transcriptionAbortRef.current = null;
      }
    }
  };

  const clearRecording = () => setRecording(null);

  const transcribeFile = async (file: File) => {
    const sessionId = sessionRef.current + 1;
    sessionRef.current = sessionId;
    cancelRequestedRef.current = false;
    transcriptionAbortRef.current?.abort();
    transcriptionAbortRef.current = null;
    updateMode('recorded');
    updateState('processing');
    setTranscript('');
    setLiveText('');
    setReviewSignals([]);
    setDeletionStatus(null);
    setRecording(null);
    setRecordingSeconds(0);
    setError('');
    setStatus('Checking the audio before transcription…');
    const mimeType = file.type.split(';')[0].trim().toLowerCase();
    let transcriptionController: AbortController | null = null;
    try {
      if (!SUPPORTED_FILE_TYPES.has(mimeType)) throw new Error('Choose an audio file in WebM, MP4, M4A, WAV, OGG, or MPEG format.');
      if (file.size > maxAudioBytes) throw new Error(`This audio file is too large. Keep it under ${Number((maxAudioBytes / (1024 * 1024)).toFixed(1))} MiB.`);
      const audio = document.createElement('audio');
      audio.preload = 'metadata';
      const objectUrl = URL.createObjectURL(file);
      try {
        const durationMs = await new Promise<number>((resolve, reject) => {
          audio.onloadedmetadata = () => resolve(Math.round(audio.duration * 1000));
          audio.onerror = () => reject(new Error('The audio duration could not be read.'));
          audio.src = objectUrl;
        });
        if (!Number.isFinite(durationMs) || durationMs < MIN_RECORDED_SPEECH_MS || durationMs > maxRecordingMs) {
          throw new Error(`Audio must be between 1 second and ${Math.round(maxRecordingMs / 1000)} seconds.`);
        }
        if (!(await validateRecordedSpeech(file, durationMs))) throw new Error('This audio is too short or contains no clear speech.');
        if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
        setRecording({ blob: file, mimeType, durationMs, transcript: '' });
        const audioBase64 = await blobToBase64(file);
        const creditRequest = await prepareVoiceCreditRequest();
        if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
        setStatus(`Estimate: ${creditRequest.estimate.estimatedCredits} credits; transcribing audio…`);
        transcriptionController = new AbortController();
        transcriptionAbortRef.current = transcriptionController;
        const data = await transcribeAudioRequest({
          audioBase64,
          mimeType: mimeType as 'audio/webm' | 'audio/mp4' | 'audio/m4a' | 'audio/wav' | 'audio/ogg' | 'audio/mpeg',
          durationMs,
          language,
        }, {
          credentials: 'include',
          headers: creditRequest.headers,
          signal: transcriptionController.signal,
        });
        if (sessionRef.current !== sessionId || cancelRequestedRef.current) return;
        const spokenText = removeRepeatedTail(data.transcript);
        setTranscript(spokenText);
        setLiveText(spokenText);
        setReviewSignals(data.reviewSignals);
        setDeletionStatus(data.deletion);
        setRecording({ blob: file, mimeType: file.type, durationMs, transcript: spokenText });
        updateState(spokenText ? 'review' : 'error');
        setError(spokenText ? '' : 'No speech was detected. Try again or type instead.');
        setStatus(`${creditPostflightMessage(data.creditReceipt, creditRequest.estimate)} Review the transcript before using it.`);
      } finally {
        URL.revokeObjectURL(objectUrl);
      }
    } catch (fileError) {
      if (transcriptionController?.signal.aborted || sessionRef.current !== sessionId || cancelRequestedRef.current) return;
      updateState('error');
      setError(fileError instanceof Error ? fileError.message : 'Audio transcription failed. Try again.');
      setStatus('');
    } finally {
      if (transcriptionAbortRef.current === transcriptionController) {
        transcriptionAbortRef.current = null;
      }
    }
  };

  useEffect(() => {
    stateRef.current = state;
  }, [state]);

  useEffect(() => {
    if (state !== 'listening') return;
    const timer = window.setInterval(() => {
      setRecordingSeconds((current) => {
        const next = current + 1;
        const sessionLimit = mode === 'live' ? realtimeMaxSessionSecondsRef.current : maxRecordingMs / 1000;
        if (next >= sessionLimit && !durationStopRequestedRef.current) {
          durationStopRequestedRef.current = true;
          window.setTimeout(stop, 0);
        }
        return next;
      });
    }, 1000);
    return () => window.clearInterval(timer);
  }, [maxRecordingMs, mode, state]);

  useEffect(() => () => {
    cancelRequestedRef.current = true;
    sessionRef.current += 1;
    transcriptionAbortRef.current?.abort();
    if (mediaRecorderRef.current && mediaRecorderRef.current.state !== 'inactive') {
      mediaRecorderRef.current.stop();
    }
    cleanupMediaStream();
    void closeRealtime(false);
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
    deletionStatus,
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
    transcribeFile,
  };
}