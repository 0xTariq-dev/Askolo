import { Router, type IRouter } from "express";
import { eq, and, desc } from "drizzle-orm";
import { db, actionItemsTable } from "@workspace/db";
import {
  ListActionItemsQueryParams,
  CreateActionItemBody,
  UpdateActionItemParams,
  UpdateActionItemBody,
  DeleteActionItemParams,
} from "@workspace/api-zod";

const router: IRouter = Router();

// GET /action-items
router.get("/action-items", async (req, res): Promise<void> => {
  const query = ListActionItemsQueryParams.safeParse(req.query);
  if (!query.success) {
    res.status(400).json({ error: query.error.message });
    return;
  }

  const conditions = [eq(actionItemsTable.userId, req.dbUser.id)];
  if (query.data.completed !== undefined) {
    conditions.push(eq(actionItemsTable.completed, query.data.completed));
  }

  const items = await db
    .select()
    .from(actionItemsTable)
    .where(and(...conditions))
    .orderBy(desc(actionItemsTable.createdAt));
  res.json(items);
});

// POST /action-items
router.post("/action-items", async (req, res): Promise<void> => {
  const parsed = CreateActionItemBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: parsed.error.message });
    return;
  }
  const [item] = await db
    .insert(actionItemsTable)
    .values({ ...parsed.data, userId: req.dbUser.id })
    .returning();
  res.status(201).json(item);
});

// PATCH /action-items/:id
router.patch("/action-items/:id", async (req, res): Promise<void> => {
  const params = UpdateActionItemParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  const parsed = UpdateActionItemBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: parsed.error.message });
    return;
  }

  const updateData: Record<string, unknown> = { ...parsed.data };
  if (parsed.data.completed === true && !parsed.data.completedAt) {
    updateData.completedAt = new Date();
  } else if (parsed.data.completed === false) {
    updateData.completedAt = null;
  }

  const [item] = await db
    .update(actionItemsTable)
    .set(updateData)
    .where(and(eq(actionItemsTable.id, params.data.id), eq(actionItemsTable.userId, req.dbUser.id)))
    .returning();
  if (!item) {
    res.status(404).json({ error: "Action item not found" });
    return;
  }
  res.json(item);
});

// DELETE /action-items/:id
router.delete("/action-items/:id", async (req, res): Promise<void> => {
  const params = DeleteActionItemParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  await db
    .delete(actionItemsTable)
    .where(and(eq(actionItemsTable.id, params.data.id), eq(actionItemsTable.userId, req.dbUser.id)));
  res.sendStatus(204);
});

export default router;
