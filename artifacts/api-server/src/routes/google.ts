import { Router, type IRouter } from "express";
import { eq, and } from "drizzle-orm";
import { db, googleConnectionsTable, eventsTable } from "@workspace/db";
import {
  listGoogleCalendarEvents,
  createGoogleCalendarEvent,
  updateGoogleCalendarEvent,
  deleteGoogleCalendarEvent,
  formatGoogleEventToLocal,
  getGoogleCalendarList,
  toGoogleCalendarEvent,
  type LocalEventInput,
} from "../lib/googleCalendar";
import {
  listGmailMessages,
  getGmailMessage,
  getGmailThread,
  sendGmailMessage,
  getHeader,
  getBodyText,
  getReplyThreadText,
  buildEmailRaw,
} from "../lib/gmail";
import {
  buildGoogleAuthUrl,
  exchangeCodeForTokens,
  storeGoogleTokens,
  generateOAuthState,
  getRedirectUri,
  getOAuthCallbackCookie,
  parseOAuthStateCookie,
  parseOAuthRedirectCookie,
} from "../lib/googleOAuth";
import { getGoogleConnectionStatus } from "../lib/googleStatus";
import { openai } from "@workspace/integrations-openai-ai-server";

const router: IRouter = Router();

const CALENDAR_SCOPE = "calendar";
const GMAIL_SCOPE = "gmail";

async function getOrCreateConnection(userId: string) {
  const [existing] = await db
    .select()
    .from(googleConnectionsTable)
    .where(eq(googleConnectionsTable.userId, userId));
  if (existing) return existing;
  const [created] = await db
    .insert(googleConnectionsTable)
    .values({ userId, connected: false, scopes: [] })
    .returning();
  return created;
}

// GET /google/status
router.get("/google/status", async (req, res): Promise<void> => {
  try {
    const status = await getGoogleConnectionStatus(req.dbUser.id);

    await db
      .insert(googleConnectionsTable)
      .values({ userId: req.dbUser.id, connected: status.connected, scopes: status.scopes })
      .onConflictDoUpdate({
        target: googleConnectionsTable.userId,
        set: { connected: status.connected, scopes: status.scopes, updatedAt: new Date() },
      });

    res.json(status);
  } catch (err) {
    req.log.error(err, "Google status check failed");
    res.json({ connected: false, scopes: [], calendarConnected: false, gmailConnected: false });
  }
});

// POST /google/calendar/sync
router.post("/google/calendar/sync", async (req, res): Promise<void> => {
  const { from, to } = req.body as { from?: string; to?: string };
  if (!from || !to) {
    res.status(400).json({ error: "from and to are required" });
    return;
  }

  try {
    const calendarList = await getGoogleCalendarList(req.dbUser.id);
    const primary = calendarList.items?.find((c) => c.primary) || calendarList.items?.[0];
    if (!primary) {
      res.status(404).json({ error: "No calendar found" });
      return;
    }

    const googleEvents = await listGoogleCalendarEvents(req.dbUser.id, primary.id, from, to);
    const mapped = (googleEvents.items || []).map(formatGoogleEventToLocal);

    for (const event of mapped) {
      if (!event.googleEventId) continue;
      const [existing] = await db
        .select({ id: eventsTable.id })
        .from(eventsTable)
        .where(and(eq(eventsTable.userId, req.dbUser.id), eq(eventsTable.googleEventId, event.googleEventId)));
      if (existing) {
        await db.update(eventsTable).set(event).where(eq(eventsTable.id, existing.id));
      } else {
        await db.insert(eventsTable).values({ ...event, userId: req.dbUser.id, color: "#3b82f6" });
      }
    }

    const existingConnection = await getOrCreateConnection(req.dbUser.id);
    const existingScopes = existingConnection.scopes || [];
    const updatedScopes = Array.from(new Set([...existingScopes, CALENDAR_SCOPE]));

    await db
      .update(googleConnectionsTable)
      .set({ connected: true, scopes: updatedScopes, updatedAt: new Date() })
      .where(eq(googleConnectionsTable.userId, req.dbUser.id));

    res.json({ synced: mapped.length, calendarId: primary.id });
  } catch (err) {
    req.log.error(err, "Google Calendar sync failed");
    res.status(500).json({ error: "Failed to sync calendar" });
  }
});

// POST /google/calendar/events
router.post("/google/calendar/events", async (req, res): Promise<void> => {
  const { calendarId, ...event } = req.body as LocalEventInput & { calendarId?: string };
  if (!calendarId) {
    res.status(400).json({ error: "calendarId is required" });
    return;
  }
  try {
    const created = await createGoogleCalendarEvent(req.dbUser.id, calendarId, toGoogleCalendarEvent(event));
    res.status(201).json(formatGoogleEventToLocal(created));
  } catch (err) {
    req.log.error(err, "Google Calendar create event failed");
    res.status(500).json({ error: "Failed to create calendar event" });
  }
});

// PATCH /google/calendar/events/:id
router.patch("/google/calendar/events/:id", async (req, res): Promise<void> => {
  const { calendarId, ...event } = req.body as LocalEventInput & { calendarId?: string };
  const eventId = req.params.id;
  if (!calendarId || !eventId) {
    res.status(400).json({ error: "calendarId and eventId are required" });
    return;
  }
  try {
    const updated = await updateGoogleCalendarEvent(req.dbUser.id, calendarId, eventId, toGoogleCalendarEvent(event));
    res.json(formatGoogleEventToLocal(updated));
  } catch (err) {
    req.log.error(err, "Google Calendar update event failed");
    res.status(500).json({ error: "Failed to update calendar event" });
  }
});

// DELETE /google/calendar/events/:id
router.delete("/google/calendar/events/:id", async (req, res): Promise<void> => {
  const { calendarId } = req.body as { calendarId?: string };
  const eventId = req.params.id;
  if (!calendarId || !eventId) {
    res.status(400).json({ error: "calendarId and eventId are required" });
    return;
  }
  try {
    await deleteGoogleCalendarEvent(req.dbUser.id, calendarId, eventId);
    res.sendStatus(204);
  } catch (err) {
    req.log.error(err, "Google Calendar delete event failed");
    res.status(500).json({ error: "Failed to delete calendar event" });
  }
});

// ─── GOOGLE OAUTH ─────────────────────────────────────────────────────────

// GET /google/gmail/connect
router.get("/google/gmail/connect", async (req, res): Promise<void> => {
  const redirectTo =
    typeof req.query.redirectTo === "string" && req.query.redirectTo.startsWith("/")
      ? req.query.redirectTo
      : "/dashboard";
  try {
    const state = generateOAuthState();
    const redirectUri = getRedirectUri(req);
    const url = buildGoogleAuthUrl(redirectUri, state);
    res.setHeader("Set-Cookie", getOAuthCallbackCookie(state, redirectTo));
    res.redirect(url);
  } catch (err) {
    req.log.error(err, "Google connect redirect failed");
    res.redirect(`${redirectTo}?google=error`);
  }
});

// GET /google/gmail/callback
router.get("/google/gmail/callback", async (req, res): Promise<void> => {
  const code = req.query.code as string | undefined;
  const error = req.query.error as string | undefined;
  const state = parseOAuthStateCookie(req.headers.cookie);
  const redirectTo = parseOAuthRedirectCookie(req.headers.cookie);

  if (error || !code || !state) {
    req.log.warn({ error, hasCode: !!code, hasState: !!state }, "Google OAuth callback rejected");
    res.redirect(`${redirectTo}?google=error`);
    return;
  }

  try {
    const redirectUri = getRedirectUri(req);
    const tokens = await exchangeCodeForTokens(code, redirectUri);
    await storeGoogleTokens(req.dbUser.id, tokens);
    res.redirect(`${redirectTo}?google=connected`);
  } catch (err) {
    req.log.error(err, "Google OAuth callback failed");
    res.redirect(`${redirectTo}?google=error`);
  }
});

// ─── GMAIL ───────────────────────────────────────────────────────────────

// GET /google/gmail/messages
router.get("/google/gmail/messages", async (req, res): Promise<void> => {
  try {
    const list = await listGmailMessages(req.dbUser.id, 20);
    const messages = await Promise.all(
      (list.messages || []).map(async (m) => {
        const msg = await getGmailMessage(req.dbUser.id, m.id);
        const subject = getHeader(msg, "Subject");
        const from = getHeader(msg, "From");
        const body = getBodyText(msg);
        const priority = await classifyEmailPriority(subject, body);
        return {
          id: msg.id,
          threadId: msg.threadId,
          subject,
          from,
          snippet: msg.snippet || "",
          body,
          internalDate: msg.internalDate,
          priority,
          labelIds: msg.labelIds || [],
        };
      }),
    );
    res.json({ messages });
  } catch (err) {
    req.log.error(err, "Gmail list messages failed");
    res.status(500).json({ error: "Failed to fetch Gmail messages" });
  }
});

// POST /google/gmail/draft
router.post("/google/gmail/draft", async (req, res): Promise<void> => {
  const { messageId, tone } = req.body as { messageId?: string; tone?: string };
  if (!messageId) {
    res.status(400).json({ error: "messageId is required" });
    return;
  }
  try {
    const msg = await getGmailMessage(req.dbUser.id, messageId);
    const thread = await getGmailThread(req.dbUser.id, msg.threadId);
    const threadText = getReplyThreadText(thread);
    const subject = getHeader(msg, "Subject");
    const to = getHeader(msg, "From");

    const response = await openai.chat.completions.create({
      model: "gpt-5.6-luna",
      max_completion_tokens: 1024,
      messages: [
        {
          role: "system",
          content: `You are a helpful email assistant. Write a reply to the email thread below. The tone should be: ${tone || "professional and concise"}. Do not include any salutation or sign-off that isn't appropriate. Return only the reply body, no subject line or explanation.`,
        },
        {
          role: "user",
          content: `Subject: ${subject}\n\nThread:\n${threadText}`,
        },
      ],
    });

    const draft = response.choices[0]?.message?.content?.trim() || "";
    res.json({ to, subject, draft, messageId });
  } catch (err) {
    req.log.error(err, "Gmail draft generation failed");
    res.status(500).json({ error: "Failed to generate reply draft" });
  }
});

// POST /google/gmail/send
router.post("/google/gmail/send", async (req, res): Promise<void> => {
  const { to, subject, body, threadId } = req.body as {
    to?: string;
    subject?: string;
    body?: string;
    threadId?: string;
  };
  if (!to || !subject || !body) {
    res.status(400).json({ error: "to, subject, and body are required" });
    return;
  }
  try {
    const payload = buildEmailRaw(to, subject, body, threadId);
    const sent = await sendGmailMessage(req.dbUser.id, payload);
    res.json({ id: sent.id, threadId: sent.threadId });
  } catch (err) {
    req.log.error(err, "Gmail send failed");
    res.status(500).json({ error: "Failed to send email" });
  }
});

async function classifyEmailPriority(subject: string, body: string): Promise<string> {
  try {
    const response = await openai.chat.completions.create({
      model: "gpt-5.6-luna",
      max_completion_tokens: 32,
      messages: [
        {
          role: "system",
          content:
            'Classify the email into exactly one of these categories: urgent, follow-up, fyi, archive. Return only the single lowercase word, no punctuation.',
        },
        {
          role: "user",
          content: `Subject: ${subject}\n\nBody: ${body.slice(0, 1200)}`,
        },
      ],
    });
    const result = response.choices[0]?.message?.content?.trim().toLowerCase() || "fyi";
    if (["urgent", "follow-up", "fyi", "archive"].includes(result)) return result;
    return "fyi";
  } catch {
    return "fyi";
  }
}

export default router;
