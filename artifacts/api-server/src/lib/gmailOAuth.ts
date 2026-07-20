import { eq } from "drizzle-orm";
import { createHmac, randomUUID } from "crypto";
import { db, gmailTokensTable } from "@workspace/db";

const GOOGLE_CLIENT_ID = process.env.GOOGLE_CLIENT_ID;
const GOOGLE_CLIENT_SECRET = process.env.GOOGLE_CLIENT_SECRET;
const GOOGLE_REDIRECT_URI = process.env.GOOGLE_REDIRECT_URI;
const SESSION_SECRET = process.env.SESSION_SECRET ?? "";

const TOKEN_URL = "https://oauth2.googleapis.com/token";
const AUTH_URL = "https://accounts.google.com/o/oauth2/v2/auth";
const GMAIL_API_BASE = "https://gmail.googleapis.com";

export const GMAIL_SCOPES = [
  "openid",
  "email",
  "profile",
  "https://www.googleapis.com/auth/gmail.modify",
];

interface TokenResponse {
  access_token: string;
  refresh_token?: string;
  expires_in: number;
  scope: string;
  token_type: string;
}

function requireConfig() {
  if (!GOOGLE_CLIENT_ID || !GOOGLE_CLIENT_SECRET || !GOOGLE_REDIRECT_URI) {
    throw new Error(
      "Google OAuth is not configured. Set GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, and GOOGLE_REDIRECT_URI.",
    );
  }
  return { GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, GOOGLE_REDIRECT_URI };
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

export function buildGmailAuthUrl(state: string) {
  const { GOOGLE_CLIENT_ID, GOOGLE_REDIRECT_URI } = requireConfig();
  const params = new URLSearchParams({
    client_id: GOOGLE_CLIENT_ID,
    redirect_uri: GOOGLE_REDIRECT_URI,
    response_type: "code",
    scope: GMAIL_SCOPES.join(" "),
    access_type: "offline",
    prompt: "consent",
    include_granted_scopes: "true",
    state,
  });
  return `${AUTH_URL}?${params.toString()}`;
}

export async function exchangeCodeForTokens(code: string): Promise<TokenResponse> {
  const { GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, GOOGLE_REDIRECT_URI } = requireConfig();
  const body = new URLSearchParams({
    grant_type: "authorization_code",
    code,
    client_id: GOOGLE_CLIENT_ID,
    client_secret: GOOGLE_CLIENT_SECRET,
    redirect_uri: GOOGLE_REDIRECT_URI,
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

export async function storeGmailTokens(userId: string, tokens: TokenResponse) {
  const expiresAt = new Date(Date.now() + tokens.expires_in * 1000);
  await db
    .insert(gmailTokensTable)
    .values({
      userId,
      accessToken: tokens.access_token,
      refreshToken: tokens.refresh_token ?? "",
      expiresAt,
      scope: tokens.scope,
    })
    .onConflictDoUpdate({
      target: gmailTokensTable.userId,
      set: {
        accessToken: tokens.access_token,
        refreshToken: tokens.refresh_token ?? "",
        expiresAt,
        scope: tokens.scope,
        updatedAt: new Date(),
      },
    });
}

export async function getGmailAccessToken(userId: string): Promise<string> {
  const [row] = await db
    .select()
    .from(gmailTokensTable)
    .where(eq(gmailTokensTable.userId, userId))
    .limit(1);

  if (!row) {
    throw new Error("Gmail is not connected. Please connect your Gmail account.");
  }

  // Refresh if the token expires in less than 60 seconds.
  if (new Date(row.expiresAt).getTime() - Date.now() < 60_000) {
    if (!row.refreshToken) {
      throw new Error("Gmail token expired and no refresh token is available.");
    }
    const tokens = await refreshAccessToken(row.refreshToken);
    await storeGmailTokens(userId, tokens);
    return tokens.access_token;
  }

  return row.accessToken;
}

export async function hasGmailConnection(userId: string): Promise<boolean> {
  try {
    await getGmailAccessToken(userId);
    return true;
  } catch {
    return false;
  }
}

export async function gmailApiRequest(
  userId: string,
  path: string,
  init: RequestInit = {},
) {
  const accessToken = await getGmailAccessToken(userId);
  const url = `${GMAIL_API_BASE}${path}`;
  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${accessToken}`);
  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(url, { ...init, headers });
  if (!res.ok) {
    throw new Error(`Gmail API request failed: ${res.status} ${res.statusText}`);
  }
  return res;
}

export function getOAuthCallbackCookie(state: string) {
  const signature = signState(state);
  const value = `${state}.${signature}`;
  return `google_oauth_state=${encodeURIComponent(value)}; Path=/api/google/gmail; HttpOnly; Secure; SameSite=Lax; Max-Age=600`;
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
