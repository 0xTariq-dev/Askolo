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
  language?: string;
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
      onSpeak: () => current.speak('assistant-test', 'Your team review is scheduled for Friday.'),
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

test('assistant speech playback uses a matching voice and supports pause, resume, and stop', async () => {
  const harness = await mountSpeechControl({ language: 'en-US' });
  try {
    await harness.click('Listen to response');
    assert.equal(harness.output.status, 'speaking');
    assert.equal(harness.synthesis.spoken.length, 1);
    assert.equal(harness.synthesis.spoken[0].text, 'Your team review is scheduled for Friday.');
    assert.equal(harness.synthesis.spoken[0].lang, 'en-US');
    assert.equal(harness.synthesis.spoken[0].voice?.lang, 'en-US');

    await harness.click('Pause response playback');
    assert.equal(harness.output.status, 'paused');
    assert.equal(harness.synthesis.pauseCalls, 1);

    await harness.click('Resume response playback');
    assert.equal(harness.output.status, 'speaking');
    assert.equal(harness.synthesis.resumeCalls, 1);

    await harness.click('Stop response playback');
    assert.equal(harness.output.status, 'idle');
    assert.equal(harness.output.activeMessageId, null);
    assert.equal(harness.synthesis.cancelCalls, 1);
  } finally {
    await harness.cleanup();
  }
});

test('Arabic assistant responses select an Arabic browser voice when available', async () => {
  const harness = await mountSpeechControl({ language: 'ar-EG' });
  try {
    await act(async () => {
      harness.output.speak('assistant-test', 'تم تحديد موعد الاجتماع يوم الجمعة.');
    });

    assert.equal(harness.output.status, 'speaking');
    assert.equal(harness.synthesis.spoken[0].lang, 'ar');
    assert.equal(harness.synthesis.spoken[0].voice?.lang, 'ar-EG');
  } finally {
    await harness.cleanup();
  }
});

test('missing browser speech support and oversized text produce accessible feedback', async () => {
  const unsupported = await mountSpeechControl({ supported: false });
  try {
    await unsupported.click('Listen to response');
    assert.equal(unsupported.output.status, 'unsupported');
    assert.equal(unsupported.container.querySelector('[role="status"]')?.textContent, 'Speech playback is unavailable in this browser.');
    assert.equal(unsupported.synthesis.spoken.length, 0);
  } finally {
    await unsupported.cleanup();
  }

  const longResponse = await mountSpeechControl();
  try {
    await act(async () => {
      longResponse.output.speak('assistant-test', 'x'.repeat(12_001));
    });
    assert.equal(longResponse.output.status, 'error');
    assert.equal(longResponse.container.querySelector('[role="status"]')?.textContent, 'This response is too long to read aloud.');
    assert.equal(longResponse.synthesis.spoken.length, 0);
  } finally {
    await longResponse.cleanup();
  }
});

test('unmounting the assistant stops an active speech utterance', async () => {
  const harness = await mountSpeechControl();
  await harness.click('Listen to response');
  assert.equal(harness.output.status, 'speaking');

  await harness.cleanup();
  assert.equal(harness.synthesis.cancelCalls, 1);
});