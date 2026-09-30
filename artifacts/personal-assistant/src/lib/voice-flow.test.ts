import assert from 'node:assert/strict';
import test from 'node:test';
import {
  appendVoiceText,
  canAutoSubmitAssistantVoiceTranscript,
  formatVoiceDuration,
  getReviewedVoiceValue,
  getVoiceCaptureClickAction,
  getVoiceStatusText,
  shouldShowVoiceTranscriptReview,
} from './voice-flow.ts';

test('voice transcript handoff preserves the existing draft and trims both parts', () => {
  assert.equal(appendVoiceText('  Existing plan notes  ', '  Add a follow-up  '), 'Existing plan notes Add a follow-up');
  assert.equal(appendVoiceText('', 'Spoken text'), 'Spoken text');
  assert.equal(appendVoiceText('Existing message', '  '), 'Existing message');
});

test('explicit transcript actions append or replace only non-empty reviewed text', () => {
  assert.equal(getReviewedVoiceValue('Existing notes', '  Add detail  ', 'append'), 'Existing notes Add detail');
  assert.equal(getReviewedVoiceValue('Existing notes', '  Replace all  ', 'replace'), 'Replace all');
  assert.equal(getReviewedVoiceValue('Existing notes', '   ', 'append'), null);
  assert.equal(getReviewedVoiceValue('Existing notes', '   ', 'replace'), null);
});

test('capture-button clicks distinguish pointer, keyboard, and assistive activation', () => {
  assert.equal(
    getVoiceCaptureClickAction({ detail: 1, suppressKeyboardClick: false, isListening: false }),
    'ignore',
  );
  assert.equal(
    getVoiceCaptureClickAction({ detail: 0, suppressKeyboardClick: true, isListening: false }),
    'ignore',
  );
  assert.equal(
    getVoiceCaptureClickAction({ detail: 0, suppressKeyboardClick: false, isListening: false }),
    'start',
  );
  assert.equal(
    getVoiceCaptureClickAction({ detail: 0, suppressKeyboardClick: false, isListening: true }),
    'stop',
  );
});

test('voice duration is stable for recording, review, and invalid values', () => {
  assert.equal(formatVoiceDuration(0), '00:00');
  assert.equal(formatVoiceDuration(9.8), '00:09');
  assert.equal(formatVoiceDuration(61), '01:01');
  assert.equal(formatVoiceDuration(-4), '00:00');
  assert.equal(formatVoiceDuration(Number.NaN), '00:00');
});

test('transcript review is shown only for non-empty completed transcripts', () => {
  assert.equal(shouldShowVoiceTranscriptReview('review', '  Words to review  '), true);
  assert.equal(shouldShowVoiceTranscriptReview('review', '   '), false);
  assert.equal(shouldShowVoiceTranscriptReview('listening', 'Words to review'), false);
});

test('assistant auto-submit requires a completed transcript with no review signals', () => {
  assert.equal(canAutoSubmitAssistantVoiceTranscript('review', 'Add milk', 0), true);
  assert.equal(canAutoSubmitAssistantVoiceTranscript('review', 'Add milk', 1), false);
  assert.equal(canAutoSubmitAssistantVoiceTranscript('review', '  ', 0), false);
  assert.equal(canAutoSubmitAssistantVoiceTranscript('listening', 'Add milk', 0), false);
});

test('voice status has a useful fallback while preserving provider status text', () => {
  assert.equal(getVoiceStatusText('listening', 'live', ''), 'Live transcription in progress.');
  assert.equal(getVoiceStatusText('listening', 'recorded', ''), 'Recording audio.');
  assert.equal(getVoiceStatusText('review', null, ''), 'Transcript ready. Review it before use.');
  assert.equal(getVoiceStatusText('processing', null, 'Checking audio…'), 'Checking audio…');
});