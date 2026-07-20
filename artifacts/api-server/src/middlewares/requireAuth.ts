import { getAuth } from '@clerk/express';
import { eq } from 'drizzle-orm';
import type { NextFunction, Request, Response } from 'express';
import { db, usersTable } from '@workspace/db';

export type DbUser = typeof usersTable.$inferSelect;

declare global {
  namespace Express {
    interface Request {
      dbUser: DbUser;
    }
  }
}

export async function requireAuth(
  req: Request,
  res: Response,
  next: NextFunction,
): Promise<void> {
  const auth = getAuth(req);
  const sessionClaims = auth?.sessionClaims as Record<string, unknown> | undefined;

  // sessionClaims.userId holds the Replit Auth subject ID for migrated users,
  // or the Clerk native ID for new users. auth.userId is always the Clerk native ID.
  const userId = (sessionClaims?.userId as string | undefined) ?? auth?.userId;

  if (!userId) {
    res.status(401).json({ error: 'Unauthorized' });
    return;
  }

  let [dbUser] = await db
    .select()
    .from(usersTable)
    .where(eq(usersTable.id, userId))
    .limit(1);

  if (!dbUser) {
    // JIT provision: insert a minimal row the first time this user is seen.
    const [inserted] = await db
      .insert(usersTable)
      .values({ id: userId })
      .onConflictDoNothing()
      .returning();

    if (inserted) {
      dbUser = inserted;
    } else {
      // Race: another request won the insert — re-fetch.
      [dbUser] = await db
        .select()
        .from(usersTable)
        .where(eq(usersTable.id, userId))
        .limit(1);
    }
  }

  if (!dbUser) {
    res.status(401).json({ error: 'Unauthorized' });
    return;
  }

  req.dbUser = dbUser;
  next();
}
