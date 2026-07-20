import { Router, type IRouter } from "express";
import { eq, and, desc } from "drizzle-orm";
import { db, notesTable } from "@workspace/db";
import {
  CreateNoteBody,
  GetNoteParams,
  UpdateNoteParams,
  UpdateNoteBody,
  DeleteNoteParams,
} from "@workspace/api-zod";

const router: IRouter = Router();

// GET /notes
router.get("/notes", async (req, res): Promise<void> => {
  if (!req.isAuthenticated()) {
    res.status(401).json({ error: "Unauthorized" });
    return;
  }
  const notes = await db
    .select()
    .from(notesTable)
    .where(eq(notesTable.userId, req.user.id))
    .orderBy(desc(notesTable.updatedAt));
  res.json(notes);
});

// POST /notes
router.post("/notes", async (req, res): Promise<void> => {
  if (!req.isAuthenticated()) {
    res.status(401).json({ error: "Unauthorized" });
    return;
  }
  const parsed = CreateNoteBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: parsed.error.message });
    return;
  }
  const [note] = await db
    .insert(notesTable)
    .values({ ...parsed.data, userId: req.user.id })
    .returning();
  res.status(201).json(note);
});

// GET /notes/:id
router.get("/notes/:id", async (req, res): Promise<void> => {
  if (!req.isAuthenticated()) {
    res.status(401).json({ error: "Unauthorized" });
    return;
  }
  const params = GetNoteParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  const [note] = await db
    .select()
    .from(notesTable)
    .where(and(eq(notesTable.id, params.data.id), eq(notesTable.userId, req.user.id)));
  if (!note) {
    res.status(404).json({ error: "Note not found" });
    return;
  }
  res.json(note);
});

// PATCH /notes/:id
router.patch("/notes/:id", async (req, res): Promise<void> => {
  if (!req.isAuthenticated()) {
    res.status(401).json({ error: "Unauthorized" });
    return;
  }
  const params = UpdateNoteParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  const parsed = UpdateNoteBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: parsed.error.message });
    return;
  }
  const [note] = await db
    .update(notesTable)
    .set(parsed.data)
    .where(and(eq(notesTable.id, params.data.id), eq(notesTable.userId, req.user.id)))
    .returning();
  if (!note) {
    res.status(404).json({ error: "Note not found" });
    return;
  }
  res.json(note);
});

// DELETE /notes/:id
router.delete("/notes/:id", async (req, res): Promise<void> => {
  if (!req.isAuthenticated()) {
    res.status(401).json({ error: "Unauthorized" });
    return;
  }
  const params = DeleteNoteParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  await db
    .delete(notesTable)
    .where(and(eq(notesTable.id, params.data.id), eq(notesTable.userId, req.user.id)));
  res.sendStatus(204);
});

export default router;
