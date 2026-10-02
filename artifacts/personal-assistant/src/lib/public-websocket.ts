export interface PublicWebSocketEnvelope<TPayload = Record<string, unknown>> {
  version: number;
  sessionId: string;
  sequence: number;
  type: string;
  correlationId?: string;
  payload?: TPayload;
}

export type SequenceRef = { current: number };

export interface PublicWebSocketResumeState {
  previousSessionId: string;
  previousStatus: 'active' | 'closed' | 'expired';
  previousTerminalState: string;
  previousServerSequence: number;
  assistantRunIds: string[];
  voiceRestartRequired: boolean;
  replayAvailable: false;
  audioOrTranscriptStored: false;
  resumeAfterSequence: number;
  metadataOnlyResume: true;
}

const RESUME_CLOSE_TIMEOUT_MS = 750;

export function publicWebSocketURL(): string {
  const rawBasePath = import.meta.env?.BASE_URL ?? '/';
  const basePath = rawBasePath.endsWith('/') ? rawBasePath : `${rawBasePath}/`;
  const url = new URL(`${basePath}ws`, window.location.origin);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  return url.toString();
}

export function sendPublicWebSocketMessage(
  socket: WebSocket,
  sequenceRef: SequenceRef,
  type: string,
  payload: Record<string, unknown> = {},
): number {
  const sequence = sequenceRef.current + 1;
  sequenceRef.current = sequence;
  socket.send(JSON.stringify({ version: 1, type, sequence, payload }));
  return sequence;
}

export function resumePublicWebSocketMetadata(
  previousSessionId: string,
  afterSequence: number,
  timeoutMs = 5_000,
): Promise<PublicWebSocketResumeState> {
  if (!isSafeProtocolId(previousSessionId) ||
    !Number.isSafeInteger(afterSequence) || afterSequence < 0 ||
    !Number.isSafeInteger(timeoutMs) || timeoutMs < 1) {
    return Promise.reject(new Error('The previous connection state could not be checked.'));
  }

  return new Promise((resolve, reject) => {
    let socket: WebSocket;
    try {
      socket = new WebSocket(publicWebSocketURL());
    } catch {
      reject(new Error('The previous connection state could not be checked.'));
      return;
    }
    const clientSequence = { current: 0 };
    let currentSessionId = '';
    let serverSequence = 0;
    let ready = false;
    let resumeState: PublicWebSocketResumeState | null = null;
    let settled = false;
    let timeout = 0;
    let closeTimeout = 0;

    const cleanup = () => {
      window.clearTimeout(timeout);
      window.clearTimeout(closeTimeout);
      socket.removeEventListener('open', onOpen);
      socket.removeEventListener('message', onMessage);
      socket.removeEventListener('error', onError);
      socket.removeEventListener('close', onClose);
      if (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING) {
        try {
          socket.close();
        } catch {
          // The status result remains usable even if the temporary socket has already closed.
        }
      }
    };

    const finish = (error?: Error) => {
      if (settled) return;
      settled = true;
      cleanup();
      if (error) reject(error);
      else if (resumeState) resolve(resumeState);
      else reject(new Error('The previous connection state could not be checked.'));
    };

    const onOpen = () => {
      try {
        sendPublicWebSocketMessage(socket, clientSequence, 'session.start', {
          resumeSessionId: previousSessionId,
          afterSequence,
        });
      } catch {
        finish(new Error('The previous connection state could not be checked.'));
      }
    };

    const onMessage = (event: MessageEvent) => {
      if (typeof event.data !== 'string') {
        finish(new Error('The previous connection state could not be checked.'));
        return;
      }
      const envelope = parsePublicWebSocketEnvelope<Record<string, unknown>>(event.data);
      if (!envelope || envelope.sequence < 1 ||
        (currentSessionId !== '' && envelope.sessionId !== currentSessionId)) {
        finish(new Error('The previous connection state could not be checked.'));
        return;
      }
      currentSessionId = envelope.sessionId;
      if (envelope.sequence <= serverSequence) return;
      if (envelope.sequence !== serverSequence + 1) {
        finish(new Error('The previous connection state could not be checked.'));
        return;
      }
      serverSequence = envelope.sequence;

      if (envelope.type === 'protocol.error') {
        finish(new Error('The previous connection state could not be checked.'));
        return;
      }
      if (envelope.type === 'session.ready') {
        if (envelope.payload?.resumeMode !== 'metadata_only') {
          finish(new Error('The previous connection state could not be checked.'));
          return;
        }
        ready = true;
        return;
      }
      if (envelope.type === 'session.resume_state') {
        if (!ready || resumeState) {
          finish(new Error('The previous connection state could not be checked.'));
          return;
        }
        const parsed = parseResumeState(envelope.payload, previousSessionId, afterSequence);
        if (!parsed) {
          finish(new Error('The previous connection state could not be checked.'));
          return;
        }
        resumeState = parsed;
        try {
          sendPublicWebSocketMessage(socket, clientSequence, 'session.close', { reason: 'completed' });
        } catch {
          finish();
          return;
        }
        closeTimeout = window.setTimeout(() => finish(), RESUME_CLOSE_TIMEOUT_MS);
        return;
      }
      if (envelope.type === 'session.closed' && resumeState) {
        finish();
      }
    };

    const onError = () => finish(new Error('The previous connection state could not be checked.'));
    const onClose = () => finish();

    timeout = window.setTimeout(() => finish(), timeoutMs);
    socket.addEventListener('open', onOpen, { once: true });
    socket.addEventListener('message', onMessage);
    socket.addEventListener('error', onError, { once: true });
    socket.addEventListener('close', onClose, { once: true });
  });
}

export function sendPublicWebSocketAudio(
  socket: WebSocket,
  sequenceRef: SequenceRef,
  audio: ArrayBuffer,
): void {
  const sequence = sequenceRef.current + 1;
  sequenceRef.current = sequence;
  const frame = new Uint8Array(8 + audio.byteLength);
  const view = new DataView(frame.buffer);
  view.setUint32(0, Math.floor(sequence / 0x1_0000_0000), false);
  view.setUint32(4, sequence >>> 0, false);
  frame.set(new Uint8Array(audio), 8);
  socket.send(frame);
}

export function parsePublicWebSocketEnvelope<TPayload = Record<string, unknown>>(
  data: string,
): PublicWebSocketEnvelope<TPayload> | null {
  try {
    const value = JSON.parse(data) as Partial<PublicWebSocketEnvelope<TPayload>>;
    if (value.version !== 1 || typeof value.type !== 'string' ||
      typeof value.sessionId !== 'string' || typeof value.sequence !== 'number' ||
      !Number.isSafeInteger(value.sequence) || value.sequence < 0) {
      return null;
    }
    return value as PublicWebSocketEnvelope<TPayload>;
  } catch {
    return null;
  }
}

function parseResumeState(
  value: unknown,
  requestedSessionId: string,
  afterSequence: number,
): PublicWebSocketResumeState | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
  const payload = value as Record<string, unknown>;
  const allowedKeys = new Set([
    'previousSessionId',
    'previousStatus',
    'previousTerminalState',
    'previousServerSequence',
    'assistantRunIds',
    'voiceRestartRequired',
    'replayAvailable',
    'audioOrTranscriptStored',
    'resumeAfterSequence',
    'metadataOnlyResume',
  ]);
  const runIds = payload.assistantRunIds;
  if (Object.keys(payload).some((key) => !allowedKeys.has(key)) ||
    payload.previousSessionId !== requestedSessionId ||
    !['active', 'closed', 'expired'].includes(String(payload.previousStatus)) ||
    typeof payload.previousTerminalState !== 'string' ||
    payload.previousTerminalState.length > 32 ||
    !Number.isSafeInteger(payload.previousServerSequence) ||
    (payload.previousServerSequence as number) < afterSequence ||
    !Array.isArray(runIds) || runIds.length > 128 ||
    !runIds.every((id) => typeof id === 'string' && isSafeProtocolId(id)) ||
    typeof payload.voiceRestartRequired !== 'boolean' ||
    payload.replayAvailable !== false ||
    payload.audioOrTranscriptStored !== false ||
    payload.resumeAfterSequence !== afterSequence ||
    payload.metadataOnlyResume !== true) {
    return null;
  }

  return {
    previousSessionId: requestedSessionId,
    previousStatus: payload.previousStatus as PublicWebSocketResumeState['previousStatus'],
    previousTerminalState: payload.previousTerminalState,
    previousServerSequence: payload.previousServerSequence as number,
    assistantRunIds: [...new Set(runIds as string[])],
    voiceRestartRequired: payload.voiceRestartRequired,
    replayAvailable: false,
    audioOrTranscriptStored: false,
    resumeAfterSequence: afterSequence,
    metadataOnlyResume: true,
  };
}

function isSafeProtocolId(value: string): boolean {
  return value.length > 0 && value.length <= 128 && /^[A-Za-z0-9._-]+$/.test(value);
}

export function waitForPublicWebSocketEvent(
  socket: WebSocket,
  eventType: string,
  timeoutMs = 3_500,
): Promise<void> {
  return new Promise((resolve) => {
    let timeout = 0;
    const finish = () => {
      window.clearTimeout(timeout);
      socket.removeEventListener('message', onMessage);
      socket.removeEventListener('close', finish);
      resolve();
    };
    const onMessage = (event: MessageEvent) => {
      if (typeof event.data !== 'string') return;
      const envelope = parsePublicWebSocketEnvelope(event.data);
      if (envelope?.type === eventType || envelope?.type === 'protocol.error') finish();
    };
    timeout = window.setTimeout(finish, timeoutMs);
    socket.addEventListener('message', onMessage);
    socket.addEventListener('close', finish, { once: true });
  });
}