import type { AssistantRun } from '@workspace/api-client-react';
import {
  parsePublicWebSocketEnvelope,
  publicWebSocketURL,
  resumePublicWebSocketMetadata,
  sendPublicWebSocketMessage,
} from '@/lib/public-websocket';

export interface AssistantRunWebSocketInput {
  conversationId?: string;
  transcript: string;
  idempotencyKey: string;
  policyVersion: number;
  voiceChat?: boolean;
}

export interface AssistantRunWebSocketCallbacks {
  onReplyDelta?: (runId: string, delta: string) => void;
  onSpeechChunk?: (runId: string, mediaType: string, data: string) => void;
  onSpeechError?: (runId: string, message: string) => void;
}

const CONNECTION_TIMEOUT_MS = 45_000;
const SESSION_CLOSE_TIMEOUT_MS = 3_500;

export function createAssistantRunOverWebSocket(
  input: AssistantRunWebSocketInput,
  signal?: AbortSignal,
  callbacks: AssistantRunWebSocketCallbacks = {},
): Promise<AssistantRun> {
  if (signal?.aborted) {
    return Promise.reject(new Error('Request stopped. No action was taken.'));
  }

  const socket = new WebSocket(publicWebSocketURL());
  const clientSequence = { current: 0 };
  let serverSequence = 0;
  let ready = false;
  let closing = false;
  let recovering = false;
  let settled = false;
  let result: AssistantRun | null = null;
  let sessionId = '';
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

    const recoverThenFail = (fallbackMessage: string) => {
      if (settled || recovering) return;
      recovering = true;
      window.clearTimeout(timeout);
      if (!sessionId) {
        fail(fallbackMessage);
        return;
      }
      void resumePublicWebSocketMetadata(sessionId, serverSequence)
        .then((state) => {
          const requestMayHaveStarted = state.assistantRunIds.length > 0;
          fail(requestMayHaveStarted
            ? 'The assistant request may have continued after the connection ended. Check the conversation before retrying.'
            : fallbackMessage);
        })
        .catch(() => {
          fail('The assistant connection ended and its status could not be confirmed. Check the conversation before retrying.');
        });
    };

    const closeSession = (reason: 'completed' | 'cancelled') => {
      if (closing || socket.readyState !== WebSocket.OPEN) {
        if (result) settle();
        else if (reason === 'cancelled') fail('Request stopped. No action was taken.');
        return;
      }
      closing = true;
      try {
        sendPublicWebSocketMessage(socket, clientSequence, 'session.close', { reason });
      } catch {
        if (reason === 'completed' && result) settle();
        else fail(reason === 'cancelled'
          ? 'Request stopped. No action was taken.'
          : 'The assistant connection closed before the request was confirmed.');
        return;
      }
      closeTimeout = window.setTimeout(() => {
        if (reason === 'completed' && result) settle();
        else fail('The assistant connection closed before the request was confirmed.');
      }, SESSION_CLOSE_TIMEOUT_MS);
    };

    const onOpen = () => {
      sendPublicWebSocketMessage(socket, clientSequence, 'session.start');
    };

    const onMessage = (event: MessageEvent) => {
      if (recovering) return;
      if (typeof event.data !== 'string') {
        fail('The assistant connection returned an invalid message.');
        return;
      }
      const envelope = parsePublicWebSocketEnvelope<Record<string, unknown>>(event.data);
      if (!envelope) {
        fail('The assistant connection returned an invalid message.');
        return;
      }
      sessionId = envelope.sessionId;
      if (envelope.type === 'protocol.error') {
        const message = typeof envelope.payload?.message === 'string'
          ? envelope.payload.message
          : 'The assistant could not complete this request.';
        fail(message);
        return;
      }
      if (envelope.sequence > 0) {
        if (envelope.sequence <= serverSequence) return;
        if (envelope.sequence !== serverSequence + 1) {
          recoverThenFail('The assistant connection missed an event. Check the conversation before retrying.');
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
          voiceChat: input.voiceChat,
        });
        return;
      }
      if (envelope.type === 'assistant.run.reply.delta') {
        const runId = envelope.payload?.runId;
        const delta = envelope.payload?.delta;
        if (typeof runId !== 'string' || typeof delta !== 'string' || delta.length > 8_000) {
          fail('The assistant returned an invalid reply update.');
          return;
        }
        callbacks.onReplyDelta?.(runId, delta);
        return;
      }
      if (envelope.type === 'assistant.run.speech.chunk') {
        const runId = envelope.payload?.runId;
        const mediaType = envelope.payload?.mediaType;
        const data = envelope.payload?.data;
        if (
          typeof runId !== 'string' ||
          mediaType !== 'audio/mpeg' ||
          typeof data !== 'string' ||
          data.length > 60_000 ||
          !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(data)
        ) {
          fail('The assistant returned an invalid audio update.');
          return;
        }
        callbacks.onSpeechChunk?.(runId, mediaType, data);
        return;
      }
      if (envelope.type === 'assistant.run.speech.error') {
        const runId = envelope.payload?.runId;
        const message = envelope.payload?.message;
        if (typeof runId !== 'string') {
          fail('The assistant returned an invalid audio status.');
          return;
        }
        callbacks.onSpeechError?.(
          runId,
          typeof message === 'string' ? message : 'Speech output could not be generated. Your reply is saved.',
        );
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

    const onError = () => recoverThenFail('The assistant connection could not be opened. Check the conversation before retrying.');
    const onClose = () => {
      if (!settled) {
        if (result) {
          settle();
          return;
        }
        recoverThenFail(result
          ? 'The assistant connection closed before the result was confirmed. Check the conversation before retrying.'
          : 'The assistant connection ended unexpectedly. Check the conversation before retrying.');
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