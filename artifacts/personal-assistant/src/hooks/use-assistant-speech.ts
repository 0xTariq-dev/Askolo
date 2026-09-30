import { useCallback, useEffect, useRef, useState } from 'react';

export type AssistantSpeechStatus = 'idle' | 'speaking' | 'paused' | 'error' | 'unsupported';

const MAX_SPOKEN_CHARACTERS = 12_000;
const ARABIC_SCRIPT = /[\u0600-\u06ff\u0750-\u077f\u08a0-\u08ff]/u;

function speechLanguage(text: string): string {
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

  const stop = useCallback(() => {
    const utterance = utteranceRef.current;
    utteranceRef.current = null;
    if (utterance) getSpeechSynthesis()?.cancel();
    setActiveMessageId(null);
    setStatus('idle');
    setError('');
  }, []);

  const speak = useCallback((messageId: string, value: string) => {
    const text = value.trim();
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

    const synthesis = getSpeechSynthesis();
    if (!synthesis || typeof SpeechSynthesisUtterance === 'undefined') {
      setActiveMessageId(messageId);
      setStatus('unsupported');
      setError('Speech playback is unavailable in this browser.');
      return;
    }

    const previousUtterance = utteranceRef.current;
    utteranceRef.current = null;
    if (previousUtterance) synthesis.cancel();

    try {
      const utterance = new SpeechSynthesisUtterance(text);
      utterance.lang = speechLanguage(text);
      const voices = synthesis.getVoices();
      const languageTag = utterance.lang.toLowerCase();
      const language = languageTag.split('-')[0];
      const selectedVoice =
        voices.find((voice) => voice.lang.toLowerCase() === languageTag) ??
        voices.find((voice) => voice.lang.toLowerCase().split('-')[0] === language);
      if (selectedVoice) utterance.voice = selectedVoice;

      utterance.onend = () => {
        if (utteranceRef.current !== utterance) return;
        utteranceRef.current = null;
        setActiveMessageId(null);
        setStatus('idle');
        setError('');
      };
      utterance.onerror = () => {
        if (utteranceRef.current !== utterance) return;
        utteranceRef.current = null;
        setActiveMessageId(messageId);
        setStatus('error');
        setError('Speech playback could not be completed.');
      };

      utteranceRef.current = utterance;
      setActiveMessageId(messageId);
      setStatus('speaking');
      setError('');
      synthesis.speak(utterance);
    } catch {
      utteranceRef.current = null;
      setActiveMessageId(messageId);
      setStatus('error');
      setError('Speech playback could not be started.');
    }
  }, []);

  const pause = useCallback((messageId: string) => {
    if (activeMessageId !== messageId || status !== 'speaking') return;
    const synthesis = getSpeechSynthesis();
    if (!synthesis) return;
    try {
      synthesis.pause();
      setStatus('paused');
      setError('');
    } catch {
      setStatus('error');
      setError('Speech playback could not be paused.');
    }
  }, [activeMessageId, status]);

  const resume = useCallback((messageId: string) => {
    if (activeMessageId !== messageId || status !== 'paused') return;
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
    const utterance = utteranceRef.current;
    utteranceRef.current = null;
    if (utterance) getSpeechSynthesis()?.cancel();
  }, []);

  return { activeMessageId, status, error, speak, pause, resume, stop };
}