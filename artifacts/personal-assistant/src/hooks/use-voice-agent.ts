import { useCallback, useEffect, useRef, useState } from 'react';
import type { AssistantRun } from '@workspace/api-client-react';
import { creditApi, formatUsdMicros, newCreditIdempotencyKey, type CreditEstimate } from '@/lib/credit-api';
import {
  parsePublicWebSocketEnvelope,
  publicWebSocketURL,
  sendPublicWebSocketMessage,
} from '@/lib/public-websocket';

type VoiceAgentStatus = 'idle' | 'connecting' | 'live' | 'stopping';
type VoiceAgentTranscript = { role: 'user' | 'assistant'; text: string };

const SAMPLE_RATE = 24_000;
const MAX_AUDIO_BYTES = 16 * 1024;
const CONNECTION_TIMEOUT_MS = 30_000;

export function useVoiceAgent() {
  const [status, setStatus] = useState<VoiceAgentStatus>('idle');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [transcripts, setTranscripts] = useState<VoiceAgentTranscript[]>([]);
  const [voiceEstimate, setVoiceEstimate] = useState<CreditEstimate | null>(null);
  const [assistantEstimate, setAssistantEstimate] = useState<CreditEstimate | null>(null);
  const [assistantEstimateUnavailable, setAssistantEstimateUnavailable] = useState(false);
  const socketRef = useRef<WebSocket | null>(null);
  const startingRef = useRef(false);
  const clientSequenceRef = useRef({ current: 0 });
  const serverSequenceRef = useRef(0);
  const attemptRef = useRef(0);
  const timerRef = useRef<number | null>(null);
  const heartbeatRef = useRef<number | null>(null);
  const readyRef = useRef(false);
  const mediaStreamRef = useRef<MediaStream | null>(null);
  const audioContextRef = useRef<AudioContext | null>(null);
  const processorRef = useRef<ScriptProcessorNode | null>(null);
  const sourceRef = useRef<MediaStreamAudioSourceNode | null>(null);
  const muteRef = useRef<GainNode | null>(null);
  const playbackSourcesRef = useRef<Set<AudioBufferSourceNode>>(new Set());
  const playheadRef = useRef(0);

  const cleanupAudio = useCallback(() => {
    readyRef.current = false;
    if (processorRef.current) {
      processorRef.current.onaudioprocess = null;
      processorRef.current.disconnect();
      processorRef.current = null;
    }
    sourceRef.current?.disconnect();
    sourceRef.current = null;
    muteRef.current?.disconnect();
    muteRef.current = null;
    mediaStreamRef.current?.getTracks().forEach((track) => track.stop());
    mediaStreamRef.current = null;
    playbackSourcesRef.current.forEach((source) => {
      try { source.stop(); } catch { /* already ended */ }
      source.disconnect();
    });
    playbackSourcesRef.current.clear();
    playheadRef.current = 0;
    const context = audioContextRef.current;
    audioContextRef.current = null;
    if (context && context.state !== 'closed') void context.close();
  }, []);

  const clearTimers = useCallback(() => {
    if (timerRef.current !== null) window.clearTimeout(timerRef.current);
    if (heartbeatRef.current !== null) window.clearInterval(heartbeatRef.current);
    timerRef.current = null;
    heartbeatRef.current = null;
  }, []);

  const closeSocket = useCallback(() => {
    clearTimers();
    const socket = socketRef.current;
    socketRef.current = null;
    if (socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) {
      socket.close();
    }
  }, [clearTimers]);

  const stop = useCallback((unmounting = false) => {
    const wasReady = readyRef.current;
    const socket = socketRef.current;
    if (!unmounting && wasReady && socket?.readyState === WebSocket.OPEN) {
      startingRef.current = false;
      cleanupAudio();
      sendPublicWebSocketMessage(socket, clientSequenceRef.current, 'voice.agent.stop');
      setStatus('stopping');
      return;
    }
    startingRef.current = false;
    attemptRef.current += 1;
    cleanupAudio();
    if (socket?.readyState === WebSocket.OPEN) {
      try {
        sendPublicWebSocketMessage(socket, clientSequenceRef.current, 'session.close', { reason: 'cancelled' });
      } catch { /* close the socket even if the connection is already failing */ }
    }
    closeSocket();
    if (!unmounting) setStatus('idle');
  }, [cleanupAudio, closeSocket]);

  const startAudio = useCallback(async () => {
    const stream = mediaStreamRef.current;
    if (!stream) throw new Error('Microphone access was lost. Start Live Mode again.');
    const audioContext = audioContextRef.current;
    if (!audioContext || audioContext.state === 'closed') throw new Error('Audio playback is unavailable.');
    await audioContext.resume();
    const source = audioContext.createMediaStreamSource(stream);
    const processor = audioContext.createScriptProcessor(2048, 1, 1);
    const mute = audioContext.createGain();
    mute.gain.value = 0;
    processor.onaudioprocess = (event) => {
      const socket = socketRef.current;
      if (!readyRef.current || !socket || socket.readyState !== WebSocket.OPEN || socket.bufferedAmount > 128_000) return;
      const pcm = floatToPCM16(event.inputBuffer.getChannelData(0), audioContext.sampleRate);
      if (pcm.byteLength < 2 || pcm.byteLength > MAX_AUDIO_BYTES) return;
      sendPublicWebSocketMessage(socket, clientSequenceRef.current, 'voice.agent.audio', { data: bytesToBase64(pcm) });
    };
    source.connect(processor);
    processor.connect(mute);
    mute.connect(audioContext.destination);
    sourceRef.current = source;
    processorRef.current = processor;
    muteRef.current = mute;
    readyRef.current = true;
    setStatus('live');
  }, []);

  const playAudio = useCallback((encoded: string) => {
    const context = audioContextRef.current;
    if (!context || encoded.length > 64 * 1024) return;
    let pcm: Uint8Array;
    try {
      pcm = base64ToBytes(encoded);
    } catch {
      return;
    }
    if (pcm.byteLength < 2 || pcm.byteLength % 2 !== 0 || pcm.byteLength > 64 * 1024) return;
    const view = new DataView(pcm.buffer, pcm.byteOffset, pcm.byteLength);
    const samples = new Float32Array(pcm.byteLength / 2);
    for (let index = 0; index < samples.length; index += 1) samples[index] = view.getInt16(index * 2, true) / 32768;
    const buffer = context.createBuffer(1, samples.length, SAMPLE_RATE);
    buffer.copyToChannel(samples, 0);
    const source = context.createBufferSource();
    source.buffer = buffer;
    source.connect(context.destination);
    const startAt = Math.max(context.currentTime, playheadRef.current);
    source.start(startAt);
    playheadRef.current = startAt + buffer.duration;
    playbackSourcesRef.current.add(source);
    source.addEventListener('ended', () => {
      playbackSourcesRef.current.delete(source);
      source.disconnect();
    }, { once: true });
  }, []);

  const start = useCallback(async (conversationId?: string) => {
    if (status !== 'idle' || socketRef.current || startingRef.current) return;
    startingRef.current = true;
    const attempt = attemptRef.current + 1;
    attemptRef.current = attempt;
    setStatus('connecting');
    setError('');
    setNotice('');
    setTranscripts([]);
    setVoiceEstimate(null);
    setAssistantEstimate(null);
    setAssistantEstimateUnavailable(false);

    try {
      if (!navigator.mediaDevices?.getUserMedia || typeof AudioContext === 'undefined') {
        throw new Error('Live voice requires microphone access in a supported browser.');
      }
      let audioContext: AudioContext;
      try {
        audioContext = new AudioContext({ sampleRate: SAMPLE_RATE });
      } catch {
        audioContext = new AudioContext();
      }
      audioContextRef.current = audioContext;
      await audioContext.resume();

      const estimate = await creditApi.estimate('voice.agent', 180);
      if (attemptRef.current !== attempt) return;
      setVoiceEstimate(estimate);
      if (!estimate.canReserve) {
        throw new Error(`Live Mode needs a maximum reservation of ${formatUsdMicros(estimate.hardCapUsdMicros)}; your available balance is ${formatUsdMicros(estimate.availableUsdMicros)}.`);
      }
      try {
        setAssistantEstimate(await creditApi.estimate('assistant', 1));
      } catch {
        setAssistantEstimateUnavailable(true);
      }
      if (attemptRef.current !== attempt) return;

      const stream = await navigator.mediaDevices.getUserMedia({
        audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true, autoGainControl: true },
      });
      if (attemptRef.current !== attempt) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }
      mediaStreamRef.current = stream;

      const socket = new WebSocket(publicWebSocketURL());
      socketRef.current = socket;
      clientSequenceRef.current = { current: 0 };
      serverSequenceRef.current = 0;
      readyRef.current = false;
      timerRef.current = window.setTimeout(() => {
        setError('The Live Mode connection timed out. Try again.');
        stop();
      }, CONNECTION_TIMEOUT_MS);

      socket.addEventListener('open', () => {
        if (attemptRef.current !== attempt) return;
        sendPublicWebSocketMessage(socket, clientSequenceRef.current, 'session.start');
        heartbeatRef.current = window.setInterval(() => {
          if (socket.readyState === WebSocket.OPEN) {
            sendPublicWebSocketMessage(socket, clientSequenceRef.current, 'session.heartbeat');
          }
        }, 15_000);
      });
      socket.addEventListener('message', (message) => {
        if (typeof message.data !== 'string' || attemptRef.current !== attempt) return;
        const envelope = parsePublicWebSocketEnvelope<Record<string, unknown>>(message.data);
        if (!envelope) {
          setError('The Live Mode connection returned an invalid message.');
          stop();
          return;
        }
        if (envelope.sequence > 0) {
          if (envelope.sequence <= serverSequenceRef.current) return;
          if (serverSequenceRef.current > 0 && envelope.sequence !== serverSequenceRef.current + 1) {
            setError('The Live Mode connection missed an event. Start a new session to continue.');
            stop();
            return;
          }
          serverSequenceRef.current = envelope.sequence;
        }
        const payload = envelope.payload ?? {};
        if (envelope.type === 'protocol.error') {
          setError(typeof payload.message === 'string' ? payload.message : 'Live Mode could not continue.');
          stop();
          return;
        }
        if (envelope.type === 'session.ready') {
          if (socket.readyState !== WebSocket.OPEN) return;
          sendPublicWebSocketMessage(socket, clientSequenceRef.current, 'voice.agent.start', {
            conversationId,
            locale: 'en',
            idempotencyKey: newCreditIdempotencyKey(),
            policyVersion: estimate.policyVersion,
            assistantPolicyVersion: estimate.policyVersion,
          });
          return;
        }
        if (envelope.type === 'voice.agent.ready') {
          startingRef.current = false;
          if (timerRef.current !== null) window.clearTimeout(timerRef.current);
          timerRef.current = null;
          void startAudio().catch(() => {
            setError('Microphone audio could not be started. End the session and try again.');
            stop();
          });
          return;
        }
        if (envelope.type === 'voice.agent.audio' && typeof payload.data === 'string') {
          playAudio(payload.data);
          return;
        }
        if (envelope.type === 'voice.agent.transcript' && typeof payload.text === 'string') {
          const role: VoiceAgentTranscript['role'] = payload.type === 'transcript.user' ? 'user' : 'assistant';
          setTranscripts((current) => [...current, { role, text: payload.text as string }].slice(-12));
          return;
        }
        if (envelope.type === 'voice.agent.transcript.delta') {
          return;
        }
        if (envelope.type === 'voice.agent.activity' && typeof payload.type === 'string') {
          if (payload.type === 'reply.started') setNotice('Askolo is speaking.');
          else if (payload.type === 'reply.interrupted' || payload.type === 'input.speech.started') setNotice('Listening.');
          return;
        }
        if (envelope.type === 'voice.agent.action_proposed') {
          const run = payload as Partial<AssistantRun>;
          if (typeof run.id === 'string' && typeof run.state === 'string') {
            window.dispatchEvent(new CustomEvent('askolo:voice-agent-action', { detail: run }));
          }
          setNotice('Askolo prepared an action for review in Assistant chat.');
          return;
        }
        if (envelope.type === 'voice.agent.action_refused' || envelope.type === 'voice.agent.action_failed') {
          setNotice(typeof payload.message === 'string' ? payload.message : 'Askolo could not prepare that action.');
          return;
        }
        if (envelope.type === 'voice.agent.limit_reached') {
          setNotice(typeof payload.message === 'string' ? payload.message : 'The 180-second session limit was reached.');
          setStatus('stopping');
          return;
        }
        if (envelope.type === 'voice.agent.stopping') {
          cleanupAudio();
          setStatus('stopping');
          return;
        }
        if (envelope.type === 'voice.agent.ended' || envelope.type === 'voice.agent.failed') {
          startingRef.current = false;
          const wasFailure = envelope.type === 'voice.agent.failed';
          if (wasFailure && typeof payload.message === 'string') setError(payload.message);
          if (payload.deletionStatus === 'soft_deleted') {
            setNotice('AssemblyAI accepted a soft-delete request; physical erasure is not verified.');
          } else if (payload.deletionStatus === 'unconfirmed') {
            setNotice('The provider deletion request could not be confirmed. This is not a confirmation of physical erasure.');
          }
          cleanupAudio();
          closeSocket();
          setStatus('idle');
        }
      });
      socket.addEventListener('error', () => {
        if (attemptRef.current !== attempt) return;
        setError('The Live Mode connection failed. Try again.');
        stop();
      });
      socket.addEventListener('close', () => {
        if (attemptRef.current !== attempt) return;
        startingRef.current = false;
        clearTimers();
        cleanupAudio();
        socketRef.current = null;
        readyRef.current = false;
        setStatus('idle');
      }, { once: true });
    } catch (cause) {
      if (attemptRef.current !== attempt) return;
      startingRef.current = false;
      cleanupAudio();
      closeSocket();
      setStatus('idle');
      setError(cause instanceof Error ? cause.message : 'Live Mode could not start.');
    }
  }, [cleanupAudio, closeSocket, clearTimers, playAudio, startAudio, status, stop]);

  useEffect(() => () => stop(true), [stop]);

  return {
    status, error, notice, transcripts, voiceEstimate, assistantEstimate,
    assistantEstimateUnavailable, start, stop,
  };
}

function floatToPCM16(input: Float32Array, inputRate: number): ArrayBuffer {
  const ratio = inputRate / SAMPLE_RATE;
  const outputLength = Math.floor(input.length / ratio);
  const output = new ArrayBuffer(outputLength * 2);
  const view = new DataView(output);
  for (let index = 0; index < outputLength; index += 1) {
    const position = index * ratio;
    const left = Math.floor(position);
    const fraction = position - left;
    const sample = Math.max(-1, Math.min(1, input[left] + (input[Math.min(left + 1, input.length - 1)] - input[left]) * fraction));
    view.setInt16(index * 2, sample < 0 ? sample * 0x8000 : sample * 0x7fff, true);
  }
  return output;
}

function bytesToBase64(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = '';
  for (let offset = 0; offset < bytes.length; offset += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(offset, Math.min(offset + 0x8000, bytes.length)));
  }
  return btoa(binary);
}

function base64ToBytes(value: string): Uint8Array {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index);
  return bytes;
}