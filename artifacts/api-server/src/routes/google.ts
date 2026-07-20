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
} from "../lib/googleCalendar";

const router: IRouter = Router();

const SCOPE_SUMMARY = "calendar";

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
  if (!req.isAuthenticated()) {
    res.status(401).json({ error: "Unauthorized" });
    return;
  }
  const connection = await getOrCreateConnection(req.user.id);
  res.json({
    connected: connection.connected,
    scopes: connection.scopes || [],
    calendar: connection.connected && (connection.scopes || []).includes(SCOPE_SUMMARY),
  });
});

// POST /google/calendar/sync
router.post("/google/calendar/sync", async (req, res): Promise<void> => {
  if (!req.isAuthenticated()) {
    res.status(401).json({ error: "Unauthorized" });
    return;
  }
  const { from, to } = req.body as { from?: string; to?: string };
  if (!from || !to) {
    res.status(400).json({ error: "from and to are required" });
    return;
  }

  try {
    const calendarList = await getGoogleCalendarList();
    const primary = calendarList.items?.find((c) => c.primary) || calendarList.items?.[0];
    if (!primary) {
      res.status(404).json({ error: "No calendar found" });
      return;
    }

    const googleEvents = await listGoogleCalendarEvents(primary.id, from, to);
    const mapped = (googleEvents.items || []).map(formatGoogleEventToLocal);

    // Upsert events into local events table by googleEventId
    for (const event of mapped) {
      if (!event.googleEventId) continue;
      const [existing] = await db
        .select({ id: eventsTable.id })
        .from(eventsTable)
        .where(and(eq(eventsTable.userId, req.user.id), eq(eventsTable.googleEventId, event.googleEventId)));
      if (existing) {
        await db.update(eventsTable).set(event).where(eq(eventsTable.id, existing.id));
      } else {
        await db.insert(eventsTable).values({ ...event, userId: req.user.id, color: "#3b82f6" });
      }
    }

    await db
      .update(googleConnectionsTable)
      .set({ connected: true, scopes: [SCOPE_SUMMARY], updatedAt: new Date() })
      .where(eq(googleConnectionsTable.userId, req.user.id));

    res.json({ synced: mapped.length, calendarId: primary.id });
  } catch (err) {
    req.log.error(err, "Google Calendar sync failed");
    res.status(500).json({ error: "Failed to sync calendar" });
  }
});

// POST /google/calendar/events
router.post("/google/calendar/events", async (req, res): Promise<void> => {
  if (!req.isAuthenticated()) {
    res.status(401).json({ error: "Unauthorized" });
    return;
  }
  const { calendarId, ...event } = req.body as any;
  if (!calendarId) {
    res.status(400).json({ error: "calendarId is required" });
    return;
  }
  try {
    const created = await createGoogleCalendarEvent(calendarId, event);
    res.status(201).json(formatGoogleEventToLocal(created));
  } catch (err) {
    req.log.error(err, "Google Calendar create event failed");
    res.status(500).json({ error: "Failed to create calendar event" });
  }
});

// PATCH /google/calendar/events/:id
router.patch("/google/calendar/events/:id", async (req, res): Promise<void> => {
  if (!req.isAuthenticated()) {
    res.status(401).json({ error: "Unauthorized" });
    return;
  }
  const { calendarId, ...event } = req.body as any;
  const eventId = req.params.id;
  if (!calendarId || !eventId) {
    res.status(400).json({ error: "calendarId and eventId are required" });
    return;
  }
  try {
    const updated = await updateGoogleCalendarEvent(calendarId, eventId, event);
    res.json(formatGoogleEventToLocal(updated));
  } catch (err) {
    req.log.error(err, "Google Calendar update event failed");
    res.status(500).json({ error: "Failed to update calendar event" });
  }
});

// DELETE /google/calendar/events/:id
router.delete("/google/calendar/events/:id", async (req, res): Promise<void> => {
  if (!req.isAuthenticated()) {
    res.status(401).json({ error: "Unauthorized" });
    return;
  }
  const { calendarId } = req.body as { calendarId?: string };
  const eventId = req.params.id;
  if (!calendarId || !eventId) {
    res.status(400).json({ error: "calendarId and eventId are required" });
    return;
  }
  try {
    await deleteGoogleCalendarEvent(calendarId, eventId);
    res.sendStatus(204);
  } catch (err) {
    req.log.error(err, "Google Calendar delete event failed");
    res.status(500).json({ error: "Failed to delete calendar event" });
  }
});

export default router;
