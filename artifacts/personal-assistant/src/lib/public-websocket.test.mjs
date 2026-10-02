import assert from 'node:assert/strict';
import test from 'node:test';
import { resumePublicWebSocketMetadata } from './public-websocket.ts';

class TestWebSocket extends EventTarget {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  readyState = TestWebSocket.CONNECTING;
  sent = [];

  send(value) {
    this.sent.push(JSON.parse(value));
  }

  open() {
    this.readyState = TestWebSocket.OPEN;
    this.dispatchEvent(new Event('open'));
  }

  message(envelope) {
    const event = new Event('message');
    Object.defineProperty(event, 'data', { value: JSON.stringify(envelope) });
    this.dispatchEvent(event);
  }

  close() {
    if (this.readyState === TestWebSocket.CLOSED) return;
    this.readyState = TestWebSocket.CLOSED;
    this.dispatchEvent(new Event('close'));
  }
}

function installBrowserStubs(t) {
  const previousWindow = globalThis.window;
  const previousWebSocket = globalThis.WebSocket;
  const sockets = [];
  globalThis.window = {
    location: { origin: 'https://askolo.example' },
    setTimeout,
    clearTimeout,
  };
  globalThis.WebSocket = class extends TestWebSocket {
    constructor(url) {
      super();
      this.url = url;
      sockets.push(this);
    }
  };
  t.after(() => {
    globalThis.window = previousWindow;
    globalThis.WebSocket = previousWebSocket;
  });
  return sockets;
}

test('metadata resume requests a bounded status check and closes its temporary session', async (t) => {
  const sockets = installBrowserStubs(t);
  const previousSessionId = 'previous-session_1';
  const request = resumePublicWebSocketMetadata(previousSessionId, 4, 1_000);
  const socket = sockets[0];

  socket.open();
  assert.deepEqual(socket.sent[0], {
    version: 1,
    type: 'session.start',
    sequence: 1,
    payload: { resumeSessionId: previousSessionId, afterSequence: 4 },
  });

  socket.message({
    version: 1,
    sessionId: 'recovery-session',
    sequence: 1,
    type: 'session.ready',
    payload: { resumeMode: 'metadata_only' },
  });
  socket.message({
    version: 1,
    sessionId: 'recovery-session',
    sequence: 2,
    type: 'session.resume_state',
    payload: {
      previousSessionId,
      previousStatus: 'closed',
      previousTerminalState: 'cancelled',
      previousServerSequence: 9,
      assistantRunIds: ['run-1', 'run-1'],
      voiceRestartRequired: true,
      replayAvailable: false,
      audioOrTranscriptStored: false,
      resumeAfterSequence: 4,
      metadataOnlyResume: true,
    },
  });

  assert.deepEqual(socket.sent[1], {
    version: 1,
    type: 'session.close',
    sequence: 2,
    payload: { reason: 'completed' },
  });
  socket.message({
    version: 1,
    sessionId: 'recovery-session',
    sequence: 3,
    type: 'session.closed',
    payload: { reason: 'completed' },
  });

  assert.deepEqual(await request, {
    previousSessionId,
    previousStatus: 'closed',
    previousTerminalState: 'cancelled',
    previousServerSequence: 9,
    assistantRunIds: ['run-1'],
    voiceRestartRequired: true,
    replayAvailable: false,
    audioOrTranscriptStored: false,
    resumeAfterSequence: 4,
    metadataOnlyResume: true,
  });
  assert.equal(socket.readyState, TestWebSocket.CLOSED);
});

test('metadata resume rejects any response that suggests content replay or storage', async (t) => {
  const sockets = installBrowserStubs(t);
  const request = resumePublicWebSocketMetadata('previous-session', 0, 1_000);
  const socket = sockets[0];
  socket.open();
  socket.message({
    version: 1,
    sessionId: 'recovery-session',
    sequence: 1,
    type: 'session.ready',
    payload: { resumeMode: 'metadata_only' },
  });
  socket.message({
    version: 1,
    sessionId: 'recovery-session',
    sequence: 2,
    type: 'session.resume_state',
    payload: {
      previousSessionId: 'previous-session',
      previousStatus: 'closed',
      previousTerminalState: 'cancelled',
      previousServerSequence: 2,
      assistantRunIds: [],
      voiceRestartRequired: false,
      replayAvailable: true,
      audioOrTranscriptStored: false,
      resumeAfterSequence: 0,
      metadataOnlyResume: true,
    },
  });

  await assert.rejects(request, /state could not be checked/);
  assert.equal(socket.readyState, TestWebSocket.CLOSED);
});

test('metadata resume rejects unexpected transcript fields', async (t) => {
  const sockets = installBrowserStubs(t);
  const request = resumePublicWebSocketMetadata('previous-session', 0, 1_000);
  const socket = sockets[0];
  socket.open();
  socket.message({
    version: 1,
    sessionId: 'recovery-session',
    sequence: 1,
    type: 'session.ready',
    payload: { resumeMode: 'metadata_only' },
  });
  socket.message({
    version: 1,
    sessionId: 'recovery-session',
    sequence: 2,
    type: 'session.resume_state',
    payload: {
      previousSessionId: 'previous-session',
      previousStatus: 'closed',
      previousTerminalState: 'cancelled',
      previousServerSequence: 2,
      assistantRunIds: [],
      voiceRestartRequired: false,
      replayAvailable: false,
      audioOrTranscriptStored: false,
      resumeAfterSequence: 0,
      metadataOnlyResume: true,
      transcript: 'must never be replayed',
    },
  });

  await assert.rejects(request, /state could not be checked/);
  assert.equal(socket.readyState, TestWebSocket.CLOSED);
});

test('metadata resume rejects a gap in the recovery session sequence', async (t) => {
  const sockets = installBrowserStubs(t);
  const request = resumePublicWebSocketMetadata('previous-session', 0, 1_000);
  const socket = sockets[0];
  socket.open();
  socket.message({
    version: 1,
    sessionId: 'recovery-session',
    sequence: 1,
    type: 'session.ready',
    payload: { resumeMode: 'metadata_only' },
  });
  socket.message({
    version: 1,
    sessionId: 'recovery-session',
    sequence: 3,
    type: 'session.resume_state',
    payload: {
      previousSessionId: 'previous-session',
      previousStatus: 'closed',
      previousTerminalState: 'cancelled',
      previousServerSequence: 2,
      assistantRunIds: [],
      voiceRestartRequired: false,
      replayAvailable: false,
      audioOrTranscriptStored: false,
      resumeAfterSequence: 0,
      metadataOnlyResume: true,
    },
  });

  await assert.rejects(request, /state could not be checked/);
  assert.equal(socket.readyState, TestWebSocket.CLOSED);
});

test('metadata resume rejects mismatched and unsafe resume identifiers', async (t) => {
  installBrowserStubs(t);
  await assert.rejects(
    resumePublicWebSocketMetadata('invalid session id', 0),
    /state could not be checked/,
  );
  await assert.rejects(
    resumePublicWebSocketMetadata('previous-session', -1),
    /state could not be checked/,
  );
});