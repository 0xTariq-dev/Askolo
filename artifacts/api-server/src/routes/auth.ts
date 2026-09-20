import { Router, type IRouter } from "express";
import { eq } from "drizzle-orm";
import { db, usersTable } from "@workspace/db";
import { clearCookie, deleteNativeSession, getNativeSession, getNativeSessionId } from "../lib/nativeAuth";

const router: IRouter = Router();

function userResponse(user: typeof usersTable.$inferSelect) {
  return {
    id: user.id,
    email: user.email,
    firstName: user.firstName,
    lastName: user.lastName,
    profileImageUrl: user.profileImageUrl,
  };
}

// GET /auth/user
router.get("/auth/user", async (req, res): Promise<void> => {
  const session = await getNativeSession(getNativeSessionId(req));
  if (!session) {
    res.json({ user: null });
    return;
  }
  const [user] = await db.select().from(usersTable).where(eq(usersTable.id, session.userId)).limit(1);
  res.json({ user: user ? userResponse(user) : null });
});

// POST /auth/logout
router.post("/auth/logout", async (req, res): Promise<void> => {
  const sid = getNativeSessionId(req);
  await deleteNativeSession(sid);
  clearCookie(res, "sid");
  res.sendStatus(204);
});

export default router;