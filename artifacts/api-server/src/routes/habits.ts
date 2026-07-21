import { Router, type IRouter } from "express";
import { eq, and, desc, gte, lte } from "drizzle-orm";
import { db, habitsTable, habitCompletionsTable } from "@workspace/db";
import {
  CreateHabitBody,
  UpdateHabitParams,
  UpdateHabitBody,
  GetHabitParams,
  DeleteHabitParams,
  GetHabitCompletionsParams,
  CompleteHabitParams,
  CompleteHabitBody,
  UncompleteHabitParams,
  ListHabitCompletionsQueryParams,
} from "@workspace/api-zod";

const router: IRouter = Router();

// Helper: recalculate streak for a habit
async function recalculateStreak(habitId: number): Promise<{ current: number; longest: number }> {
  const completions = await db
    .select()
    .from(habitCompletionsTable)
    .where(eq(habitCompletionsTable.habitId, habitId))
    .orderBy(desc(habitCompletionsTable.date));

  if (completions.length === 0) return { current: 0, longest: 0 };

  const dates = completions.map((c) => c.date).sort().reverse();
  const today = new Date().toISOString().split("T")[0];
  const yesterday = new Date(Date.now() - 86400000).toISOString().split("T")[0];

  let currentStreak = 0;
  let longestStreak = 0;
  let tempStreak = 0;
  let prevDate: string | null = null;

  for (const date of dates) {
    if (prevDate === null) {
      if (date === today || date === yesterday) {
        tempStreak = 1;
      } else {
        tempStreak = 0;
        break;
      }
    } else {
      const prev = new Date(prevDate);
      const cur = new Date(date);
      const diff = Math.round((prev.getTime() - cur.getTime()) / 86400000);
      if (diff === 1) {
        tempStreak++;
      } else {
        break;
      }
    }
    prevDate = date;
  }
  currentStreak = tempStreak;

  // Calculate longest streak
  let streak = 1;
  for (let i = 1; i < dates.length; i++) {
    const prev = new Date(dates[i - 1]);
    const cur = new Date(dates[i]);
    const diff = Math.round((prev.getTime() - cur.getTime()) / 86400000);
    if (diff === 1) {
      streak++;
      if (streak > longestStreak) longestStreak = streak;
    } else {
      streak = 1;
    }
  }
  if (streak > longestStreak) longestStreak = streak;
  if (currentStreak > longestStreak) longestStreak = currentStreak;

  return { current: currentStreak, longest: longestStreak };
}

// GET /habits
router.get("/habits", async (req, res): Promise<void> => {
  const habits = await db
    .select()
    .from(habitsTable)
    .where(eq(habitsTable.userId, req.dbUser.id))
    .orderBy(desc(habitsTable.createdAt));

  const today = new Date().toISOString().split("T")[0];

  const result = await Promise.all(
    habits.map(async (habit) => {
      const todayCompletion = await db
        .select()
        .from(habitCompletionsTable)
        .where(and(eq(habitCompletionsTable.habitId, habit.id), eq(habitCompletionsTable.date, today)));
      return { ...habit, completedToday: todayCompletion.length > 0 };
    }),
  );

  res.json(result);
});

// POST /habits
router.post("/habits", async (req, res): Promise<void> => {
  const parsed = CreateHabitBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: parsed.error.message });
    return;
  }
  const [habit] = await db
    .insert(habitsTable)
    .values({ ...parsed.data, userId: req.dbUser.id })
    .returning();
  res.status(201).json({ ...habit, completedToday: false });
});

// GET /habits/completions — bulk completions for all user habits within a date range
// NOTE: must be registered BEFORE /habits/:id so "completions" is not parsed as an id
router.get("/habits/completions", async (req, res): Promise<void> => {
  const query = ListHabitCompletionsQueryParams.safeParse(req.query);
  if (!query.success) {
    res.status(400).json({ error: query.error.message });
    return;
  }

  const conditions = [eq(habitsTable.userId, req.dbUser.id)];
  if (query.data.from) conditions.push(gte(habitCompletionsTable.date, query.data.from));
  if (query.data.to) conditions.push(lte(habitCompletionsTable.date, query.data.to));

  const completions = await db
    .select({
      id: habitCompletionsTable.id,
      habitId: habitCompletionsTable.habitId,
      date: habitCompletionsTable.date,
      createdAt: habitCompletionsTable.createdAt,
    })
    .from(habitCompletionsTable)
    .innerJoin(habitsTable, eq(habitCompletionsTable.habitId, habitsTable.id))
    .where(and(...conditions))
    .orderBy(habitCompletionsTable.date);

  res.json(completions);
});

// GET /habits/:id
router.get("/habits/:id", async (req, res): Promise<void> => {
  const params = GetHabitParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  const [habit] = await db
    .select()
    .from(habitsTable)
    .where(and(eq(habitsTable.id, params.data.id), eq(habitsTable.userId, req.dbUser.id)));
  if (!habit) {
    res.status(404).json({ error: "Habit not found" });
    return;
  }
  const today = new Date().toISOString().split("T")[0];
  const todayCompletion = await db
    .select()
    .from(habitCompletionsTable)
    .where(and(eq(habitCompletionsTable.habitId, habit.id), eq(habitCompletionsTable.date, today)));
  res.json({ ...habit, completedToday: todayCompletion.length > 0 });
});

// PATCH /habits/:id
router.patch("/habits/:id", async (req, res): Promise<void> => {
  const params = UpdateHabitParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  const parsed = UpdateHabitBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: parsed.error.message });
    return;
  }
  const [habit] = await db
    .update(habitsTable)
    .set(parsed.data)
    .where(and(eq(habitsTable.id, params.data.id), eq(habitsTable.userId, req.dbUser.id)))
    .returning();
  if (!habit) {
    res.status(404).json({ error: "Habit not found" });
    return;
  }
  const today = new Date().toISOString().split("T")[0];
  const todayCompletion = await db
    .select()
    .from(habitCompletionsTable)
    .where(and(eq(habitCompletionsTable.habitId, habit.id), eq(habitCompletionsTable.date, today)));
  res.json({ ...habit, completedToday: todayCompletion.length > 0 });
});

// DELETE /habits/:id
router.delete("/habits/:id", async (req, res): Promise<void> => {
  const params = DeleteHabitParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  await db
    .delete(habitsTable)
    .where(and(eq(habitsTable.id, params.data.id), eq(habitsTable.userId, req.dbUser.id)));
  res.sendStatus(204);
});

// GET /habits/:id/completions
router.get("/habits/:id/completions", async (req, res): Promise<void> => {
  const params = GetHabitCompletionsParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  // Verify ownership before returning completions
  const [habit] = await db
    .select()
    .from(habitsTable)
    .where(and(eq(habitsTable.id, params.data.id), eq(habitsTable.userId, req.dbUser.id)));
  if (!habit) {
    res.status(404).json({ error: "Habit not found" });
    return;
  }
  const completions = await db
    .select()
    .from(habitCompletionsTable)
    .where(eq(habitCompletionsTable.habitId, params.data.id))
    .orderBy(desc(habitCompletionsTable.date));
  res.json(completions);
});

// POST /habits/:id/completions
router.post("/habits/:id/completions", async (req, res): Promise<void> => {
  const params = CompleteHabitParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }
  const parsed = CompleteHabitBody.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: parsed.error.message });
    return;
  }

  // Check habit belongs to user
  const [habit] = await db
    .select()
    .from(habitsTable)
    .where(and(eq(habitsTable.id, params.data.id), eq(habitsTable.userId, req.dbUser.id)));
  if (!habit) {
    res.status(404).json({ error: "Habit not found" });
    return;
  }

  // Upsert completion
  const existing = await db
    .select()
    .from(habitCompletionsTable)
    .where(and(eq(habitCompletionsTable.habitId, params.data.id), eq(habitCompletionsTable.date, parsed.data.date)));
  if (existing.length > 0) {
    res.status(201).json(existing[0]);
    return;
  }

  const [completion] = await db
    .insert(habitCompletionsTable)
    .values({ habitId: params.data.id, date: parsed.data.date })
    .returning();

  // Recalculate streaks
  const { current, longest } = await recalculateStreak(params.data.id);
  await db
    .update(habitsTable)
    .set({ currentStreak: current, longestStreak: Math.max(longest, habit.longestStreak) })
    .where(eq(habitsTable.id, params.data.id));

  res.status(201).json(completion);
});

// DELETE /habits/:id/completions/:date
router.delete("/habits/:id/completions/:date", async (req, res): Promise<void> => {
  const params = UncompleteHabitParams.safeParse(req.params);
  if (!params.success) {
    res.status(400).json({ error: params.error.message });
    return;
  }

  // Check habit belongs to user
  const [habit] = await db
    .select()
    .from(habitsTable)
    .where(and(eq(habitsTable.id, params.data.id), eq(habitsTable.userId, req.dbUser.id)));
  if (!habit) {
    res.status(404).json({ error: "Habit not found" });
    return;
  }

  await db
    .delete(habitCompletionsTable)
    .where(and(eq(habitCompletionsTable.habitId, params.data.id), eq(habitCompletionsTable.date, params.data.date)));

  // Recalculate streaks
  const { current, longest } = await recalculateStreak(params.data.id);
  await db.update(habitsTable).set({ currentStreak: current }).where(eq(habitsTable.id, params.data.id));

  res.sendStatus(204);
});

export default router;
