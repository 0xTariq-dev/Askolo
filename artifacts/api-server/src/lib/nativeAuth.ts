import { createHash, randomBytes } from "node:crypto";
import type { Request, Response } from "express";
import { eq, sql } from "drizzle-orm";
import { db, sessionsTable, usersTable } from "@workspace/db";

export const NATIVE_SESSION_COOKIE = "sid";
export const NATIVE_SESSION_TTL_MS = 7 * 24 * 60 * 60 * 1000;
export const GOOGLE_OAUTH_COOKIE = "askolo_google_oauth";
export const GOOGLE_OAUTH_TTL_MS = 10 * 60 * 1000;
export const GOOGLE_CALLBACK_PATH = "/api/auth/google/callback";

const GOOGLE_AUTH_URL = "https://accounts.google.com/o/oauth2/v2/auth";
const GOOGLE_TOKEN_URL = "https://oauth2.googleapis.com/token";
const GOOGLE_USERINFO_URL = "https://openidconnect.googleapis.com/v1/userinfo";
const AUTH_RATE_LIMIT_WINDOW_MS = 10 * 60 * 1000;
const AUTH_RATE_LIMIT_MAX_ATTEMPTS = 20;
const authAttempts = new Map<string, { count: number; resetAt: number }>();

type GoogleOAuthState = {
  state: string;
  codeVerifier: string;
  returnTo: string;
};

export type NativeSession = {
  userId: string;
  provider: "google" | "github" | "password";
  createdAt: string;
  mfaRequired?: boolean;
  mfaVerified?: boolean;
};

export type GoogleUserInfo = {
  sub: string;
  email: string;
  email_verified?: boolean;
  given_name?: string;
  family_name?: string;
  picture?: string;
};

export function consumeAuthRateLimit(req: Request): { allowed: boolean; retryAfterSeconds: number } {
  const forwardedFor = getHeaderValue(req.headers["x-forwarded-for"]);
  const key = forwardedFor?.split(",")[0]?.trim() || req.socket.remoteAddress || "unknown";
  const now = Date.now();
  const current = authAttempts.get(key);
  if (!current || current.resetAt <= now) {
    authAttempts.set(key, { count: 1, resetAt: now + AUTH_RATE_LIMIT_WINDOW_MS });
    return { allowed: true, retryAfterSeconds: 0 };
  }
  if (current.count >= AUTH_RATE_LIMIT_MAX_ATTEMPTS) {
    return {
      allowed: false,
      retryAfterSeconds: Math.max(1, Math.ceil((current.resetAt - now) / 1000)),
    };
  }
  current.count += 1;
  return { allowed: true, retryAfterSeconds: 0 };
}

function requiredGoogleConfig() {
  const clientId = process.env.GOOGLE_LOGIN_CLIENT_ID;
  const clientSecret = process.env.GOOGLE_LOGIN_CLIENT_SECRET;
  if (!clientId || !clientSecret) {
    throw new Error("Google account authentication is not configured.");
  }
  return { clientId, clientSecret };
}

function getHeaderValue(value: string | string[] | undefined): string | undefined {
  return Array.isArray(value) ? value[0] : value;
}

export function getRequestOrigin(req: Request): string {
  const protocol = getHeaderValue(req.headers["x-forwarded-proto"]) || "https";
  const host = getHeaderValue(req.headers["x-forwarded-host"]) || req.headers.host;
  if (!host) throw new Error("Unable to determine request host.");
  return `${protocol}://${host}`;
}

export function getGoogleCallbackUrl(req: Request): string {
  return `${getRequestOrigin(req)}${GOOGLE_CALLBACK_PATH}`;
}

export function getSafeReturnTo(value: unknown): string {
  if (typeof value !== "string" || !value.startsWith("/") || value.startsWith("//")) {
    return "/dashboard";
  }
  return value;
}

function base64Url(value: Buffer): string {
  return value.toString("base64url");
}

function sessionStorageKey(sessionId: string): string {
  return createHash("sha256").update(sessionId).digest("hex");
}

export function createPkcePair() {
  const codeVerifier = base64Url(randomBytes(32));
  const codeChallenge = base64Url(createHash("sha256").update(codeVerifier).digest());
  return { codeVerifier, codeChallenge };
}

export function createOAuthState(returnTo: string): GoogleOAuthState & { cookieValue: string } {
  const state = base64Url(randomBytes(32));
  const { codeVerifier, codeChallenge: _codeChallenge } = createPkcePair();
  const value: GoogleOAuthState = { state, codeVerifier, returnTo };
  return {
    ...value,
    cookieValue: base64Url(Buffer.from(JSON.stringify(value), "utf8")),
  };
}

export function decodeOAuthState(value: string | undefined): GoogleOAuthState | null {
  if (!value) return null;
  try {
    const parsed = JSON.parse(Buffer.from(value, "base64url").toString("utf8")) as Partial<GoogleOAuthState>;
    if (
      typeof parsed.state !== "string" ||
      typeof parsed.codeVerifier !== "string" ||
      typeof parsed.returnTo !== "string"
    ) {
      return null;
    }
    return parsed as GoogleOAuthState;
  } catch {
    return null;
  }
}

export function getCookie(req: Request, name: string): string | undefined {
  const header = req.headers.cookie;
  if (!header) return undefined;
  for (const part of header.split(";")) {
    const separator = part.indexOf("=");
    if (separator < 0) continue;
    const key = part.slice(0, separator).trim();
    if (key === name) return decodeURIComponent(part.slice(separator + 1).trim());
  }
  return undefined;
}

function cookieAttributes(maxAge: number) {
  return `Path=/; HttpOnly; Secure; SameSite=Lax; Max-Age=${Math.floor(maxAge / 1000)}`;
}

export function setOAuthStateCookie(res: Response, value: string) {
  res.setHeader(
    "Set-Cookie",
    `${GOOGLE_OAUTH_COOKIE}=${encodeURIComponent(value)}; ${cookieAttributes(GOOGLE_OAUTH_TTL_MS)}`,
  );
}

export function clearCookie(res: Response, name: string) {
  res.append("Set-Cookie", `${name}=; ${cookieAttributes(0)}`);
}

export function setNativeSessionCookie(res: Response, sid: string) {
  res.append(
    "Set-Cookie",
    `${NATIVE_SESSION_COOKIE}=${encodeURIComponent(sid)}; ${cookieAttributes(NATIVE_SESSION_TTL_MS)}`,
  );
}

export function getNativeSessionId(req: Request): string | undefined {
  const bearer = req.headers.authorization;
  if (bearer?.startsWith("Bearer ")) return bearer.slice("Bearer ".length).trim();
  return getCookie(req, NATIVE_SESSION_COOKIE);
}

export async function createNativeSession(userId: string): Promise<string> {
  const sid = base64Url(randomBytes(32));
  const session: NativeSession = {
    userId,
    provider: "google",
    createdAt: new Date().toISOString(),
  };
  await db.insert(sessionsTable).values({
    sid: sessionStorageKey(sid),
    sess: session,
    expire: new Date(Date.now() + NATIVE_SESSION_TTL_MS),
  });
  return sid;
}

export async function getNativeSession(sid: string | undefined): Promise<NativeSession | null> {
  if (!sid) return null;
  const [row] = await db
    .select()
    .from(sessionsTable)
    .where(eq(sessionsTable.sid, sessionStorageKey(sid)))
    .limit(1);
  if (!row || row.expire <= new Date()) {
    if (row) {
      await db
        .delete(sessionsTable)
        .where(eq(sessionsTable.sid, sessionStorageKey(sid)));
    }
    return null;
  }
  const session = row.sess as Partial<NativeSession>;
  if (
    !["google", "github", "password"].includes(String(session.provider)) ||
    typeof session.userId !== "string"
  ) {
    return null;
  }
  return session as NativeSession;
}

export async function deleteNativeSession(sid: string | undefined) {
  if (sid) {
    await db
      .delete(sessionsTable)
      .where(eq(sessionsTable.sid, sessionStorageKey(sid)));
  }
}

export async function findOrCreateGoogleUser(info: GoogleUserInfo) {
  const email = info.email.trim().toLowerCase();
  if (email.length > 320 || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
    throw new Error("Google did not provide a valid email identity.");
  }
  const [existing] = await db
    .select()
    .from(usersTable)
    .where(sql`lower(${usersTable.email}) = ${email}`)
    .limit(1);

  if (existing) return { user: existing, isNew: false };

  const [created] = await db
    .insert(usersTable)
    .values({
      email,
      firstName: info.given_name?.trim().slice(0, 100) || null,
      lastName: info.family_name?.trim().slice(0, 100) || null,
      profileImageUrl: isSafeImageUrl(info.picture) ? info.picture : null,
    })
    .returning();

  return { user: created, isNew: true };
}

function isSafeImageUrl(value: string | undefined): value is string {
  if (!value || value.length > 2048) return false;
  try {
    const url = new URL(value);
    return url.protocol === "https:";
  } catch {
    return false;
  }
}

async function fetchWithTimeout(url: string, init: RequestInit, timeoutMs = 10_000) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(url, { ...init, signal: controller.signal });
  } finally {
    clearTimeout(timer);
  }
}

export async function buildGoogleAuthorizationUrl(req: Request, oauth: GoogleOAuthState) {
  const { clientId } = requiredGoogleConfig();
  const { codeVerifier: _codeVerifier } = oauth;
  const challenge = base64Url(createHash("sha256").update(oauth.codeVerifier).digest());
  const params = new URLSearchParams({
    client_id: clientId,
    redirect_uri: getGoogleCallbackUrl(req),
    response_type: "code",
    scope: "openid email profile",
    access_type: "online",
    prompt: "select_account",
    state: oauth.state,
    code_challenge: challenge,
    code_challenge_method: "S256",
  });
  return `${GOOGLE_AUTH_URL}?${params.toString()}`;
}

export async function exchangeGoogleCode(req: Request, code: string, oauth: GoogleOAuthState) {
  const { clientId, clientSecret } = requiredGoogleConfig();
  const body = new URLSearchParams({
    code,
    client_id: clientId,
    client_secret: clientSecret,
    redirect_uri: getGoogleCallbackUrl(req),
    grant_type: "authorization_code",
    code_verifier: oauth.codeVerifier,
  });
  const response = await fetchWithTimeout(GOOGLE_TOKEN_URL, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body,
  });
  if (!response.ok) throw new Error(`Google token exchange returned ${response.status}.`);
  const payload = (await response.json()) as { access_token?: string };
  if (!payload.access_token) throw new Error("Google did not return an access token.");
  return payload.access_token;
}

export async function fetchGoogleUserInfo(accessToken: string): Promise<GoogleUserInfo> {
  const response = await fetchWithTimeout(GOOGLE_USERINFO_URL, {
    headers: { Authorization: `Bearer ${accessToken}` },
  });
  if (!response.ok) throw new Error(`Google userinfo returned ${response.status}.`);
  const info = (await response.json()) as Partial<GoogleUserInfo>;
  if (
    typeof info.sub !== "string" ||
    typeof info.email !== "string" ||
    info.email_verified !== true
  ) {
    throw new Error("Google did not provide a verified email identity.");
  }
  return info as GoogleUserInfo;
}