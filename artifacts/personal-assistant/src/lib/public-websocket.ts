export interface PublicWebSocketEnvelope<TPayload = Record<string, unknown>> {
  version: number;
  sessionId: string;
  sequence: number;
  type: string;
  correlationId?: string;
  payload?: TPayload;
}

export type SequenceRef = { current: number };

export function publicWebSocketURL(): string {
  const rawBasePath = import.meta.env.BASE_URL ?? '/';
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