import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import {
  VoiceCaptureFeedback,
  VoiceTranscriptReview,
} from '@/components/voice/voice-session-ui';

Object.assign(globalThis, { React });

const idleVoice = {
  state: 'idle' as const,
  mode: null as 'live' | 'recorded' | null,
  status: '',
  error: '',
  liveText: '',
  reviewSignals: [] as Array<{ text: string; confidence: number; startMs?: number }>,
  deletionStatus: null,
  recordingSeconds: 0,
  audioLevel: 0,
  recording: null,
  isBusy: false,
  isListening: false,
  cancel: () => {},
  reset: () => {},
  retry: async () => {},
  clearRecording: () => {},
};

test('transcript review exposes the editable value and an explicit next action', () => {
  const html = renderToStaticMarkup(
    React.createElement(
      VoiceTranscriptReview,
      {
        id: 'voice-review',
        state: 'review',
        transcript: 'Original words',
        value: 'Corrected words',
        onChange: () => {},
        label: 'Editable transcript',
      },
      React.createElement('button', { type: 'button' }, 'Use transcript'),
    ),
  );

  assert.match(html, /Review transcript/);
  assert.match(html, /for="voice-review"/);
  assert.match(html, /dir="auto">Corrected words<\/textarea>/);
  assert.match(html, /Use transcript/);
});

test('shared voice feedback announces status, elapsed time, waveform, live preview, and uncertain values without numeric confidence', () => {
  const html = renderToStaticMarkup(
    React.createElement(VoiceCaptureFeedback, {
      voice: {
        ...idleVoice,
        state: 'listening',
        mode: 'live',
        liveText: 'Preview words',
        recordingSeconds: 8,
        audioLevel: 0.4,
        isListening: true,
        reviewSignals: [{ text: 'Friday', confidence: 0.52, startMs: 1200 }],
      },
    }),
  );

  assert.match(html, /role="status"/);
  assert.match(html, /00:08/);
  assert.match(html, /role="img" aria-label="Microphone audio level active"/);
  assert.match(html, /Preview words/);
  assert.match(html, /Check these details against your recording/);
  assert.match(html, /<bdi dir="auto">Friday<\/bdi>/);
  assert.doesNotMatch(html, /52%|confidence/);
});

test('voice transcript review is hidden before a completed transcript exists', () => {
  const html = renderToStaticMarkup(
    React.createElement(VoiceTranscriptReview, {
      id: 'voice-review',
      state: 'listening',
      transcript: 'Still being captured',
      value: '',
      onChange: () => {},
      children: React.createElement('button', null, 'Use transcript'),
    }),
  );

  assert.equal(html, '');
});

test('recorded audio URL cleanup stays attached to the shared in-memory feedback component', async () => {
  const componentPath = fileURLToPath(
    new URL('../components/voice/voice-session-ui.tsx', import.meta.url),
  );
  const source = await readFile(componentPath, 'utf8');

  assert.match(source, /URL\.createObjectURL\(voice\.recording\.blob\)/);
  assert.match(source, /URL\.revokeObjectURL\(url\)/);
  assert.match(source, /<audio[\s\S]*?controls/);
  assert.match(source, /onClick=\{\(\) => void voice\.retry\(\)\}/);
  assert.match(source, /onClick=\{voice\.clearRecording\}/);
});