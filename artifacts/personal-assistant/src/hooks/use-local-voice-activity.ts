import { useCallback, useRef, useState } from 'react';

const PROCESSOR_BUFFER_SIZE = 2_048;
const VOICE_START_RMS = 0.018;
const VOICE_END_RMS = 0.010;
const SPEECH_START_FRAMES = 3;
const SPEECH_END_FRAMES = 18;
const PRE_ROLL_SAMPLES = 8_000;
const TARGET_SAMPLE_RATE = 16_000;
const MAX_PROVIDER_START_BUFFER_SAMPLES = TARGET_SAMPLE_RATE * 8;

export interface LocalVoiceActivityOptions {
  onSpeechStart?: (
    stream: MediaStream,
    preRollPcm: Int16Array,
    takeBufferedPcm: () => { samples: Int16Array; overflow: boolean },
  ) => void;
  onSpeechEnd?: () => void;
}

export interface LocalVoiceActivityResult {
  active: boolean;
  starting: boolean;
  muted: boolean;
  speaking: boolean;
  audioLevel: number;
  error: string;
  stream: MediaStream | null;
  start: () => Promise<void>;
  stop: () => void;
  setMuted: (muted: boolean) => void;
}

function resampleToPcm16(input: Float32Array, inputRate: number): Int16Array {
  if (input.length === 0 || inputRate < TARGET_SAMPLE_RATE) return new Int16Array(0);
  const ratio = inputRate / TARGET_SAMPLE_RATE;
  const output = new Int16Array(Math.floor(input.length / ratio));
  for (let index = 0; index < output.length; index += 1) {
    const sample = Math.max(-1, Math.min(1, input[Math.floor(index * ratio)]));
    output[index] = sample < 0 ? sample * 0x8000 : sample * 0x7fff;
  }
  return output;
}

function appendBounded(frames: Int16Array[], next: Int16Array): Int16Array[] {
  if (next.length === 0) return frames;
  const combined = [...frames, next];
  let sampleCount = combined.reduce((sum, frame) => sum + frame.length, 0);
  while (combined.length > 1 && sampleCount > PRE_ROLL_SAMPLES) {
    sampleCount -= combined.shift()!.length;
  }
  return combined;
}

function flattenFrames(frames: Int16Array[]): Int16Array {
  const count = frames.reduce((sum, frame) => sum + frame.length, 0);
  const output = new Int16Array(Math.min(count, PRE_ROLL_SAMPLES));
  let offset = 0;
  for (const frame of frames) {
    if (offset >= output.length) break;
    const length = Math.min(frame.length, output.length - offset);
    output.set(frame.subarray(0, length), offset);
    offset += length;
  }
  return output;
}

export function useLocalVoiceActivity({
  onSpeechStart,
  onSpeechEnd,
}: LocalVoiceActivityOptions = {}): LocalVoiceActivityResult {
  const [active, setActive] = useState(false);
  const [starting, setStarting] = useState(false);
  const [muted, setMutedState] = useState(false);
  const [speaking, setSpeaking] = useState(false);
  const [audioLevel, setAudioLevel] = useState(0);
  const [error, setError] = useState('');
  const callbacksRef = useRef({ onSpeechStart, onSpeechEnd });
  callbacksRef.current = { onSpeechStart, onSpeechEnd };
  const activeRef = useRef(false);
  const startingRef = useRef(false);
  const mutedRef = useRef(false);
  const speakingRef = useRef(false);
  const streamRef = useRef<MediaStream | null>(null);
  const contextRef = useRef<AudioContext | null>(null);
  const sourceRef = useRef<MediaStreamAudioSourceNode | null>(null);
  const processorRef = useRef<ScriptProcessorNode | null>(null);
  const silentGainRef = useRef<GainNode | null>(null);
  const generationRef = useRef(0);
  const speechFramesRef = useRef(0);
  const quietFramesRef = useRef(0);
  const preRollRef = useRef<Int16Array[]>([]);
  const providerStartBufferRef = useRef<Int16Array[]>([]);
  const providerStartBufferActiveRef = useRef(false);
  const providerStartBufferOverflowRef = useRef(false);

  const releaseResources = useCallback((stopTracks: boolean) => {
    const processor = processorRef.current;
    if (processor) {
      processor.onaudioprocess = null;
      processor.disconnect();
      processorRef.current = null;
    }
    sourceRef.current?.disconnect();
    sourceRef.current = null;
    silentGainRef.current?.disconnect();
    silentGainRef.current = null;
    const context = contextRef.current;
    contextRef.current = null;
    if (context && context.state !== 'closed') void context.close();
    if (stopTracks) streamRef.current?.getTracks().forEach((track) => track.stop());
    streamRef.current = null;
    speechFramesRef.current = 0;
    quietFramesRef.current = 0;
    preRollRef.current = [];
    providerStartBufferRef.current = [];
    providerStartBufferActiveRef.current = false;
    providerStartBufferOverflowRef.current = false;
    speakingRef.current = false;
    setSpeaking(false);
    setAudioLevel(0);
  }, []);

  const stop = useCallback(() => {
    generationRef.current += 1;
    activeRef.current = false;
    startingRef.current = false;
    setActive(false);
    setStarting(false);
    mutedRef.current = false;
    setMutedState(false);
    if (speakingRef.current) callbacksRef.current.onSpeechEnd?.();
    releaseResources(true);
  }, [releaseResources]);

  const start = useCallback(async () => {
    if (activeRef.current || startingRef.current) return;
    if (!navigator.mediaDevices?.getUserMedia) {
      setError('Microphone access is not supported in this browser. Switch to Normal Chat.');
      return;
    }
    const AudioContextCtor = window.AudioContext ??
      (window as Window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
    if (!AudioContextCtor) {
      setError('Local voice detection is not supported in this browser. Switch to Normal Chat.');
      return;
    }

    const generation = generationRef.current + 1;
    generationRef.current = generation;
    startingRef.current = true;
    setStarting(true);
    setError('');
    setMutedState(false);
    mutedRef.current = false;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          channelCount: { ideal: 1 },
          echoCancellation: { ideal: true },
          noiseSuppression: { ideal: true },
          autoGainControl: { ideal: true },
        },
      });
      if (generationRef.current !== generation) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }
      streamRef.current = stream;
      const context = new AudioContextCtor();
      contextRef.current = context;
      await context.resume();
      if (generationRef.current !== generation) {
        releaseResources(true);
        return;
      }
      const source = context.createMediaStreamSource(stream);
      const processor = context.createScriptProcessor(PROCESSOR_BUFFER_SIZE, 1, 1);
      const silentGain = context.createGain();
      silentGain.gain.value = 0;
      source.connect(processor);
      processor.connect(silentGain);
      silentGain.connect(context.destination);
      sourceRef.current = source;
      processorRef.current = processor;
      silentGainRef.current = silentGain;
      processor.onaudioprocess = (event) => {
        if (!activeRef.current || mutedRef.current || generationRef.current !== generation) {
          setAudioLevel(0);
          return;
        }
        const input = event.inputBuffer.getChannelData(0);
        let sumSquares = 0;
        for (const sample of input) sumSquares += sample * sample;
        const rms = Math.sqrt(sumSquares / Math.max(1, input.length));
        setAudioLevel(Math.min(1, rms * 5));
        const pcm = resampleToPcm16(input, context.sampleRate);
        if (providerStartBufferActiveRef.current && !providerStartBufferOverflowRef.current && pcm.length > 0) {
          const bufferedSamples = providerStartBufferRef.current.reduce((sum, frame) => sum + frame.length, 0);
          if (bufferedSamples + pcm.length > MAX_PROVIDER_START_BUFFER_SAMPLES) {
            providerStartBufferOverflowRef.current = true;
          } else {
            providerStartBufferRef.current.push(pcm);
          }
        } else if (!providerStartBufferActiveRef.current) {
          preRollRef.current = appendBounded(preRollRef.current, pcm);
        }

        if (!speakingRef.current) {
          speechFramesRef.current = rms >= VOICE_START_RMS ? speechFramesRef.current + 1 : 0;
          if (speechFramesRef.current >= SPEECH_START_FRAMES) {
            speakingRef.current = true;
            setSpeaking(true);
            quietFramesRef.current = 0;
            if (!providerStartBufferActiveRef.current) {
              providerStartBufferRef.current = [...preRollRef.current];
              providerStartBufferActiveRef.current = true;
              providerStartBufferOverflowRef.current = false;
              const takeBufferedPcm = () => {
                const overflow = providerStartBufferOverflowRef.current;
                const samples = overflow
                  ? new Int16Array(0)
                  : flattenFrames(providerStartBufferRef.current);
                providerStartBufferRef.current = [];
                providerStartBufferActiveRef.current = false;
                providerStartBufferOverflowRef.current = false;
                return { samples, overflow };
              };
              callbacksRef.current.onSpeechStart?.(
                stream,
                flattenFrames(preRollRef.current),
                takeBufferedPcm,
              );
            }
          }
          return;
        }

        quietFramesRef.current = rms <= VOICE_END_RMS ? quietFramesRef.current + 1 : 0;
        if (quietFramesRef.current >= SPEECH_END_FRAMES) {
          speakingRef.current = false;
          setSpeaking(false);
          speechFramesRef.current = 0;
          quietFramesRef.current = 0;
          preRollRef.current = [];
          callbacksRef.current.onSpeechEnd?.();
        }
      };
      activeRef.current = true;
      startingRef.current = false;
      setActive(true);
      setStarting(false);
    } catch (captureError) {
      releaseResources(true);
      activeRef.current = false;
      startingRef.current = false;
      setActive(false);
      setStarting(false);
      setError(captureError instanceof DOMException &&
        (captureError.name === 'NotAllowedError' || captureError.name === 'SecurityError')
        ? 'Microphone permission was denied. Allow access or switch to Normal Chat.'
        : captureError instanceof Error
          ? captureError.message
          : 'The microphone could not be started. Switch to Normal Chat.');
    }
  }, [releaseResources]);

  const setMuted = useCallback((nextMuted: boolean) => {
    mutedRef.current = nextMuted;
    setMutedState(nextMuted);
    streamRef.current?.getAudioTracks().forEach((track) => {
      track.enabled = !nextMuted;
    });
    if (nextMuted && speakingRef.current) {
      speakingRef.current = false;
      setSpeaking(false);
      speechFramesRef.current = 0;
      quietFramesRef.current = 0;
      preRollRef.current = [];
      callbacksRef.current.onSpeechEnd?.();
    }
  }, []);

  return {
    active,
    starting,
    muted,
    speaking,
    audioLevel,
    error,
    stream: streamRef.current,
    start,
    stop,
    setMuted,
  };
}
