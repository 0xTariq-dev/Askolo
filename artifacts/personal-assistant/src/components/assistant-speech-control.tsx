import React from 'react';
import { Pause, Play, Square, Volume2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import type { AssistantSpeechStatus } from '@/hooks/use-assistant-speech';

interface AssistantSpeechControlProps {
  status: AssistantSpeechStatus;
  error: string;
  onSpeak: () => void;
  onPause: () => void;
  onResume: () => void;
  onStop: () => void;
}

export function AssistantSpeechControl({
  status,
  error,
  onSpeak,
  onPause,
  onResume,
  onStop,
}: AssistantSpeechControlProps) {
  const isSpeaking = status === 'speaking';
  const isPaused = status === 'paused';
  const isActive = isSpeaking || isPaused;
  const label = isSpeaking
    ? 'Pause response playback'
    : isPaused
      ? 'Resume response playback'
      : 'Listen to response';

  return (
    <div className="mt-2 flex flex-wrap items-center gap-2">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="min-h-9"
        aria-label={label}
        aria-pressed={isActive}
        onClick={isSpeaking ? onPause : isPaused ? onResume : onSpeak}
      >
        {isSpeaking ? <Pause className="mr-1.5 h-4 w-4" aria-hidden="true" /> :
          isPaused ? <Play className="mr-1.5 h-4 w-4" aria-hidden="true" /> :
            <Volume2 className="mr-1.5 h-4 w-4" aria-hidden="true" />}
        {isSpeaking ? 'Pause' : isPaused ? 'Resume' : status === 'unsupported' ? 'Try again' : 'Listen'}
      </Button>
      {isActive && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="min-h-9"
          aria-label="Stop response playback"
          onClick={onStop}
        >
          <Square className="mr-1.5 h-4 w-4" aria-hidden="true" />
          Stop
        </Button>
      )}
      {error && (
        <span className="text-xs text-destructive" role="status" aria-live="polite">
          {error}
        </span>
      )}
    </div>
  );
}