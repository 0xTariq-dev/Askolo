import type { AssistantRun } from '@workspace/api-client-react';
import {
  parsePublicWebSocketEnvelope,
  publicWebSocketURL,
  sendPublicWebSocketMessage,
} from '@/lib/public-websocket';

export interface AssistantRunWebSocketInput {
  conversationId?: string;
  transcript: string;
  idempotencyKey: string;
  policyVersion: number;
}

const CONNECTION_TIMEOUT_MS = 45_000;
const SESSION_CLOSE_TIMEOUT_MS = 3_500;

export function createAssistantRunOverWebSocket(
  input: AssistantRunWebSocketInput,
  signal?: AbortSignal,
): Promise<AssistantRun> {
  if (signal?.aborted) {
    return Promise.reject(new Error('Request stopped. No action was taken.'));
  }

  const socket = new WebSocket(publicWebSocketURL());
  const clientSequence = { current: 0 };
  let serverSequence = 0;
  let ready = false;
  let closing = false;
  let settled = false;
  let result: AssistantRun | null = null;
  let timeout = 0;
  let closeTimeout = 0;

  return new Promise<AssistantRun>((resolve, reject) => {
    const cleanup = () => {
      window.clearTimeout(timeout);
      window.clearTimeout(closeTimeout);
      socket.removeEventListener('open', onOpen);
      socket.removeEventListener('message', onMessage);
      socket.removeEventListener('error', onError);
      socket.removeEventListener('close', onClose);
      signal?.removeEventListener('abort', onAbort);
    };

    const settle = (error?: Error) => {
      if (settled) return;
      settled = true;
      cleanup();
      if (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING) {
        socket.close();
      }
      if (error) reject(error);
      else if (result) resolve(result);
      else reject(new Error('The assistant did not return a result.'));
    };

    const fail = (message: string) => settle(new Error(message));

    const closeSession = (reason: 'completed' | 'cancelled') => {
      if (closing || socket.readyState !== WebSocket.OPEN) {
        if (result) settle();
        else if (reason === 'cancelled') fail('Request stopped. No action was taken.');
        return;
      }
      closing = true;
      sendPublicWebSocketMessage(socket, clientSequence, 'session.close', { reason });
      closeTimeout = window.setTimeout(() => {
        if (reason === 'completed' && result) settle();
        else fail('The assistant connection closed before the request was confirmed.');
      }, SESSION_CLOSE_TIMEOUT_MS);
    };

    const onOpen = () => {
      sendPublicWebSocketMessage(socket, clientSequence, 'session.start');
    };

    const onMessage = (event: MessageEvent) => {
      if (typeof event.data !== 'string') {
        fail('The assistant connection returned an invalid message.');
        return;
      }
      const envelope = parsePublicWebSocketEnvelope<Record<string, unknown>>(event.data);
      if (!envelope) {
        fail('The assistant connection returned an invalid message.');
        return;
      }
      if (envelope.type === 'protocol.error') {
        const message = typeof envelope.payload?.message === 'string'
          ? envelope.payload.message
          : 'The assistant could not complete this request.';
        fail(message);
        return;
      }
      if (envelope.sequence > 0) {
        if (envelope.sequence <= serverSequence) return;
        if (serverSequence > 0 && envelope.sequence !== serverSequence + 1) {
          fail('The assistant connection missed an event. Retry the request.');
          return;
        }
        serverSequence = envelope.sequence;
      }
      if (envelope.type === 'session.ready') {
        if (ready) return;
        ready = true;
        sendPublicWebSocketMessage(socket, clientSequence, 'assistant.run.create', {
          conversationId: input.conversationId,
          transcript: input.transcript,
          idempotencyKey: input.idempotencyKey,
          policyVersion: input.policyVersion,
        });
        return;
      }
      if (envelope.type === 'assistant.run.pending' || envelope.type === 'assistant.run.completed') {
        const candidate = envelope.payload as Partial<AssistantRun> | undefined;
        if (!candidate || typeof candidate.id !== 'string' || typeof candidate.state !== 'string') {
          fail('The assistant returned an invalid result.');
          return;
        }
        result = candidate as AssistantRun;
        closeSession('completed');
        return;
      }
      if (envelope.type === 'assistant.run.failed' || envelope.type === 'assistant.run.cancelled') {
        const message = typeof envelope.payload?.message === 'string'
          ? envelope.payload.message
          : 'The assistant could not complete this request.';
        fail(message);
        return;
      }
      if (envelope.type === 'session.closed') {
        if (result) settle();
        else fail('Request stopped. No action was taken.');
      }
    };

    const onError = () => fail('The assistant connection could not be opened. Try again.');
    const onClose = () => {
      if (!settled) {
        fail(result
          ? 'The assistant connection closed before the result was confirmed.'
          : 'The assistant connection ended unexpectedly. Try again.');
      }
    };
    const onAbort = () => {
      if (ready && socket.readyState === WebSocket.OPEN) {
        closeSession('cancelled');
      } else {
        fail('Request stopped. No action was taken.');
      }
    };

    timeout = window.setTimeout(() => {
      fail('The assistant took too long to respond. Check the conversation before retrying.');
    }, CONNECTION_TIMEOUT_MS);
    socket.addEventListener('open', onOpen, { once: true });
    socket.addEventListener('message', onMessage);
    socket.addEventListener('error', onError, { once: true });
    socket.addEventListener('close', onClose, { once: true });
    signal?.addEventListener('abort', onAbort, { once: true });
  });
}