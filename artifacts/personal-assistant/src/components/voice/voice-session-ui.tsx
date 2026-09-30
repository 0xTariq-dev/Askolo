import { useEffect, useRef, useState, type ComponentProps, type ReactNode } from 'react';
import { Mic, MicOff } from 'lucide-react';
import { Button } from '@workspace/askolo-design-system/components/ui/button';
import { Label } from '@workspace/askolo-design-system/components/ui/label';
import { Textarea } from '@workspace/askolo-design-system/components/ui/textarea';
import { cn } from '@workspace/askolo-design-system/lib/utils';
import type { VoiceTranscriptionResult } from '@/hooks/use-voice-transcription';
import {
  formatVoiceDuration,
  getVoiceCaptureClickAction,
  getVoiceStatusText,
  shouldShowVoiceTranscriptReview,
} from '@/lib/voice-flow';

type CaptureControlState = Pick<
  VoiceTranscriptionResult,
  'isListening' | 'stop' | 'cancel'
>;

type CaptureButtonProps = Omit<
  ComponentProps<typeof Button>,
  | 'aria-label'
  | 'aria-pressed'
  | 'children'
  | 'onBlur'
  | 'onClick'
  | 'onKeyDown'
  | 'onKeyUp'
  | 'onPointerCancel'
  | 'onPointerDown'
  | 'onPointerUp'
> & {
  voice: CaptureControlState;
  onStart: () => void | Promise<void>;
  startText: string;
  stopText: string;
  iconOnly?: boolean;
};

export function VoiceCaptureButton({
  voice,
  onStart,
  startText,
  stopText,
  iconOnly = false,
  className,
  disabled,
  ...buttonProps
}: CaptureButtonProps) {
  const suppressKeyboardClick = useRef(false);
  const beginCapture = () => {
    if (!disabled) void onStart();
  };

  return (
    <Button
      {...buttonProps}
      type={buttonProps.type ?? 'button'}
      className={cn(
        'border-border',
        voice.isListening && 'border-destructive/30 bg-destructive/10 text-destructive',
        className,
      )}
      disabled={disabled}
      aria-label={voice.isListening ? stopText : startText}
      aria-pressed={voice.isListening}
      onPointerDown={(event) => {
        if (event.button !== 0 || disabled) return;
        event.currentTarget.setPointerCapture?.(event.pointerId);
        beginCapture();
      }}
      onPointerUp={() => voice.stop()}
      onPointerCancel={() => voice.cancel()}
      onBlur={() => {
        if (voice.isListening) voice.cancel();
      }}
      onKeyDown={(event) => {
        if ((event.key === ' ' || event.key === 'Enter') && !event.repeat && !disabled) {
          event.preventDefault();
          suppressKeyboardClick.current = true;
          beginCapture();
        }
      }}
      onKeyUp={(event) => {
        if (event.key === ' ' || event.key === 'Enter') {
          event.preventDefault();
          voice.stop();
          window.setTimeout(() => {
            suppressKeyboardClick.current = false;
          }, 0);
        }
      }}
      onClick={(event) => {
        event.preventDefault();
        const action = getVoiceCaptureClickAction({
          detail: event.detail,
          suppressKeyboardClick: suppressKeyboardClick.current,
          isListening: voice.isListening,
        });
        if (action === 'ignore') {
          suppressKeyboardClick.current = false;
          return;
        }
        if (action === 'stop') voice.stop();
        else beginCapture();
      }}
    >
      {voice.isListening ? <MicOff aria-hidden="true" /> : <Mic aria-hidden="true" />}
      {!iconOnly && (voice.isListening ? 'Release to stop' : 'Record')}
    </Button>
  );
}

type FeedbackState = Pick<
  VoiceTranscriptionResult,
  | 'state'
  | 'mode'
  | 'status'
  | 'error'
  | 'liveText'
  | 'reviewSignals'
  | 'deletionStatus'
  | 'recordingSeconds'
  | 'audioLevel'
  | 'recording'
  | 'isBusy'
  | 'isListening'
  | 'cancel'
  | 'reset'
  | 'retry'
  | 'clearRecording'
>;

function VoiceWaveform({ active, level }: { active: boolean; level: number }) {
  return (
    <div
      className="flex h-8 min-w-24 items-center justify-center gap-1 rounded-md border border-border bg-background px-2"
      role="img"
      aria-label={active ? 'Microphone audio level active' : 'Microphone audio level inactive'}
      data-testid="voice-waveform"
    >
      {Array.from({ length: 18 }, (_, index) => {
        const position = index / 17;
        const shape = 0.35 + Math.sin(position * Math.PI) * 0.65;
        const height = active ? Math.max(4, Math.round(4 + level * shape * 22)) : 4;
        return (
          <span
            key={index}
            className={cn('w-0.5 rounded-full', active ? 'bg-primary' : 'bg-muted-foreground/40')}
            style={{ height }}
            aria-hidden="true"
          />
        );
      })}
    </div>
  );
}

export function VoiceCaptureFeedback({ voice }: { voice: FeedbackState }) {
  const [recordingUrl, setRecordingUrl] = useState('');
  const durationSeconds = voice.recording
    ? Math.round(voice.recording.durationMs / 1000)
    : voice.recordingSeconds;

  useEffect(() => {
    if (!voice.recording) {
      setRecordingUrl('');
      return;
    }

    const url = URL.createObjectURL(voice.recording.blob);
    setRecordingUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [voice.recording]);

  return (
    <section
      className="min-w-0 space-y-3 rounded-md border border-border bg-muted/20 p-3"
      aria-label="Voice input status"
      data-testid="voice-capture-feedback"
    >
      <div className="flex min-w-0 flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <p role="status" aria-live="polite" aria-atomic="true" className="text-sm font-medium">
            {getVoiceStatusText(voice.state, voice.mode, voice.status)}
          </p>
          <p className="text-xs text-muted-foreground">
            {voice.mode === 'live' ? 'Live mode' : voice.mode === 'recorded' ? 'Recorded mode' : 'Voice input'}
            {' · '}
            <bdi dir="ltr">{formatVoiceDuration(durationSeconds)}</bdi>
          </p>
        </div>
        <VoiceWaveform active={voice.isListening} level={voice.audioLevel} />
      </div>

      {voice.error && <p role="alert" className="text-sm text-destructive">{voice.error}</p>}

      {voice.state === 'listening' && voice.mode === 'live' && voice.liveText && (
        <p
          aria-live="off"
          className="whitespace-pre-wrap break-words rounded-md border border-border bg-background p-2 text-sm text-muted-foreground"
        >
          <span className="sr-only">Live transcript preview: </span>
          {voice.liveText}
        </p>
      )}

      {voice.reviewSignals.length > 0 && (
        <div className="rounded-md border border-warning/30 bg-warning/10 p-2 text-sm text-warning">
          <p className="font-medium">Please verify these low-confidence details:</p>
          <ul className="mt-1 list-disc ps-5">
            {voice.reviewSignals.map((signal) => (
              <li key={`${signal.startMs ?? 'unknown'}-${signal.text}`}>
                {signal.text} ({Math.round(signal.confidence * 100)}% confidence)
              </li>
            ))}
          </ul>
        </div>
      )}

      {voice.deletionStatus && (
        <p
          role={voice.deletionStatus.providerTranscript === 'deletion_failed' ? 'alert' : 'status'}
          className="text-xs text-muted-foreground"
        >
          {voice.deletionStatus.providerTranscript === 'deleted'
            ? 'Askolo did not store this recording. AssemblyAI confirmed deletion of the transcript; this does not confirm deletion of provider audio.'
            : 'Askolo did not store this recording, but AssemblyAI transcript deletion could not be confirmed. The provider may retain transcript data under its settings.'}
        </p>
      )}

      {voice.recording && recordingUrl && (
        <div className="min-w-0 space-y-2 rounded-md border border-border bg-background p-3">
          <p className="text-sm font-medium">Recorded audio · held in this browser’s memory</p>
          <audio
            controls
            preload="metadata"
            src={recordingUrl}
            aria-label="Recorded voice audio playback"
            className="w-full"
          />
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => void voice.retry()}
              disabled={voice.isBusy}
            >
              Retry transcription
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={voice.clearRecording}
              aria-label="Delete recorded audio from this browser"
            >
              Delete recording
            </Button>
          </div>
        </div>
      )}

      {voice.isBusy && (
        <Button type="button" size="sm" variant="ghost" onClick={voice.cancel}>
          Cancel voice input
        </Button>
      )}
      {voice.state === 'error' && !voice.isBusy && (
        <Button type="button" size="sm" variant="ghost" onClick={voice.reset}>
          Discard voice input
        </Button>
      )}
    </section>
  );
}

export function VoiceTranscriptReview({
  id,
  state,
  transcript,
  value,
  onChange,
  label = 'Editable transcript',
  children,
}: {
  id: string;
  state: FeedbackState['state'];
  transcript: string;
  value: string;
  onChange: (value: string) => void;
  label?: string;
  children: ReactNode;
}) {
  if (!shouldShowVoiceTranscriptReview(state, transcript)) return null;

  return (
    <section
      className="min-w-0 space-y-2 rounded-md border border-primary/20 bg-primary/5 p-3"
      aria-labelledby={`${id}-heading`}
      data-testid="voice-transcript-review"
    >
      <h3 id={`${id}-heading`} className="text-sm font-medium">Review transcript</h3>
      <Label htmlFor={id}>{label}</Label>
      <Textarea
        id={id}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className="min-h-24 bg-background"
        dir="auto"
      />
      <div className="flex flex-wrap gap-2">{children}</div>
    </section>
  );
}