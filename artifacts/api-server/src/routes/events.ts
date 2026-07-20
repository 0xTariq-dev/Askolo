import { Router, type IRouter } from "express";
import { eq, and, gte, lte, desc } from "drizzle-orm";
import { db, eventsTable } from "@workspace/db";
import {
  ListEventsQueryParams,
  CreateEventBody,
  GetEventParams,
  UpdateEventParams,
  UpdateEventBody,
  DeleteEventParams,
} from "@workspace/api-zod";

const router: IRouter = Router();

// GET /events
router.get("/events", async (req, res): Promise<void> => {
  const query = ListEventsQueryParams.safeParse(req.query);
  if (!query.success) {
    res.status(400).json({ error: query.error.message });
    return;
  }

  const conditions = [eq(eventsTable.userId, req.dbUser.id)];
  if (query.data.from) conditions.push(gte(eventsTable.startDate, query.data.from));
  if (query.data.to) conditions.push(lte(eventsTable.startDate, query.data.to));

  const events = await db
    .select()
    .from(eventsTable)
    .where(and(...conditions))
    .orderBy(eventsTable.startDate, eventsTable.startTime);
  res.json(events);
});

// POST /events
router.post("/events", async (req, res): Promise<void> => {
  const parsed = CreateEventBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: parsed.error.message });
    return;
  }
  const [event] = await db
    .insert(eventsTable)
    .values({ ...parsed.data, userId: req.dbUser.id, allDay: parsed.data.allDay ?? false })
    .returning();
  res.status(201).json(event);
});

// GET /events/:id
router.get("/events/:id", async (req, res): Promise<void> => {
  const params = GetEventParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  const [event] = await db
    .select()
    .from(eventsTable)
    .where(and(eq(eventsTable.id, params.data.id), eq(eventsTable.userId, req.dbUser.id)));
  if (!event) {
    res.status(404).json({ error: "Event not found" });
    return;
  }
  res.json(event);
});

// PATCH /events/:id
router.patch("/events/:id", async (req, res): Promise<void> => {
  const params = UpdateEventParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  const parsed = UpdateEventBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: parsed.error.message });
    return;
  }
  const [event] = await db
    .update(eventsTable)
    .set(parsed.data)
    .where(and(eq(eventsTable.id, params.data.id), eq(eventsTable.userId, req.dbUser.id)))
    .returning();
  if (!event) {
    res.status(404).json({ error: "Event not found" });
    return;
  }
  res.json(event);
});

// DELETE /events/:id
router.delete("/events/:id", async (req, res): Promise<void> => {
  const params = DeleteEventParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  await db
    .delete(eventsTable)
    .where(and(eq(eventsTable.id, params.data.id), eq(eventsTable.userId, req.dbUser.id)));
  res.sendStatus(204);
});

export default router;
