import { Router, type IRouter } from "express";
import { eq } from "drizzle-orm";
import { db, usersTable } from "@workspace/db";
import {
  buildGoogleAuthorizationUrl,
  clearCookie,
  consumeAuthRateLimit,
  createOAuthState,
  createNativeSession,
  decodeOAuthState,
  deleteNativeSession,
  exchangeGoogleCode,
  fetchGoogleUserInfo,
  findOrCreateGoogleUser,
  getCookie,
  getNativeSession,
  getNativeSessionId,
  getSafeReturnTo,
  setNativeSessionCookie,
  setOAuthStateCookie,
  GOOGLE_OAUTH_COOKIE,
} from "../lib/nativeAuth";
import { logger } from "../lib/logger";

const router: IRouter = Router();

function redirectWithStatus(returnTo: string, status: "success" | "error", isNew?: boolean) {
  const url = new URL(returnTo, "https://askolo.app");
  url.searchParams.set("auth", status);
  if (isNew) url.searchParams.set("new", "1");
  return `${url.pathname}${url.search}${url.hash}`;
}

function userResponse(user: typeof usersTable.$inferSelect) {
  return {
    id: user.id,
    email: user.email,
    firstName: user.firstName,
    lastName: user.lastName,
    profileImageUrl: user.profileImageUrl,
  };
}

// GET /auth/google
router.get("/auth/google", async (req, res): Promise<void> => {
  const rateLimit = consumeAuthRateLimit(req);
  if (!rateLimit.allowed) {
    res.setHeader("Retry-After", String(rateLimit.retryAfterSeconds));
    res.status(429).json({ error: "Too many sign-in attempts. Please try again shortly." });
    return;
  }
  const returnTo = getSafeReturnTo(req.query.returnTo);
  try {
    const oauth = createOAuthState(returnTo);
    setOAuthStateCookie(res, oauth.cookieValue);
    res.redirect(await buildGoogleAuthorizationUrl(req, oauth));
  } catch (error) {
    req.log.error({ err: error, route: "auth.google.start" }, "Google sign-in could not start");
    res.redirect(`${returnTo}?auth=error`);
  }
});

// GET /auth/google/callback
router.get("/auth/google/callback", async (req, res): Promise<void> => {
  const rateLimit = consumeAuthRateLimit(req);
  if (!rateLimit.allowed) {
    res.setHeader("Retry-After", String(rateLimit.retryAfterSeconds));
    res.status(429).json({ error: "Too many sign-in attempts. Please try again shortly." });
    return;
  }
  const oauth = decodeOAuthState(getCookie(req, GOOGLE_OAUTH_COOKIE));
  const error = typeof req.query.error === "string" ? req.query.error : undefined;
  const code = typeof req.query.code === "string" ? req.query.code : undefined;
  const state = typeof req.query.state === "string" ? req.query.state : undefined;
  const returnTo = oauth?.returnTo ?? "/dashboard";
  clearCookie(res, GOOGLE_OAUTH_COOKIE);

  if (error || !code || !oauth || !state || state !== oauth.state) {
    req.log.warn({ route: "auth.google.callback", reason: error ? "provider_error" : "invalid_oauth_state" }, "Google sign-in callback rejected");
    res.redirect(redirectWithStatus(returnTo, "error"));
    return;
  }

  try {
    const accessToken = await exchangeGoogleCode(req, code, oauth);
    const info = await fetchGoogleUserInfo(accessToken);
    const { user, isNew } = await findOrCreateGoogleUser(info);
    const sid = await createNativeSession(user.id);
    setNativeSessionCookie(res, sid);
    req.log.info({ userId: user.id, isNew, provider: "google" }, "Google sign-in completed");
    res.redirect(redirectWithStatus(isNew ? "/profile" : returnTo, "success", isNew));
  } catch (error) {
    req.log.error({ err: error, route: "auth.google.callback" }, "Google sign-in failed");
    res.redirect(redirectWithStatus(returnTo, "error"));
  }
});

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