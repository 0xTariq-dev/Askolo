import { Router, type IRouter } from "express";
import { eq, and, desc } from "drizzle-orm";
import { db, goalsTable } from "@workspace/db";
import {
  CreateGoalBody,
  GetGoalParams,
  UpdateGoalParams,
  UpdateGoalBody,
  DeleteGoalParams,
} from "@workspace/api-zod";

const router: IRouter = Router();

// GET /goals
router.get("/goals", async (req, res): Promise<void> => {
  const goals = await db
    .select()
    .from(goalsTable)
    .where(eq(goalsTable.userId, req.dbUser.id))
    .orderBy(desc(goalsTable.createdAt));
  res.json(goals);
});

// POST /goals
router.post("/goals", async (req, res): Promise<void> => {
  const parsed = CreateGoalBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: parsed.error.message });
    return;
  }
  const [goal] = await db
    .insert(goalsTable)
    .values({ ...parsed.data, userId: req.dbUser.id, status: "active", progress: parsed.data.progress ?? 0 })
    .returning();
  res.status(201).json(goal);
});

// GET /goals/:id
router.get("/goals/:id", async (req, res): Promise<void> => {
  const params = GetGoalParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  const [goal] = await db
    .select()
    .from(goalsTable)
    .where(and(eq(goalsTable.id, params.data.id), eq(goalsTable.userId, req.dbUser.id)));
  if (!goal) {
    res.status(404).json({ error: "Goal not found" });
    return;
  }
  res.json(goal);
});

// PATCH /goals/:id
router.patch("/goals/:id", async (req, res): Promise<void> => {
  const params = UpdateGoalParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  const parsed = UpdateGoalBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: parsed.error.message });
    return;
  }
  const [goal] = await db
    .update(goalsTable)
    .set(parsed.data)
    .where(and(eq(goalsTable.id, params.data.id), eq(goalsTable.userId, req.dbUser.id)))
    .returning();
  if (!goal) {
    res.status(404).json({ error: "Goal not found" });
    return;
  }
  res.json(goal);
});

// DELETE /goals/:id
router.delete("/goals/:id", async (req, res): Promise<void> => {
  const params = DeleteGoalParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  await db
    .delete(goalsTable)
    .where(and(eq(goalsTable.id, params.data.id), eq(goalsTable.userId, req.dbUser.id)));
  res.sendStatus(204);
});

export default router;
