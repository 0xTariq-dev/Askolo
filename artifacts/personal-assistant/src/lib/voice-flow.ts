export type VoiceFlowState =
  | 'idle'
  | 'starting'
  | 'listening'
  | 'processing'
  | 'review'
  | 'error';

export type VoiceFlowMode = 'live' | 'recorded' | null;

export function appendVoiceText(base: string, spoken: string): string {
  return [base.trim(), spoken.trim()].filter(Boolean).join(' ');
}

export function getReviewedVoiceValue(
  currentValue: string,
  reviewedText: string,
  strategy: 'append' | 'replace',
): string | null {
  const reviewed = reviewedText.trim();
  if (!reviewed) return null;
  return strategy === 'append' ? appendVoiceText(currentValue, reviewed) : reviewed;
}

export function getVoiceCaptureClickAction({
  detail,
  suppressKeyboardClick,
  isListening,
}: {
  detail: number;
  suppressKeyboardClick: boolean;
  isListening: boolean;
}): 'ignore' | 'start' | 'stop' {
  if (detail !== 0 || suppressKeyboardClick) return 'ignore';
  return isListening ? 'stop' : 'start';
}

export function formatVoiceDuration(seconds: number): string {
  const safeSeconds = Number.isFinite(seconds) ? Math.max(0, Math.floor(seconds)) : 0;
  const minutes = Math.floor(safeSeconds / 60).toString().padStart(2, '0');
  const remainder = (safeSeconds % 60).toString().padStart(2, '0');
  return `${minutes}:${remainder}`;
}

export function shouldShowVoiceTranscriptReview(
  state: VoiceFlowState,
  transcript: string,
): boolean {
  return state === 'review' && transcript.trim().length > 0;
}

export function canAutoSubmitAssistantVoiceTranscript(
  state: VoiceFlowState,
  transcript: string,
  reviewSignalCount: number,
): boolean {
  return shouldShowVoiceTranscriptReview(state, transcript) && reviewSignalCount === 0;
}

export function getVoiceStatusText(
  state: VoiceFlowState,
  mode: VoiceFlowMode,
  status: string,
): string {
  if (status.trim()) return status.trim();

  switch (state) {
    case 'starting':
      return 'Starting microphone…';
    case 'listening':
      return mode === 'live' ? 'Live transcription in progress.' : 'Recording audio.';
    case 'processing':
      return 'Transcribing audio…';
    case 'review':
      return 'Transcript ready. Review it before use.';
    case 'error':
      return 'Voice input could not be completed.';
    case 'idle':
    default:
      return 'Ready for voice input.';
  }
}