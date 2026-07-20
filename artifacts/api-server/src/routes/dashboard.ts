import { Router, type IRouter } from "express";
import { eq, and, gte } from "drizzle-orm";
import {
  db,
  habitsTable,
  habitCompletionsTable,
  goalsTable,
  dailyPlansTable,
  eventsTable,
  choresTable,
  actionItemsTable,
  googleConnectionsTable,
} from "@workspace/db";

const router: IRouter = Router();

// GET /dashboard/summary
router.get("/dashboard/summary", async (req, res): Promise<void> => {
  if (!req.isAuthenticated()) {
    res.status(401).json({ error: "Unauthorized" });
    return;
  }

  const userId = req.user.id;
  const today = new Date().toISOString().split("T")[0];
  const oneWeekLater = new Date(Date.now() + 7 * 86400000).toISOString().split("T")[0];

  // Fetch all in parallel
  const [habits, goals, todayPlan, upcomingEvents, pendingChores, openActionItems, googleConnection] =
    await Promise.all([
      db.select().from(habitsTable).where(eq(habitsTable.userId, userId)),
      db.select().from(goalsTable).where(eq(goalsTable.userId, userId)),
      db.select().from(dailyPlansTable).where(and(eq(dailyPlansTable.userId, userId), eq(dailyPlansTable.date, today))),
      db
        .select()
        .from(eventsTable)
        .where(and(eq(eventsTable.userId, userId), gte(eventsTable.startDate, today)))
        .orderBy(eventsTable.startDate)
        .limit(5),
      db
        .select()
        .from(choresTable)
        .where(and(eq(choresTable.userId, userId), eq(choresTable.completed, false)))
        .limit(5),
      db
        .select()
        .from(actionItemsTable)
        .where(and(eq(actionItemsTable.userId, userId), eq(actionItemsTable.completed, false)))
        .limit(10),
      db.select().from(googleConnectionsTable).where(eq(googleConnectionsTable.userId, userId)).limit(1),
    ]);

  // Get today's completions for habits
  const habitCompletions = await db
    .select()
    .from(habitCompletionsTable)
    .where(eq(habitCompletionsTable.date, today));

  const completedHabitIds = new Set(habitCompletions.map((c) => c.habitId));

  const habitSummary = habits.map((h) => ({
    id: h.id,
    name: h.name,
    currentStreak: h.currentStreak,
    longestStreak: h.longestStreak,
    completedToday: completedHabitIds.has(h.id),
    color: h.color,
    icon: h.icon,
  }));

  const habitsCompletedToday = habitSummary.filter((h) => h.completedToday).length;
  const habitsTotal = habits.length;
  const goalsActive = goals.filter((g) => g.status === "active").length;
  const goalsCompleted = goals.filter((g) => g.status === "completed").length;

  res.json({
    habits: habitSummary,
    goals,
    todayPlan,
    upcomingEvents: upcomingEvents.map((e) => ({
      id: e.id,
      title: e.title,
      startDate: e.startDate,
      startTime: e.startTime,
      allDay: e.allDay,
      color: e.color,
    })),
    pendingChores,
    openActionItems,
    habitsCompletedToday,
    habitsTotal,
    goalsActive,
    goalsCompleted,
    googleConnection: googleConnection[0] || { connected: false, scopes: [] },
  });
});

export default router;
