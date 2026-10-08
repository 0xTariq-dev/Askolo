import {
  AudioLines,
  MessageSquareText,
  Mic,
  MicOff,
  PhoneOff,
  RotateCcw,
  Volume2,
  VolumeX,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

export type VoiceChatControlsProps = {
  active: boolean;
  starting?: boolean;
  muted: boolean;
  status: string;
  error: string;
  speechError: boolean;
  speaking: boolean;
  onStart: () => void;
  onEnd: () => void;
  onToggleMute: () => void;
  onInterrupt: () => void;
  onRetrySpeech: () => void;
  onSwitchToChat: () => void;
};

export function VoiceChatControls({
  active,
  starting = false,
  muted,
  status,
  error,
  speechError,
  speaking,
  onStart,
  onEnd,
  onToggleMute,
  onInterrupt,
  onRetrySpeech,
  onSwitchToChat,
}: VoiceChatControlsProps) {
  const statusText =
    status.trim() ||
    (starting
      ? 'Requesting microphone permission'
      : speaking
        ? 'Askolo is speaking'
        : active
          ? muted
            ? 'Listening is paused'
            : 'Listening for your voice'
          : 'Voice Chat is off');
  const showRecovery = speechError || Boolean(error);

  return (
    <section
      aria-labelledby="voice-chat-controls-title"
      className="w-full min-w-0 overflow-hidden rounded-xl border border-border bg-card text-card-foreground shadow-sm"
      data-testid="voice-chat-controls"
    >
      <div className="flex flex-col gap-5 p-4 sm:p-5">
        <header className="flex min-w-0 items-start gap-3">
          <div
            aria-hidden="true"
            className={cn(
              'flex size-10 shrink-0 items-center justify-center rounded-full border',
              active
                ? 'border-primary/25 bg-primary/10 text-primary'
                : 'border-border bg-muted/50 text-muted-foreground',
            )}
          >
            {active ? <AudioLines className="size-5" /> : <Mic className="size-5" />}
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
              <h2 id="voice-chat-controls-title" className="text-sm font-semibold">
                Voice Chat
              </h2>
              <span
                className={cn(
                  'inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] font-medium',
                  active
                    ? 'bg-primary/10 text-primary'
                    : 'bg-muted text-muted-foreground',
                )}
              >
                <span
                  aria-hidden="true"
                  className={cn(
                    'size-1.5 rounded-full',
                    active ? 'bg-primary' : 'bg-muted-foreground/60',
                  )}
                />
                {starting ? 'Starting' : active ? 'On' : 'Off'}
              </span>
            </div>
            <p
              aria-live="polite"
              aria-atomic="true"
              className="mt-1 text-sm text-muted-foreground"
              data-testid="status-voice-chat"
            >
              {statusText}
            </p>
          </div>
        </header>

        <div className="rounded-lg border border-border/80 bg-muted/25 px-3.5 py-3">
          <p className="text-xs leading-relaxed text-muted-foreground">
            {active && muted
              ? 'The microphone is muted. Askolo will not receive voice audio until you unmute.'
              : active
                ? 'Speech is detected locally. AssemblyAI handles one utterance at a time; recognized text is sent to chat without transcript review. You can interrupt spoken replies.'
                : 'Listening starts locally on this device. AssemblyAI connects after speech is detected and closes its session after each utterance. Recognized speech is sent to chat without a review step.'}
          </p>
          <p className="mt-1.5 text-[11px] font-medium text-muted-foreground/85">
            A separate mode from Normal Chat and Live Calls.
          </p>
        </div>

        {error && (
          <p
            className="rounded-md border border-destructive/25 bg-destructive/5 px-3 py-2 text-sm text-destructive"
            role="alert"
            data-testid="error-voice-chat"
          >
            {error}
          </p>
        )}

        <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
          {!active && !starting ? (
            <Button
              type="button"
              onClick={onStart}
              className="min-h-11 w-full sm:w-auto"
              aria-label="Start Voice Chat and begin local listening"
              data-testid="button-start-voice-chat"
            >
              <Mic className="size-4" aria-hidden="true" />
              Start Voice Chat
            </Button>
          ) : starting ? (
            <Button type="button" variant="outline" disabled className="min-h-11 w-full sm:w-auto">
              <AudioLines className="size-4 animate-pulse" aria-hidden="true" />
              Starting Voice Chat…
            </Button>
          ) : (
            <>
              <Button
                type="button"
                variant={muted ? 'secondary' : 'outline'}
                onClick={onToggleMute}
                aria-pressed={muted}
                aria-label={muted ? 'Unmute microphone' : 'Mute microphone'}
                className="min-h-11 w-full sm:w-auto"
                data-testid="button-toggle-voice-mute"
              >
                {muted ? (
                  <MicOff className="size-4" aria-hidden="true" />
                ) : (
                  <Mic className="size-4" aria-hidden="true" />
                )}
                {muted ? 'Unmute' : 'Mute'}
              </Button>
              {speaking && (
                <Button
                  type="button"
                  variant="outline"
                  onClick={onInterrupt}
                  className="min-h-11 w-full border-primary/30 text-primary hover:bg-primary/5 sm:w-auto"
                  aria-label="Interrupt Askolo's spoken reply"
                  data-testid="button-interrupt-voice-reply"
                >
                  <AudioLines className="size-4" aria-hidden="true" />
                  Interrupt reply
                </Button>
              )}
              <Button
                type="button"
                variant="ghost"
                onClick={onEnd}
                className="min-h-11 w-full text-muted-foreground hover:text-destructive sm:ml-auto sm:w-auto"
                aria-label="End Voice Chat"
                data-testid="button-end-voice-chat"
              >
                <PhoneOff className="size-4" aria-hidden="true" />
                End Voice Chat
              </Button>
            </>
          )}
        </div>

        {active && (
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
            {muted ? (
              <MicOff className="size-3.5" aria-hidden="true" />
            ) : (
              <Mic className="size-3.5" aria-hidden="true" />
            )}
            <span>{muted ? 'Microphone muted' : 'Local listening active'}</span>
            <span aria-hidden="true" className="text-border">·</span>
            {speaking ? (
              <>
                <Volume2 className="size-3.5" aria-hidden="true" />
                <span>Spoken reply</span>
              </>
            ) : (
              <>
                <VolumeX className="size-3.5" aria-hidden="true" />
                <span>Waiting for speech</span>
              </>
            )}
          </div>
        )}

        {showRecovery && (
          <div className="flex flex-col gap-2 border-t border-border pt-3 sm:flex-row sm:items-center">
            {speechError && (
              <Button
                type="button"
                variant="outline"
                onClick={onRetrySpeech}
                className="min-h-10 w-full sm:w-auto"
                data-testid="button-retry-voice-speech"
              >
                <RotateCcw className="size-4" aria-hidden="true" />
                Retry spoken reply
              </Button>
            )}
            <Button
              type="button"
              variant="ghost"
              onClick={onSwitchToChat}
              className="min-h-10 w-full sm:ml-auto sm:w-auto"
              data-testid="button-switch-to-normal-chat"
            >
              <MessageSquareText className="size-4" aria-hidden="true" />
              Switch to Normal Chat
            </Button>
          </div>
        )}
      </div>
    </section>
  );
}
