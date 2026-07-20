import { Router, type IRouter } from "express";
import { eq, and, desc } from "drizzle-orm";
import { db, dailyPlansTable } from "@workspace/db";
import {
  ListDailyPlansQueryParams,
  CreateDailyPlanBody,
  UpdateDailyPlanParams,
  UpdateDailyPlanBody,
  DeleteDailyPlanParams,
} from "@workspace/api-zod";

const router: IRouter = Router();

// GET /daily-plans
router.get("/daily-plans", async (req, res): Promise<void> => {
  const query = ListDailyPlansQueryParams.safeParse(req.query);
  if (!query.success) {
    res.status(400).json({ error: query.error.message });
    return;
  }

  const conditions = [eq(dailyPlansTable.userId, req.dbUser.id)];
  if (query.data.date) {
    conditions.push(eq(dailyPlansTable.date, query.data.date));
  }

  const plans = await db
    .select()
    .from(dailyPlansTable)
    .where(and(...conditions))
    .orderBy(dailyPlansTable.date, dailyPlansTable.id);
  res.json(plans);
});

// POST /daily-plans
router.post("/daily-plans", async (req, res): Promise<void> => {
  const parsed = CreateDailyPlanBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: parsed.error.message });
    return;
  }
  const [plan] = await db
    .insert(dailyPlansTable)
    .values({ ...parsed.data, userId: req.dbUser.id, priority: parsed.data.priority ?? "medium" })
    .returning();
  res.status(201).json(plan);
});

// PATCH /daily-plans/:id
router.patch("/daily-plans/:id", async (req, res): Promise<void> => {
  const params = UpdateDailyPlanParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  const parsed = UpdateDailyPlanBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: parsed.error.message });
    return;
  }
  const [plan] = await db
    .update(dailyPlansTable)
    .set(parsed.data)
    .where(and(eq(dailyPlansTable.id, params.data.id), eq(dailyPlansTable.userId, req.dbUser.id)))
    .returning();
  if (!plan) {
    res.status(404).json({ error: "Daily plan not found" });
    return;
  }
  res.json(plan);
});

// DELETE /daily-plans/:id
router.delete("/daily-plans/:id", async (req, res): Promise<void> => {
  const params = DeleteDailyPlanParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  await db
    .delete(dailyPlansTable)
    .where(and(eq(dailyPlansTable.id, params.data.id), eq(dailyPlansTable.userId, req.dbUser.id)));
  res.sendStatus(204);
});

export default router;
