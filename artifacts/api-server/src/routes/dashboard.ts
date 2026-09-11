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
} from "@workspace/db";
import { getGoogleConnectionStatus } from "../lib/googleStatus";

const router: IRouter = Router();

// GET /dashboard/summary
router.get("/dashboard/summary", async (req, res): Promise<void> => {
  const userId = req.dbUser.id;
  const today = new Date().toISOString().split("T")[0];
  const startedAt = Date.now();

  try {
    const [
      habits,
      goals,
      todayPlan,
      upcomingEvents,
      pendingChores,
      openActionItems,
      googleConnection,
      habitCompletions,
    ] = await Promise.all([
      db.select().from(habitsTable).where(eq(habitsTable.userId, userId)),
      db.select().from(goalsTable).where(eq(goalsTable.userId, userId)),
      db
        .select()
        .from(dailyPlansTable)
        .where(and(eq(dailyPlansTable.userId, userId), eq(dailyPlansTable.date, today))),
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
      // Live Google connection status (not stale DB state).
      getGoogleConnectionStatus(userId),
      // Select only what the summary needs and scope completions through the
      // user's habits rather than scanning every user's completion rows.
      db
        .select({ habitId: habitCompletionsTable.habitId })
        .from(habitCompletionsTable)
        .innerJoin(habitsTable, eq(habitCompletionsTable.habitId, habitsTable.id))
        .where(
          and(
            eq(habitCompletionsTable.date, today),
            eq(habitsTable.userId, userId),
          ),
        ),
    ]);

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

    req.log.info(
      {
        requestId: req.id,
        userId,
        durationMs: Date.now() - startedAt,
        habitCount: habits.length,
        goalCount: goals.length,
      },
      "Dashboard summary generated",
    );

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
      googleConnection,
    });
  } catch (error) {
    req.log.error(
      { err: error, requestId: req.id, userId, durationMs: Date.now() - startedAt },
      "Dashboard summary failed",
    );
    res.status(500).json({ error: "Failed to load dashboard summary" });
  }
});

export default router;
