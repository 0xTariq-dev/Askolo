import { useCallback, useEffect, useRef, useState } from 'react';

import { goApi } from '@/lib/go-api';

export type AssistantSpeechStatus = 'idle' | 'loading' | 'speaking' | 'paused' | 'error' | 'unsupported';

const MAX_SPOKEN_CHARACTERS = 12_000;

export function useAssistantSpeech() {
  const [activeMessageId, setActiveMessageId] = useState<string | null>(null);
  const [status, setStatus] = useState<AssistantSpeechStatus>('idle');
  const [error, setError] = useState('');
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const objectUrlRef = useRef<string | null>(null);
  const requestRef = useRef<AbortController | null>(null);
  const generationRef = useRef(0);

  const releaseAudio = useCallback(() => {
    const audio = audioRef.current;
    audioRef.current = null;
    if (audio) {
      audio.onended = null;
      audio.onerror = null;
      try {
        audio.pause();
        audio.removeAttribute('src');
        audio.load?.();
      } catch {
        // Cleanup must not prevent the rest of playback state from being cleared.
      }
    }
    const objectUrl = objectUrlRef.current;
    objectUrlRef.current = null;
    if (objectUrl) URL.revokeObjectURL(objectUrl);
  }, []);

  const cancelPlayback = useCallback(() => {
    requestRef.current?.abort();
    requestRef.current = null;
    releaseAudio();
  }, [releaseAudio]);

  const stop = useCallback(() => {
    generationRef.current += 1;
    cancelPlayback();
    setActiveMessageId(null);
    setStatus('idle');
    setError('');
  }, [cancelPlayback]);

  const speak = useCallback(async (
    messageId: string,
    value: string,
    options: { runId?: string; locale?: string } = {},
  ) => {
    const text = value.trim();
    generationRef.current += 1;
    const generation = generationRef.current;
    cancelPlayback();
    if (!messageId) return;
    if (!text) {
      setActiveMessageId(messageId);
      setStatus('error');
      setError('There is no text to read.');
      return;
    }
    if (text.length > MAX_SPOKEN_CHARACTERS) {
      setActiveMessageId(messageId);
      setStatus('error');
      setError('This response is too long to read aloud.');
      return;
    }

    if (!options.runId) {
      setActiveMessageId(messageId);
      setStatus('error');
      setError('This response is not available from Azure speech.');
      return;
    }

    const controller = new AbortController();
    requestRef.current = controller;
    setActiveMessageId(messageId);
    setStatus('loading');
    setError('');

    const isCurrent = () => generation === generationRef.current && !controller.signal.aborted;
    const finishAzurePlayback = () => {
      if (!isCurrent()) return;
      requestRef.current = null;
      releaseAudio();
      setActiveMessageId(null);
      setStatus('idle');
      setError('');
    };

    try {
      const audioBlob = await goApi.synthesizeAssistantSpeech(options.runId, controller.signal);
      if (!isCurrent()) return;
      if (
        !(audioBlob instanceof Blob) ||
        audioBlob.size === 0 ||
        audioBlob.size > 4 * 1024 * 1024 ||
        audioBlob.type.toLowerCase() !== 'audio/mpeg'
      ) {
        requestRef.current = null;
        setStatus('error');
        setError('Azure speech output could not be completed. Please try again.');
        return;
      }
      if (typeof Audio === 'undefined' || typeof URL.createObjectURL !== 'function') {
        requestRef.current = null;
        setStatus('unsupported');
        setError('Azure speech playback is unavailable on this device.');
        return;
      }

      const objectUrl = URL.createObjectURL(audioBlob);
      objectUrlRef.current = objectUrl;
      const audio = new Audio(objectUrl);
      audioRef.current = audio;
      audio.onended = finishAzurePlayback;
      audio.onerror = () => {
        if (!isCurrent()) return;
        requestRef.current = null;
        setStatus('error');
        setError('Azure speech playback could not be completed. Please try again.');
        releaseAudio();
      };
      requestRef.current = null;
      setStatus('speaking');
      const playResult = audio.play();
      if (playResult) await playResult;
    } catch (failure) {
      if (!isCurrent()) return;
      const httpStatus = (failure as { status?: unknown } | null)?.status;
      requestRef.current = null;
      setStatus('error');
      setError(
        httpStatus === 403
          ? 'Azure speech output requires your consent. Review the permission and try again.'
          : 'Azure speech output could not be completed. Please try again.',
      );
    }
  }, [cancelPlayback, releaseAudio]);

  const pause = useCallback((messageId: string) => {
    if (activeMessageId !== messageId || status !== 'speaking') return;
    try {
      if (!audioRef.current) return;
      audioRef.current.pause();
      setStatus('paused');
      setError('');
    } catch {
      setStatus('error');
      setError('Speech playback could not be paused.');
    }
  }, [activeMessageId, status]);

  const resume = useCallback((messageId: string) => {
    if (activeMessageId !== messageId || status !== 'paused') return;
    if (audioRef.current) {
      const audio = audioRef.current;
      void audio.play().then(() => {
        if (audioRef.current === audio) {
          setStatus('speaking');
          setError('');
        }
      }).catch(() => {
        setStatus('error');
        setError('Azure speech playback could not be resumed. Please try again.');
      });
    }
  }, [activeMessageId, status]);

  useEffect(() => () => {
    generationRef.current += 1;
    cancelPlayback();
  }, [cancelPlayback]);

  return { activeMessageId, status, error, speak, pause, resume, stop };
}