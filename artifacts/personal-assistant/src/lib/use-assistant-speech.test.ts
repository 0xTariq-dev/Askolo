import assert from 'node:assert/strict';
import test from 'node:test';
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { Window as HappyWindow } from 'happy-dom';
import { AssistantSpeechControl } from '@/components/assistant-speech-control';
import { useAssistantSpeech } from '@/hooks/use-assistant-speech';

Object.assign(globalThis, { React });

class FakeUtterance {
  lang = '';
  voice: SpeechSynthesisVoice | null = null;
  onend: ((event: SpeechSynthesisEvent) => void) | null = null;
  onerror: ((event: SpeechSynthesisErrorEvent) => void) | null = null;

  constructor(readonly text: string) {}
}

class FakeSpeechSynthesis {
  spoken: FakeUtterance[] = [];
  cancelCalls = 0;
  pauseCalls = 0;
  resumeCalls = 0;

  constructor(private readonly voices: SpeechSynthesisVoice[]) {}

  speak(utterance: SpeechSynthesisUtterance) {
    this.spoken.push(utterance as unknown as FakeUtterance);
  }

  cancel() {
    this.cancelCalls += 1;
  }

  pause() {
    this.pauseCalls += 1;
  }

  resume() {
    this.resumeCalls += 1;
  }

  getVoices() {
    return this.voices;
  }
}

async function mountSpeechControl(options: {
  supported?: boolean;
  audioSupported?: boolean;
  language?: string;
  azureStatus?: number;
  azurePlaybackFails?: boolean;
  holdAzureResponse?: boolean;
} = {}) {
  const browser = new HappyWindow({ url: 'http://localhost/' });
  const previousGlobals = new Map<string, PropertyDescriptor | undefined>();
  const setGlobal = (name: string, value: unknown) => {
    if (!previousGlobals.has(name)) previousGlobals.set(name, Object.getOwnPropertyDescriptor(globalThis, name));
    Object.defineProperty(globalThis, name, { configurable: true, writable: true, value });
  };
  const voiceLanguage = options.language ?? 'en-US';
  const voices = [{
    voiceURI: `test-${voiceLanguage}`,
    name: `Test ${voiceLanguage}`,
    lang: voiceLanguage,
    localService: true,
    default: true,
  } as SpeechSynthesisVoice];
  const synthesis = new FakeSpeechSynthesis(voices);
  Object.defineProperty(browser, 'speechSynthesis', {
    configurable: true,
    value: options.supported === false ? undefined : synthesis,
  });

  setGlobal('window', browser);
  setGlobal('document', browser.document);
  setGlobal('navigator', browser.navigator);
  setGlobal('HTMLElement', browser.HTMLElement);
  setGlobal('Node', browser.Node);
  setGlobal('Event', browser.Event);
  setGlobal('MouseEvent', browser.MouseEvent);
  setGlobal('SpeechSynthesisUtterance', FakeUtterance);
  setGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const audioInstances: Array<{
    src: string;
    onended: (() => void) | null;
    onerror: (() => void) | null;
    playCalls: number;
    pauseCalls: number;
    play: () => Promise<void>;
    pause: () => void;
    removeAttribute: (name: string) => void;
    load: () => void;
  }> = [];
  class FakeAudio {
    onended: (() => void) | null = null;
    onerror: (() => void) | null = null;
    playCalls = 0;
    pauseCalls = 0;
    constructor(public src: string) { audioInstances.push(this); }
    play() {
      this.playCalls += 1;
      return options.azurePlaybackFails
        ? Promise.reject(new Error('playback failed'))
        : Promise.resolve();
    }
    pause() { this.pauseCalls += 1; }
    removeAttribute(name: string) { if (name === 'src') this.src = ''; }
    load() {}
  }
  setGlobal('Audio', options.audioSupported === false ? undefined : FakeAudio);
  const NativeURL = URL;
  class TestURL extends NativeURL {}
  Object.defineProperty(TestURL, 'createObjectURL', { value: () => 'blob:test-audio', configurable: true });
  Object.defineProperty(TestURL, 'revokeObjectURL', { value: () => undefined, configurable: true });
  setGlobal('URL', TestURL);
  let lastRequest: { input: RequestInfo | URL; init?: RequestInit } | null = null;
  setGlobal('fetch', (input: RequestInfo | URL, init?: RequestInit) => {
    lastRequest = { input, init };
    if (options.holdAzureResponse) {
      return new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true });
      });
    }
    const status = options.azureStatus ?? 502;
    const response = status === 200
      ? new Response(new Blob(['mp3'], { type: 'audio/mpeg' }), {
          status,
          headers: { 'Content-Type': 'audio/mpeg' },
        })
      : new Response(JSON.stringify({ code: 'VOICE_SYNTHESIS_FAILED', message: 'Speech unavailable' }), {
          status,
          headers: { 'Content-Type': 'application/json' },
        });
    return Promise.resolve(response);
  });

  let output: ReturnType<typeof useAssistantSpeech> | undefined;
  const container = browser.document.createElement('div');
  browser.document.body.append(container);
  const root = createRoot(container);

  function SpeechProbe() {
    const current = useAssistantSpeech();
    output = current;
    const active = current.activeMessageId === 'assistant-test';
    return React.createElement(AssistantSpeechControl, {
      status: active ? current.status : 'idle',
      error: active ? current.error : '',
      onSpeak: () => current.speak('assistant-test', 'Your team review is scheduled for Friday.', { runId: 'run-test' }),
      onPause: () => current.pause('assistant-test'),
      onResume: () => current.resume('assistant-test'),
      onStop: current.stop,
    });
  }

  await act(async () => {
    root.render(React.createElement(SpeechProbe));
  });

  return {
    browser,
    container,
    synthesis,
    audioInstances,
    get lastRequest() { return lastRequest; },
    get output() {
      assert.ok(output, 'speech hook has not rendered');
      return output;
    },
    async click(label: string) {
      const button = container.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`);
      assert.ok(button, `missing button with aria-label "${label}"`);
      await act(async () => {
        button.dispatchEvent(new browser.MouseEvent('click', {
          bubbles: true,
          cancelable: true,
          detail: 0,
        }));
        await new Promise((resolve) => setTimeout(resolve, 0));
      });
    },
    async cleanup() {
      await act(async () => root.unmount());
      await browser.happyDOM.abort();
      for (const [name, descriptor] of previousGlobals) {
        if (descriptor) Object.defineProperty(globalThis, name, descriptor);
        else Reflect.deleteProperty(globalThis, name);
      }
    },
  };
}

test('assistant Azure speech playback supports pause, resume, and stop', async () => {
  const harness = await mountSpeechControl({ language: 'en-US', azureStatus: 200 });
  try {
    await harness.click('Listen to response');
    assert.equal(harness.output.status, 'speaking');
    assert.equal(harness.audioInstances.length, 1);
    assert.equal(harness.audioInstances[0].playCalls, 1);
    assert.equal(harness.synthesis.spoken.length, 0);

    await harness.click('Pause response playback');
    assert.equal(harness.output.status, 'paused');
    assert.equal(harness.audioInstances[0].pauseCalls, 1);

    await harness.click('Resume response playback');
    assert.equal(harness.output.status, 'speaking');
    assert.equal(harness.audioInstances[0].playCalls, 2);

    await harness.click('Stop response playback');
    assert.equal(harness.output.status, 'idle');
    assert.equal(harness.output.activeMessageId, null);
    assert.equal(harness.audioInstances[0].pauseCalls, 2);
    assert.equal(harness.synthesis.spoken.length, 0);
  } finally {
    await harness.cleanup();
  }
});

test('Arabic assistant responses use Azure playback without browser speech fallback', async () => {
  const harness = await mountSpeechControl({ language: 'ar-EG', azureStatus: 200 });
  try {
    await act(async () => {
      await harness.output.speak('assistant-test', 'تم تحديد موعد الاجتماع يوم الجمعة.', { runId: 'run-test' });
    });

    assert.equal(harness.output.status, 'speaking');
    assert.equal(harness.audioInstances.length, 1);
    assert.equal(harness.synthesis.spoken.length, 0);
    assert.equal(String(harness.lastRequest?.input), '/api/ai/assistant/runs/run-test/speech');
  } finally {
    await harness.cleanup();
  }
});

test('missing Azure audio playback and oversized text produce accessible feedback', async () => {
  const unsupported = await mountSpeechControl({ audioSupported: false, azureStatus: 200 });
  try {
    await unsupported.click('Listen to response');
    assert.equal(unsupported.output.status, 'unsupported');
    assert.equal(unsupported.output.error, 'Azure speech playback is unavailable on this device.');
    assert.equal(unsupported.synthesis.spoken.length, 0);
  } finally {
    await unsupported.cleanup();
  }

  const longResponse = await mountSpeechControl();
  try {
    await act(async () => {
      await longResponse.output.speak('assistant-test', 'x'.repeat(12_001), { runId: 'run-test' });
    });
    assert.equal(longResponse.output.status, 'error');
    assert.equal(longResponse.container.querySelector('[role="status"]')?.textContent, 'This response is too long to read aloud.');
    assert.equal(longResponse.synthesis.spoken.length, 0);
  } finally {
    await longResponse.cleanup();
  }
});

test('Azure audio is primary and does not use browser speech on success', async () => {
  const harness = await mountSpeechControl({ azureStatus: 200 });
  try {
    await harness.click('Listen to response');
    assert.equal(harness.output.status, 'speaking');
    assert.equal(harness.audioInstances.length, 1);
    assert.equal(harness.audioInstances[0].playCalls, 1);
    assert.equal(harness.synthesis.spoken.length, 0);
    assert.equal(String(harness.lastRequest?.input), '/api/ai/assistant/runs/run-test/speech');
    assert.equal(harness.lastRequest?.init?.body, undefined);
  } finally {
    await harness.cleanup();
  }
});

test('Azure synthesis or playback failure does not start device speech', async () => {
  const synthesisFailure = await mountSpeechControl({ azureStatus: 502 });
  try {
    await synthesisFailure.click('Listen to response');
    assert.equal(synthesisFailure.output.status, 'error');
    assert.equal(synthesisFailure.synthesis.spoken.length, 0);
  } finally {
    await synthesisFailure.cleanup();
  }

  const playbackFailure = await mountSpeechControl({ azureStatus: 200, azurePlaybackFails: true });
  try {
    await playbackFailure.click('Listen to response');
    assert.equal(playbackFailure.output.status, 'error');
    assert.equal(playbackFailure.synthesis.spoken.length, 0);
  } finally {
    await playbackFailure.cleanup();
  }
});

test('authorization failures do not trigger device speech, and Stop cancels pending Azure requests', async () => {
  const denied = await mountSpeechControl({ azureStatus: 403 });
  try {
    await denied.click('Listen to response');
    assert.equal(denied.output.status, 'error');
    assert.equal(denied.synthesis.spoken.length, 0);
  } finally {
    await denied.cleanup();
  }

  const pending = await mountSpeechControl({ holdAzureResponse: true });
  try {
    await pending.click('Listen to response');
    assert.equal(pending.output.status, 'loading');
    await pending.click('Stop response playback');
    assert.equal(pending.output.status, 'idle');
    assert.equal(pending.synthesis.spoken.length, 0);
  } finally {
    await pending.cleanup();
  }
});

test('unmounting the assistant stops active Azure audio playback', async () => {
  const harness = await mountSpeechControl({ azureStatus: 200 });
  await harness.click('Listen to response');
  assert.equal(harness.output.status, 'speaking');

  await harness.cleanup();
  assert.equal(harness.audioInstances[0].pauseCalls, 1);
  assert.equal(harness.audioInstances[0].src, '');
  assert.equal(harness.synthesis.spoken.length, 0);
});