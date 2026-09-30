import { useCallback, useEffect, useRef, useState } from 'react';

import { goApi } from '@/lib/go-api';

export type AssistantSpeechStatus = 'idle' | 'loading' | 'speaking' | 'paused' | 'error' | 'unsupported';

const MAX_SPOKEN_CHARACTERS = 12_000;
const ARABIC_SCRIPT = /[\u0600-\u06ff\u0750-\u077f\u08a0-\u08ff]/u;

function speechLanguage(text: string, locale?: string): string {
  if (locale === 'ar') return 'ar-EG';
  if (locale === 'en') return 'en-US';
  if (ARABIC_SCRIPT.test(text)) return 'ar';
  if (typeof navigator !== 'undefined' && navigator.language) return navigator.language;
  return 'en-US';
}

function getSpeechSynthesis(): SpeechSynthesis | null {
  if (typeof window === 'undefined') return null;
  return window.speechSynthesis ?? null;
}

export function useAssistantSpeech() {
  const [activeMessageId, setActiveMessageId] = useState<string | null>(null);
  const [status, setStatus] = useState<AssistantSpeechStatus>('idle');
  const [error, setError] = useState('');
  const utteranceRef = useRef<SpeechSynthesisUtterance | null>(null);
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const objectUrlRef = useRef<string | null>(null);
  const requestRef = useRef<AbortController | null>(null);
  const generationRef = useRef(0);
  const fallbackRef = useRef<(() => void) | null>(null);

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
    fallbackRef.current = null;
    releaseAudio();
    const utterance = utteranceRef.current;
    utteranceRef.current = null;
    if (utterance) getSpeechSynthesis()?.cancel();
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

    let fallbackStarted = false;
    const isCurrent = () => generation === generationRef.current && !controller.signal.aborted;
    const startDeviceFallback = () => {
      if (fallbackStarted || !isCurrent()) return;
      fallbackStarted = true;
      fallbackRef.current = null;
      requestRef.current = null;
      releaseAudio();

      const synthesis = getSpeechSynthesis();
      if (!synthesis || typeof SpeechSynthesisUtterance === 'undefined') {
        setStatus('unsupported');
        setError('Azure speech failed and device speech is unavailable.');
        return;
      }
      try {
        const utterance = new SpeechSynthesisUtterance(text);
        utterance.lang = speechLanguage(text, options.locale);
        const voices = synthesis.getVoices();
        const languageTag = utterance.lang.toLowerCase();
        const language = languageTag.split('-')[0];
        const selectedVoice =
          voices.find((voice) => voice.lang.toLowerCase() === languageTag) ??
          voices.find((voice) => voice.lang.toLowerCase().split('-')[0] === language);
        if (selectedVoice) utterance.voice = selectedVoice;

        utterance.onend = () => {
          if (generationRef.current !== generation || utteranceRef.current !== utterance) return;
          utteranceRef.current = null;
          fallbackRef.current = null;
          setActiveMessageId(null);
          setStatus('idle');
          setError('');
        };
        utterance.onerror = () => {
          if (generationRef.current !== generation || utteranceRef.current !== utterance) return;
          utteranceRef.current = null;
          fallbackRef.current = null;
          setActiveMessageId(messageId);
          setStatus('error');
          setError('Speech playback could not be completed.');
        };

        utteranceRef.current = utterance;
        setStatus('speaking');
        setError('');
        synthesis.speak(utterance);
      } catch {
        utteranceRef.current = null;
        fallbackRef.current = null;
        setStatus('error');
        setError('Speech playback could not be started.');
      }
    };
    fallbackRef.current = startDeviceFallback;

    const finishAzurePlayback = () => {
      if (!isCurrent()) return;
      fallbackRef.current = null;
      requestRef.current = null;
      releaseAudio();
      setActiveMessageId(null);
      setStatus('idle');
      setError('');
    };

    const allowDeviceFallback = (failure: unknown) => {
      if (!isCurrent()) return;
      const httpStatus = (failure as { status?: unknown } | null)?.status;
      if (typeof httpStatus === 'number' && httpStatus >= 400 && httpStatus < 500) {
        fallbackRef.current = null;
        requestRef.current = null;
        setStatus('error');
        setError(
          httpStatus === 403
            ? 'Azure speech output requires your consent. Review the permission and try again.'
            : 'Speech output could not be started. Try again later.',
        );
        return;
      }
      startDeviceFallback();
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
        startDeviceFallback();
        return;
      }
      if (typeof Audio === 'undefined' || typeof URL.createObjectURL !== 'function') {
        startDeviceFallback();
        return;
      }

      const objectUrl = URL.createObjectURL(audioBlob);
      objectUrlRef.current = objectUrl;
      const audio = new Audio(objectUrl);
      audioRef.current = audio;
      audio.onended = finishAzurePlayback;
      audio.onerror = () => startDeviceFallback();
      requestRef.current = null;
      setStatus('speaking');
      const playResult = audio.play();
      if (playResult) await playResult;
    } catch (failure) {
      allowDeviceFallback(failure);
    }
  }, [cancelPlayback, releaseAudio]);

  const pause = useCallback((messageId: string) => {
    if (activeMessageId !== messageId || status !== 'speaking') return;
    try {
      if (audioRef.current) {
        audioRef.current.pause();
      } else {
        const synthesis = getSpeechSynthesis();
        if (!synthesis) return;
        synthesis.pause();
      }
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
        fallbackRef.current?.();
      });
      return;
    }
    const synthesis = getSpeechSynthesis();
    if (!synthesis) return;
    try {
      synthesis.resume();
      setStatus('speaking');
      setError('');
    } catch {
      setStatus('error');
      setError('Speech playback could not be resumed.');
    }
  }, [activeMessageId, status]);

  useEffect(() => () => {
    generationRef.current += 1;
    cancelPlayback();
  }, [cancelPlayback]);

  return { activeMessageId, status, error, speak, pause, resume, stop };
}