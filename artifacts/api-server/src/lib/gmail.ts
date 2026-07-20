import { gmailApiRequest } from "./gmailOAuth";

const GMAIL_API_PREFIX = "/gmail/v1/users/me";

interface GmailMessageHeader {
  name: string;
  value: string;
}

interface GmailMessagePayload {
  headers?: GmailMessageHeader[];
  parts?: Array<{
    mimeType: string;
    body?: { data?: string; size?: number };
    parts?: Array<{ mimeType: string; body?: { data?: string; size?: number } }>;
  }>;
  body?: { data?: string; size?: number };
}

interface GmailMessage {
  id: string;
  threadId: string;
  labelIds?: string[];
  payload?: GmailMessagePayload;
  snippet?: string;
  internalDate?: string;
}

export async function listGmailMessages(userId: string, maxResults = 20) {
  const res = await gmailApiRequest(
    userId,
    `${GMAIL_API_PREFIX}/messages?maxResults=${maxResults}&labelIds=INBOX`,
    { method: "GET" },
  );
  return (await res.json()) as { messages?: Array<{ id: string; threadId: string }> };
}

export async function verifyGmailConnection(userId: string): Promise<boolean> {
  try {
    const res = await gmailApiRequest(
      userId,
      `${GMAIL_API_PREFIX}/messages?maxResults=1&labelIds=INBOX`,
      { method: "GET" },
    );
    return res.ok;
  } catch {
    return false;
  }
}

export async function getGmailMessage(userId: string, messageId: string) {
  const res = await gmailApiRequest(
    userId,
    `${GMAIL_API_PREFIX}/messages/${messageId}?format=full`,
    { method: "GET" },
  );
  return (await res.json()) as GmailMessage;
}

export async function getGmailThread(userId: string, threadId: string) {
  const res = await gmailApiRequest(
    userId,
    `${GMAIL_API_PREFIX}/threads/${threadId}?format=full`,
    { method: "GET" },
  );
  return (await res.json()) as { messages: GmailMessage[] };
}

export async function sendGmailMessage(userId: string, payload: { raw: string; threadId?: string }) {
  const res = await gmailApiRequest(
    userId,
    `${GMAIL_API_PREFIX}/messages/send`,
    {
      method: "POST",
      body: JSON.stringify(payload),
      headers: { "Content-Type": "application/json" },
    },
  );
  return (await res.json()) as GmailMessage;
}

export function getHeader(message: GmailMessage, name: string) {
  return message.payload?.headers?.find((h) => h.name.toLowerCase() === name.toLowerCase())?.value || "";
}

export function getBodyText(message: GmailMessage) {
  const payload = message.payload;
  if (!payload) return "";

  const decode = (data?: string) => {
    if (!data) return "";
    try {
      return Buffer.from(data, "base64url").toString("utf-8");
    } catch {
      return "";
    }
  };

  if (payload.body?.data) return decode(payload.body.data);

  for (const part of payload.parts || []) {
    if (part.mimeType === "text/plain" && part.body?.data) return decode(part.body.data);
  }
  for (const part of payload.parts || []) {
    if (part.mimeType === "text/html" && part.body?.data) return decode(part.body.data);
  }
  return "";
}

export function getReplyThreadText(thread: { messages: GmailMessage[] }) {
  return thread.messages
    .map((m) => {
      const from = getHeader(m, "From");
      const body = getBodyText(m);
      return `From: ${from}\n\n${body}`;
    })
    .join("\n\n---\n\n");
}

export function base64UrlEncode(str: string) {
  return Buffer.from(str)
    .toString("base64url")
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
}

export function buildEmailRaw(to: string, subject: string, body: string, threadId?: string) {
  const encodedBody = Buffer.from(body, "utf-8").toString("base64");
  const mime = [
    `To: ${to}`,
    `Subject: ${subject}`,
    "Content-Type: text/plain; charset=utf-8",
    "Content-Transfer-Encoding: base64",
    "",
    encodedBody,
  ].join("\r\n");
  const raw = base64UrlEncode(mime);
  return threadId ? { raw, threadId } : { raw };
}
