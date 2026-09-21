import { createHash } from 'node:crypto';
import type { Request, Response } from 'express';

const backendURL =
  process.env.ASKOLO_GOOGLE_BACKEND_URL?.trim() ||
  (process.env.NODE_ENV === 'development' ? 'http://127.0.0.1:8090' : undefined);
const internalToken =
  process.env.ASKOLO_INTERNAL_TOKEN?.trim() ||
  (process.env.NODE_ENV !== 'production' && process.env.SESSION_SECRET
    ? createHash('sha256').update(`askolo-internal-auth:${process.env.SESSION_SECRET}`).digest('hex')
    : undefined);
const authorizationTimeoutMs = 2_000;

type AuthorizationPayload = {
  allowed?: boolean;
  workspaceId?: string;
  code?: string;
};

function actionForRequest(req: Request): string {
  const path = req.originalUrl.split('?')[0] ?? req.path;
  if (path.startsWith('/api/ai/')) return 'ai.execute';
  if (path.startsWith('/api/google/') || path.startsWith('/api/integrations/')) {
    return ['GET', 'HEAD'].includes(req.method) ? 'provider.read' : 'provider.write';
  }
  switch (req.method) {
    case 'GET':
    case 'HEAD':
      return 'resource.read';
    case 'POST':
      return 'resource.create';
    case 'PATCH':
    case 'PUT':
      return 'resource.update';
    case 'DELETE':
      return 'resource.delete';
    default:
      return 'resource.read';
  }
}

function requestWorkspaceID(req: Request): string | undefined {
  const value = req.headers['x-askolo-workspace-id'];
  return typeof value === 'string' && value.trim() ? value.trim() : undefined;
}

export async function authorizeRequest(req: Request, res: Response): Promise<boolean> {
  if (!backendURL || !internalToken) {
    res.status(503).json({ code: 'AUTHORIZATION_UNAVAILABLE', error: 'Authorization is temporarily unavailable.' });
    return false;
  }

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), authorizationTimeoutMs);
  try {
    const response = await fetch(new URL('/internal/authz/decision', backendURL), {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Askolo-Internal-Token': internalToken,
        ...(req.headers.cookie ? { cookie: req.headers.cookie } : {}),
        ...(typeof req.headers['x-request-id'] === 'string'
          ? { 'X-Request-ID': req.headers['x-request-id'] }
          : {}),
      },
      body: JSON.stringify({
        workspaceId: requestWorkspaceID(req),
        action: actionForRequest(req),
      }),
      signal: controller.signal,
    });
    const payload = (await response.json().catch(() => null)) as AuthorizationPayload | null;
    if (response.ok && payload?.allowed === true) {
      return true;
    }
    if (response.status === 401) {
      res.status(401).json({ error: 'Unauthorized' });
      return false;
    }
    if (response.status === 403) {
      res.status(403).json({ code: payload?.code ?? 'AUTHORIZATION_DENIED', error: 'Not authorized.' });
      return false;
    }
    res.status(503).json({ code: 'AUTHORIZATION_UNAVAILABLE', error: 'Authorization is temporarily unavailable.' });
    return false;
  } catch {
    res.status(503).json({ code: 'AUTHORIZATION_UNAVAILABLE', error: 'Authorization is temporarily unavailable.' });
    return false;
  } finally {
    clearTimeout(timer);
  }
}