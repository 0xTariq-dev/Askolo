import { eq } from 'drizzle-orm';
import type { NextFunction, Request, Response } from 'express';
import { db, usersTable } from '@workspace/db';
import { clearCookie, getNativeSession, getNativeSessionId } from '../lib/nativeAuth';
import { authorizeRequest } from '../lib/goAuthorization';

export type DbUser = typeof usersTable.$inferSelect;

declare global {
  namespace Express {
    interface Request {
      dbUser: DbUser;
  authProvider?: 'google' | 'github' | 'password';
      nativeSessionId?: string;
    }
  }
}

export async function requireAuth(
  req: Request,
  res: Response,
  next: NextFunction,
): Promise<void> {
  const nativeSessionId = getNativeSessionId(req);
  const nativeSession = await getNativeSession(nativeSessionId);
  if (!nativeSession) {
    if (nativeSessionId) clearCookie(res, 'sid');
    res.status(401).json({ error: 'Unauthorized' });
    return;
  }
  if (nativeSession.mfaRequired === true && nativeSession.mfaVerified !== true) {
    res.status(403).json({ code: 'MFA_REQUIRED', error: 'Complete MFA before accessing the app.' });
    return;
  }
  if (!(await authorizeRequest(req, res))) {
    return;
  }

  const [dbUser] = await db
    .select()
    .from(usersTable)
    .where(eq(usersTable.id, nativeSession.userId))
    .limit(1);
  if (!dbUser) {
    res.status(401).json({ error: 'Unauthorized' });
    return;
  }

  req.dbUser = dbUser;
  req.authProvider = nativeSession.provider;
  req.nativeSessionId = nativeSessionId;
  next();
}
