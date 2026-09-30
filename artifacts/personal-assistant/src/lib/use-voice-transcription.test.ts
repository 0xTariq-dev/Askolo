import assert from 'node:assert/strict';
import test from 'node:test';
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { Window as HappyWindow } from 'happy-dom';
import { useVoiceTranscription } from '@/hooks/use-voice-transcription';
import {
  VoiceCaptureButton,
  VoiceCaptureFeedback,
  VoiceTranscriptReview,
} from '@/components/voice/voice-session-ui';

Object.assign(globalThis, { React });

type VoiceHook = ReturnType<typeof useVoiceTranscription>;
type RequestRecord = {
  url: string;
  method: string;
  body: string;
  headers: Headers;
  credentials: RequestCredentials | undefined;
};

const transcriptResponse = {
  transcript: 'Plan the team review for Friday at 3 PM',
  confidence: 0.96,
  reviewSignals: [],
  deletion: {
    rawAudio: 'not_stored',
    providerTranscript: 'deleted',
    marker: 'raw_audio_not_stored_provider_transcript_deleted',
  },
  creditReceipt: {
    id: 'credit-receipt-test-1',
    reservationId: 'reservation-test-1',
    operationType: 'voice',
    provider: 'assemblyai',
    mode: 'recorded',
    status: 'settled',
    reservedCredits: 100,
    settledCredits: 80,
    refundedCredits: 20,
    balance: 920,
    policyVersion: 1,
    reservedUsdMicros: 100_000,
    settledUsdMicros: 80_000,
    refundedUsdMicros: 20_000,
    balanceUsdMicros: 920_000,
  },
};

class FakeAudioContext {
  static instances: FakeAudioContext[] = [];

  state: AudioContextState = 'running';
  readonly sampleRate = 16_000;

  constructor() {
    FakeAudioContext.instances.push(this);
  }

  createMediaStreamSource() {
    return { connect() {}, disconnect() {} };
  }

  createAnalyser() {
    return {
      fftSize: 0,
      smoothingTimeConstant: 0,
      getByteTimeDomainData(samples: Uint8Array) {
        samples.fill(128);
      },
    };
  }

  async decodeAudioData() {
    const samples = new Float32Array(this.sampleRate).fill(0.1);
    return {
      sampleRate: this.sampleRate,
      getChannelData: () => samples,
    } as unknown as AudioBuffer;
  }

  async resume() {}

  async close() {
    this.state = 'closed';
  }
}

class FakeMediaRecorder {
  static instances: FakeMediaRecorder[] = [];
  static isTypeSupported(type: string) {
    return type === 'audio/webm;codecs=opus';
  }

  readonly mimeType: string;
  state: RecordingState = 'inactive';
  startTimeslice: number | undefined;
  ondataavailable: ((event: BlobEvent) => void) | null = null;
  onstop: ((event: Event) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  private stopCompletion: Promise<unknown> = Promise.resolve();

  constructor(_stream: MediaStream, options?: MediaRecorderOptions) {
    this.mimeType = options?.mimeType ?? 'audio/webm;codecs=opus';
    FakeMediaRecorder.instances.push(this);
  }

  start(timeslice?: number) {
    this.startTimeslice = timeslice;
    this.state = 'recording';
  }

  stop() {
    if (this.state !== 'recording') return;
    this.state = 'inactive';
    const data = new Blob(['synthetic captured audio'], { type: this.mimeType });
    this.ondataavailable?.({ data } as BlobEvent);
    this.stopCompletion = Promise.resolve(this.onstop?.(new Event('stop')));
  }

  whenStopped() {
    return this.stopCompletion;
  }
}

async function mountCaptureUI(options: {
  canReserve?: boolean;
  getUserMedia: (constraints: MediaStreamConstraints) => Promise<MediaStream>;
}) {
  const browser = new HappyWindow({ url: 'http://localhost/' });
  const previousGlobals = new Map<string, PropertyDescriptor | undefined>();
  const globalNames = [
    'window',
    'document',
    'navigator',
    'HTMLElement',
    'Node',
    'Event',
    'MouseEvent',
    'PointerEvent',
    'KeyboardEvent',
    'DOMException',
    'MediaRecorder',
    'AudioContext',
    'requestAnimationFrame',
    'cancelAnimationFrame',
    'IS_REACT_ACT_ENVIRONMENT',
    'fetch',
  ];

  const setGlobal = (name: string, value: unknown) => {
    if (!previousGlobals.has(name)) previousGlobals.set(name, Object.getOwnPropertyDescriptor(globalThis, name));
    Object.defineProperty(globalThis, name, { configurable: true, writable: true, value });
  };

  FakeAudioContext.instances = [];
  FakeMediaRecorder.instances = [];
  Object.defineProperty(browser, 'AudioContext', { configurable: true, value: FakeAudioContext });
  Object.defineProperty(browser, 'MediaRecorder', { configurable: true, value: FakeMediaRecorder });
  Object.defineProperty(browser, 'requestAnimationFrame', { configurable: true, value: () => 1 });
  Object.defineProperty(browser, 'cancelAnimationFrame', { configurable: true, value: () => {} });
  Object.defineProperty(browser.navigator, 'mediaDevices', {
    configurable: true,
    value: { getUserMedia: options.getUserMedia },
  });

  const requests: RequestRecord[] = [];
  const estimate = {
    pricingKey: 'voice.recorded',
    units: 60,
    unit: 'seconds',
    estimatedUsdMicros: 80_000,
    hardCapUsdMicros: 100_000,
    availableUsdMicros: options.canReserve === false ? 0 : 2_500_000,
    policyVersion: 1,
    canReserve: options.canReserve !== false,
    overrunMarginPercent: 10,
    currency: 'USD',
  };
  const fetchStub: typeof fetch = async (input, init) => {
    const rawUrl = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
    const url = new URL(rawUrl, 'http://localhost/');
    const method = init?.method ?? (typeof input === 'string' || input instanceof URL ? 'GET' : input.method);
    const body = typeof init?.body === 'string' ? init.body : '';
    requests.push({
      url: url.href,
      method,
      body,
      headers: new Headers(init?.headers),
      credentials: init?.credentials,
    });

    if (url.pathname === '/api/ai/credits/estimate') {
      return new Response(JSON.stringify(estimate), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    if (url.pathname === '/api/ai/transcribe-audio') {
      return new Response(JSON.stringify(transcriptResponse), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    return new Response(JSON.stringify({ message: `Unexpected test request: ${url.pathname}` }), {
      status: 404,
      headers: { 'Content-Type': 'application/json' },
    });
  };

  setGlobal('window', browser);
  setGlobal('document', browser.document);
  setGlobal('navigator', browser.navigator);
  setGlobal('HTMLElement', browser.HTMLElement);
  setGlobal('Node', browser.Node);
  setGlobal('Event', browser.Event);
  setGlobal('MouseEvent', browser.MouseEvent);
  setGlobal('PointerEvent', browser.PointerEvent);
  setGlobal('KeyboardEvent', browser.KeyboardEvent);
  setGlobal('DOMException', browser.DOMException);
  setGlobal('MediaRecorder', FakeMediaRecorder);
  setGlobal('AudioContext', FakeAudioContext);
  setGlobal('requestAnimationFrame', () => 1);
  setGlobal('cancelAnimationFrame', () => {});
  setGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  setGlobal('fetch', fetchStub);

  let voice: VoiceHook | undefined;
  let pendingStart: Promise<void> | undefined;
  const container = browser.document.createElement('div');
  browser.document.body.append(container);
  const root = createRoot(container);

  function CaptureProbe() {
    const current = useVoiceTranscription();
    voice = current;
    return React.createElement(
      'section',
      null,
      React.createElement(VoiceCaptureFeedback, { voice: current }),
      React.createElement(VoiceCaptureButton, {
        voice: current,
        onStart: () => {
          pendingStart = current.start();
          return pendingStart;
        },
        startText: 'Press and hold or activate to record voice input',
        stopText: 'Recording voice input; release or activate to stop',
        disabled: current.state === 'starting' || current.state === 'processing',
        'data-testid': 'button-voice-record',
      }),
      React.createElement(VoiceTranscriptReview, {
        id: 'captured-transcript',
        state: current.state,
        transcript: current.transcript,
        value: current.transcript,
        onChange: () => {},
        label: 'Review captured transcript',
      }),
    );
  }

  await act(async () => {
    root.render(React.createElement(CaptureProbe));
  });

  return {
    browser,
    container,
    requests,
    get voice() {
      assert.ok(voice, 'capture hook has not rendered');
      return voice;
    },
    get pendingStart() {
      return pendingStart;
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

function clickForAssistiveTechnology(browser: HappyWindow, button: HTMLButtonElement) {
  button.dispatchEvent(new browser.MouseEvent('click', {
    bubbles: true,
    cancelable: true,
    detail: 0,
  }));
}

test('microphone capture button records, transcribes, and presents a reviewable transcript', async () => {
  let constraints: MediaStreamConstraints | undefined;
  let trackStopCalls = 0;
  const track = { stop: () => { trackStopCalls += 1; } };
  const stream = { getTracks: () => [track] } as unknown as MediaStream;
  const harness = await mountCaptureUI({
    getUserMedia: async (requested) => {
      constraints = requested;
      return stream;
    },
  });
  const originalDateNow = Date.now;
  let fakeNow = 1_700_000_000_000;
  Date.now = () => fakeNow;

  try {
    let button = harness.container.querySelector<HTMLButtonElement>('[data-testid="button-voice-record"]');
    assert.ok(button);
    assert.equal(button.getAttribute('aria-label'), 'Press and hold or activate to record voice input');
    assert.equal(button.getAttribute('aria-pressed'), 'false');

    await act(async () => {
      clickForAssistiveTechnology(harness.browser, button!);
      assert.ok(harness.pendingStart, 'button did not start the voice session');
      await harness.pendingStart;
    });

    button = harness.container.querySelector<HTMLButtonElement>('[data-testid="button-voice-record"]');
    assert.equal(harness.voice.state, 'listening');
    assert.equal(harness.voice.mode, 'recorded');
    assert.equal(harness.voice.isListening, true);
    assert.deepEqual(constraints, { audio: true });
    assert.equal(FakeMediaRecorder.instances.length, 1);
    assert.equal(FakeMediaRecorder.instances[0].startTimeslice, 1000);
    assert.equal(button?.getAttribute('aria-pressed'), 'true');
    assert.equal(button?.getAttribute('aria-label'), 'Recording voice input; release or activate to stop');

    fakeNow += 900;
    await act(async () => {
      clickForAssistiveTechnology(harness.browser, button!);
      await FakeMediaRecorder.instances[0].whenStopped();
    });

    assert.equal(harness.voice.state, 'review');
    assert.equal(harness.voice.transcript, transcriptResponse.transcript);
    assert.equal(harness.voice.recording?.mimeType, 'audio/webm;codecs=opus');
    assert.equal(harness.voice.deletionStatus?.providerTranscript, 'deleted');
    assert.equal(trackStopCalls, 1, 'microphone track was not stopped');
    assert.ok(FakeAudioContext.instances.length >= 2, 'capture and speech-validation contexts were not both created');
    assert.ok(FakeAudioContext.instances.every((context) => context.state === 'closed'), 'audio contexts were not closed');
    assert.equal(harness.container.querySelector<HTMLTextAreaElement>('#captured-transcript')?.value, transcriptResponse.transcript);

    const estimateRequests = harness.requests.filter((request) => new URL(request.url).pathname === '/api/ai/credits/estimate');
    const transcriptionRequests = harness.requests.filter((request) => new URL(request.url).pathname === '/api/ai/transcribe-audio');
    assert.equal(estimateRequests.length, 2);
    assert.equal(transcriptionRequests.length, 1);
    const transcriptionRequest = transcriptionRequests[0];
    const payload = JSON.parse(transcriptionRequest.body) as {
      audioBase64: string;
      mimeType: string;
      durationMs: number;
      language: string;
    };
    assert.match(payload.audioBase64, /^[A-Za-z0-9+/]+=*$/);
    assert.equal(payload.mimeType, 'audio/webm');
    assert.ok(payload.durationMs >= 700, `recorded duration ${payload.durationMs}ms was below the speech gate`);
    assert.equal(payload.language, 'en-US');
    assert.equal(transcriptionRequest.credentials, 'include');
    assert.ok(transcriptionRequest.headers.get('Idempotency-Key'));
    assert.equal(transcriptionRequest.headers.get('X-AI-Credit-Policy-Version'), '1');
  } finally {
    Date.now = originalDateNow;
    await harness.cleanup();
  }
});

test('microphone permission denial is shown in the capture UI without transcription', async () => {
  let getUserMediaCalls = 0;
  const harness = await mountCaptureUI({
    getUserMedia: async () => {
      getUserMediaCalls += 1;
      throw new harness.browser.DOMException('permission denied', 'NotAllowedError');
    },
  });

  try {
    const button = harness.container.querySelector<HTMLButtonElement>('[data-testid="button-voice-record"]');
    assert.ok(button);
    await act(async () => {
      clickForAssistiveTechnology(harness.browser, button!);
      assert.ok(harness.pendingStart);
      await harness.pendingStart;
    });

    assert.equal(harness.voice.state, 'error');
    assert.equal(harness.voice.error, 'Microphone permission was denied. Allow microphone access or type instead.');
    assert.match(harness.container.textContent ?? '', /Microphone permission was denied/);
    assert.equal(getUserMediaCalls, 1);
    assert.equal(harness.requests.filter((request) => new URL(request.url).pathname === '/api/ai/transcribe-audio').length, 0);
    assert.equal(harness.requests.filter((request) => new URL(request.url).pathname === '/api/ai/credits/estimate').length, 1);
  } finally {
    await harness.cleanup();
  }
});

test('insufficient voice credits stop capture before the browser requests microphone access', async () => {
  let getUserMediaCalls = 0;
  const harness = await mountCaptureUI({
    canReserve: false,
    getUserMedia: async () => {
      getUserMediaCalls += 1;
      throw new Error('getUserMedia must not be called without available credits');
    },
  });

  try {
    const button = harness.container.querySelector<HTMLButtonElement>('[data-testid="button-voice-record"]');
    assert.ok(button);
    await act(async () => {
      clickForAssistiveTechnology(harness.browser, button!);
      assert.ok(harness.pendingStart);
      await harness.pendingStart;
    });

    assert.equal(harness.voice.state, 'error');
    assert.match(harness.voice.error, /are available/);
    assert.equal(getUserMediaCalls, 0);
    assert.equal(harness.requests.filter((request) => new URL(request.url).pathname === '/api/ai/transcribe-audio').length, 0);
    assert.equal(harness.requests.filter((request) => new URL(request.url).pathname === '/api/ai/credits/estimate').length, 1);
  } finally {
    await harness.cleanup();
  }
});