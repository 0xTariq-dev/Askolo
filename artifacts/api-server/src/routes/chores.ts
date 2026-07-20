import { Router, type IRouter } from "express";
import { eq, and, desc } from "drizzle-orm";
import { db, choresTable } from "@workspace/db";
import {
  CreateChoreBody,
  GetChoreParams,
  UpdateChoreParams,
  UpdateChoreBody,
  DeleteChoreParams,
  CompleteChoreParams,
  CompleteChoreBody,
} from "@workspace/api-zod";

const router: IRouter = Router();

// GET /chores
router.get("/chores", async (req, res): Promise<void> => {
  const chores = await db
    .select()
    .from(choresTable)
    .where(eq(choresTable.userId, req.dbUser.id))
    .orderBy(desc(choresTable.createdAt));
  res.json(chores);
});

// POST /chores
router.post("/chores", async (req, res): Promise<void> => {
  const parsed = CreateChoreBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: parsed.error.message });
    return;
  }
  const [chore] = await db
    .insert(choresTable)
    .values({ ...parsed.data, userId: req.dbUser.id, frequency: parsed.data.frequency ?? "once" })
    .returning();
  res.status(201).json(chore);
});

// GET /chores/:id
router.get("/chores/:id", async (req, res): Promise<void> => {
  const params = GetChoreParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  const [chore] = await db
    .select()
    .from(choresTable)
    .where(and(eq(choresTable.id, params.data.id), eq(choresTable.userId, req.dbUser.id)));
  if (!chore) {
    res.status(404).json({ error: "Chore not found" });
    return;
  }
  res.json(chore);
});

// PATCH /chores/:id
router.patch("/chores/:id", async (req, res): Promise<void> => {
  const params = UpdateChoreParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  const parsed = UpdateChoreBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: parsed.error.message });
    return;
  }
  const [chore] = await db
    .update(choresTable)
    .set(parsed.data)
    .where(and(eq(choresTable.id, params.data.id), eq(choresTable.userId, req.dbUser.id)))
    .returning();
  if (!chore) {
    res.status(404).json({ error: "Chore not found" });
    return;
  }
  res.json(chore);
});

// DELETE /chores/:id
router.delete("/chores/:id", async (req, res): Promise<void> => {
  const params = DeleteChoreParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  await db
    .delete(choresTable)
    .where(and(eq(choresTable.id, params.data.id), eq(choresTable.userId, req.dbUser.id)));
  res.sendStatus(204);
});

// POST /chores/:id/complete
router.post("/chores/:id/complete", async (req, res): Promise<void> => {
  const params = CompleteChoreParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  const parsed = CompleteChoreBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: parsed.error.message });
    return;
  }
  const completedAt = parsed.data.completedAt ? new Date(parsed.data.completedAt) : new Date();
  const [chore] = await db
    .update(choresTable)
    .set({ completed: true, completedAt })
    .where(and(eq(choresTable.id, params.data.id), eq(choresTable.userId, req.dbUser.id)))
    .returning();
  if (!chore) {
    res.status(404).json({ error: "Chore not found" });
    return;
  }
  res.json(chore);
});

export default router;
