import { eq } from "drizzle-orm";
import { createHmac, randomUUID } from "crypto";
import { db, gmailTokensTable } from "@workspace/db";
import type { Request } from "express";

const GOOGLE_CLIENT_ID = process.env.GOOGLE_CLIENT_ID;
const GOOGLE_CLIENT_SECRET = process.env.GOOGLE_CLIENT_SECRET;
const SESSION_SECRET = process.env.SESSION_SECRET ?? "";

const TOKEN_URL = "https://oauth2.googleapis.com/token";
const AUTH_URL = "https://accounts.google.com/o/oauth2/v2/auth";

const GMAIL_API_BASE = "https://gmail.googleapis.com";
const CALENDAR_API_BASE = "https://www.googleapis.com/calendar/v3";

const CALENDAR_SCOPE = "https://www.googleapis.com/auth/calendar.events";
const GMAIL_SCOPE = "https://www.googleapis.com/auth/gmail.modify";

export const GOOGLE_SCOPES = ["openid", "email", "profile", CALENDAR_SCOPE, GMAIL_SCOPE];

export const GOOGLE_REDIRECT_PATH = "/api/google/gmail/callback";

interface TokenResponse {
  access_token: string;
  refresh_token?: string;
  expires_in: number;
  scope: string;
  token_type: string;
}

function requireConfig() {
  if (!GOOGLE_CLIENT_ID || !GOOGLE_CLIENT_SECRET) {
    throw new Error("Google OAuth is not configured. Set GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET.");
  }
  return { GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET };
}

export function getRedirectUri(req: Request) {
  const host = req.get("x-forwarded-host") || req.headers.host || "";
  return `https://${host}${GOOGLE_REDIRECT_PATH}`;
}

export function signState(state: string) {
  const hmac = createHmac("sha256", SESSION_SECRET);
  hmac.update(state);
  return hmac.digest("hex");
}

export function verifyState(state: string, signature: string) {
  try {
    return signState(state) === signature;
  } catch {
    return false;
  }
}

export function generateOAuthState() {
  return randomUUID();
}

export function buildGoogleAuthUrl(redirectUri: string, state: string) {
  const { GOOGLE_CLIENT_ID } = requireConfig();
  const params = new URLSearchParams({
    client_id: GOOGLE_CLIENT_ID,
    redirect_uri: redirectUri,
    response_type: "code",
    scope: GOOGLE_SCOPES.join(" "),
    access_type: "offline",
    prompt: "consent",
    include_granted_scopes: "true",
    state,
  });
  return `${AUTH_URL}?${params.toString()}`;
}

export async function exchangeCodeForTokens(code: string, redirectUri: string): Promise<TokenResponse> {
  const { GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET } = requireConfig();
  const body = new URLSearchParams({
    grant_type: "authorization_code",
    code,
    client_id: GOOGLE_CLIENT_ID,
    client_secret: GOOGLE_CLIENT_SECRET,
    redirect_uri: redirectUri,
  });

  const res = await fetch(TOKEN_URL, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: body.toString(),
  });

  if (!res.ok) {
    const text = await res.text();
    throw new Error(`Google token exchange failed: ${res.status} ${text}`);
  }

  return (await res.json()) as TokenResponse;
}

export async function refreshAccessToken(refreshToken: string): Promise<TokenResponse> {
  const { GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET } = requireConfig();
  const body = new URLSearchParams({
    grant_type: "refresh_token",
    refresh_token: refreshToken,
    client_id: GOOGLE_CLIENT_ID,
    client_secret: GOOGLE_CLIENT_SECRET,
  });

  const res = await fetch(TOKEN_URL, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: body.toString(),
  });

  if (!res.ok) {
    const text = await res.text();
    throw new Error(`Google token refresh failed: ${res.status} ${text}`);
  }

  return (await res.json()) as TokenResponse;
}

export async function storeGoogleTokens(userId: string, tokens: TokenResponse) {
  const expiresAt = new Date(Date.now() + tokens.expires_in * 1000);

  // Preserve the existing refresh token if Google does not return a new one
  // (e.g., re-authorization without a new offline token).
  const existing = await db
    .select({ refreshToken: gmailTokensTable.refreshToken })
    .from(gmailTokensTable)
    .where(eq(gmailTokensTable.userId, userId))
    .limit(1)
    .then((rows) => rows[0]);

  const refreshToken = tokens.refresh_token || existing?.refreshToken || "";

  await db
    .insert(gmailTokensTable)
    .values({
      userId,
      accessToken: tokens.access_token,
      refreshToken,
      expiresAt,
      scope: tokens.scope,
    })
    .onConflictDoUpdate({
      target: gmailTokensTable.userId,
      set: {
        accessToken: tokens.access_token,
        refreshToken,
        expiresAt,
        scope: tokens.scope,
        updatedAt: new Date(),
      },
    });
}

export async function getGoogleToken(userId: string) {
  const [row] = await db
    .select()
    .from(gmailTokensTable)
    .where(eq(gmailTokensTable.userId, userId))
    .limit(1);

  if (!row) {
    throw new Error("Google account is not connected. Please connect your Google account.");
  }

  // Refresh if the token expires in less than 60 seconds.
  if (new Date(row.expiresAt).getTime() - Date.now() < 60_000) {
    if (!row.refreshToken) {
      throw new Error("Google token expired and no refresh token is available.");
    }
    const tokens = await refreshAccessToken(row.refreshToken);
    await storeGoogleTokens(userId, tokens);
    return { ...row, ...tokens, accessToken: tokens.access_token, scope: tokens.scope };
  }

  return row;
}

export async function getGoogleAccessToken(userId: string): Promise<string> {
  const token = await getGoogleToken(userId);
  return token.accessToken;
}

export function tokenHasScope(token: { scope: string }, scope: string) {
  return token.scope.split(" ").includes(scope);
}

export async function hasGoogleScope(userId: string, scope: string): Promise<boolean> {
  try {
    const token = await getGoogleToken(userId);
    return tokenHasScope(token, scope);
  } catch {
    return false;
  }
}

export async function verifyGmailConnection(userId: string): Promise<boolean> {
  if (!(await hasGoogleScope(userId, GMAIL_SCOPE))) return false;
  try {
    const accessToken = await getGoogleAccessToken(userId);
    const res = await fetch(`${GMAIL_API_BASE}/gmail/v1/users/me/messages?maxResults=1&labelIds=INBOX`, {
      headers: { Authorization: `Bearer ${accessToken}` },
    });
    return res.ok;
  } catch {
    return false;
  }
}

export async function verifyCalendarConnection(userId: string): Promise<boolean> {
  if (!(await hasGoogleScope(userId, CALENDAR_SCOPE))) return false;
  try {
    const accessToken = await getGoogleAccessToken(userId);
    const res = await fetch(`${CALENDAR_API_BASE}/users/me/calendarList`, {
      headers: { Authorization: `Bearer ${accessToken}` },
    });
    return res.ok;
  } catch {
    return false;
  }
}

export async function googleApiRequest(
  userId: string,
  baseUrl: string,
  path: string,
  init: RequestInit = {},
) {
  const accessToken = await getGoogleAccessToken(userId);
  const url = `${baseUrl}${path}`;
  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${accessToken}`);
  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(url, { ...init, headers });
  if (!res.ok) {
    throw new Error(`Google API request failed: ${res.status} ${res.statusText}`);
  }
  return res;
}

export async function gmailApiRequest(userId: string, path: string, init: RequestInit = {}) {
  return googleApiRequest(userId, GMAIL_API_BASE, path, init);
}

export async function calendarApiRequest(userId: string, path: string, init: RequestInit = {}) {
  return googleApiRequest(userId, CALENDAR_API_BASE, path, init);
}

export function getOAuthCallbackCookie(state: string, redirectTo?: string) {
  const signature = signState(state);
  const value = `${state}.${signature}`;
  const cookies = [
    `google_oauth_state=${encodeURIComponent(value)}; Path=/api/google/gmail; HttpOnly; Secure; SameSite=None; Max-Age=600`,
  ];
  if (redirectTo) {
    const redirectSignature = signState(redirectTo);
    const redirectValue = `${redirectTo}.${redirectSignature}`;
    cookies.push(
      `google_oauth_redirect=${encodeURIComponent(redirectValue)}; Path=/api/google/gmail; HttpOnly; Secure; SameSite=None; Max-Age=600`,
    );
  }
  return cookies;
}

export function parseOAuthStateCookie(cookieHeader?: string) {
  if (!cookieHeader) return null;
  const match = cookieHeader.match(/(?:^|;\s*)google_oauth_state=([^;]+)/);
  if (!match) return null;
  const value = decodeURIComponent(match[1]);
  const [state, signature] = value.split(".");
  if (!state || !signature) return null;
  if (!verifyState(state, signature)) return null;
  return state;
}

export function parseOAuthRedirectCookie(cookieHeader?: string) {
  if (!cookieHeader) return "/dashboard";
  const match = cookieHeader.match(/(?:^|;\s*)google_oauth_redirect=([^;]+)/);
  if (!match) return "/dashboard";
  const value = decodeURIComponent(match[1]);
  const [redirectTo, signature] = value.split(".");
  if (!redirectTo || !signature) return "/dashboard";
  if (!verifyState(redirectTo, signature)) return "/dashboard";
  // Only allow relative paths to avoid open redirects.
  if (!redirectTo.startsWith("/")) return "/dashboard";
  return redirectTo;
}
