import { Router, type IRouter } from "express";
import { openai } from "@workspace/integrations-openai-ai-server";
import { db, dailyPlansTable } from "@workspace/db";

const router: IRouter = Router();

interface PlanItem {
  title: string;
  timeBlock: string | null;
  priority: "high" | "medium" | "low";
  notes: string | null;
}

// POST /ai/generate-plan
// Takes raw notes text and returns a structured daily plan
router.post("/ai/generate-plan", async (req, res): Promise<void> => {
  if (!req.isAuthenticated()) {
    res.status(401).json({ error: "Unauthorized" });
    return;
  }

  const { notes, date } = req.body as { notes?: string; date?: string };
  if (!notes || typeof notes !== "string" || notes.trim().length === 0) {
    res.status(400).json({ error: "notes is required" });
    return;
  }

  const today = date || new Date().toISOString().split("T")[0];

  try {
    const response = await openai.chat.completions.create({
      model: "gpt-5.6-luna",
      max_completion_tokens: 2048,
      messages: [
        {
          role: "system",
          content: `You are a personal productivity coach. Given the user's raw notes, intentions, or tasks for the day, organize them into a clean, prioritized, time-blocked daily plan. Return ONLY valid JSON — no markdown, no code fences, just raw JSON.

The response must be a JSON array of plan items. Each item has these fields:
- title: string (concise task name, max 60 chars)
- timeBlock: string or null (e.g. "9:00 AM - 10:00 AM", "Morning", "Afternoon", "Evening")
- priority: "high" | "medium" | "low"
- notes: string or null (brief note or context if helpful)

Rules:
- Identify the 3-6 most important tasks. Don't create more than 8 items.
- Order by time block then priority.
- Use realistic time estimates.
- Mark truly urgent/important items as high priority.
- Keep titles action-oriented.`,
        },
        {
          role: "user",
          content: `Today is ${today}. Here are my rough notes/intentions for today:\n\n${notes.trim()}`,
        },
      ],
    });

    const content = response.choices[0]?.message?.content ?? "[]";
    let items: PlanItem[];
    try {
      const parsed = JSON.parse(content);
      items = Array.isArray(parsed) ? parsed : [];
    } catch {
      items = [];
    }

    // Sanitize and persist each item to daily_plans
    const validPriorities = new Set(["high", "medium", "low"]);
    const toInsert = items
      .filter((item) => typeof item.title === "string" && item.title.trim().length > 0)
      .slice(0, 8)
      .map((item) => ({
        userId: req.user.id,
        date: today,
        title: String(item.title).trim().slice(0, 120),
        timeBlock: item.timeBlock ? String(item.timeBlock).slice(0, 60) : null,
        priority: validPriorities.has(item.priority) ? item.priority : ("medium" as const),
        notes: item.notes ? String(item.notes).slice(0, 500) : null,
        completed: false,
      }));

    const created = toInsert.length > 0
      ? await db.insert(dailyPlansTable).values(toInsert).returning()
      : [];

    res.json({ items: created, date: today });
  } catch (err) {
    req.log.error(err, "AI plan generation failed");
    res.status(500).json({ error: "Failed to generate plan" });
  }
});

// POST /ai/coaching
// Returns a short daily coaching nudge based on habit + goal data
router.post("/ai/coaching", async (req, res): Promise<void> => {
  if (!req.isAuthenticated()) {
    res.status(401).json({ error: "Unauthorized" });
    return;
  }

  const { habits = [], goals = [], habitsCompletedToday = 0, habitsTotal = 0 } = req.body as {
    habits?: Array<{ name: string; currentStreak: number; completedToday: boolean }>;
    goals?: Array<{ title: string; status: string; progress: number }>;
    habitsCompletedToday?: number;
    habitsTotal?: number;
  };

  try {
    const habitSummary = habits
      .slice(0, 5)
      .map((h) => `${h.name}: ${h.currentStreak} day streak${h.completedToday ? " (done today)" : ""}`)
      .join(", ");

    const goalSummary = goals
      .filter((g) => g.status === "active")
      .slice(0, 3)
      .map((g) => `${g.title} (${g.progress}% complete)`)
      .join(", ");

    const response = await openai.chat.completions.create({
      model: "gpt-5.6-luna",
      max_completion_tokens: 200,
      messages: [
        {
          role: "system",
          content: `You are a warm, encouraging personal coach. Write a short (2-3 sentences) daily coaching message for the user based on their habit and goal data. Be specific, genuine, and motivating — not generic or preachy. Vary your tone and focus. No emojis. Return only the message text, nothing else.`,
        },
        {
          role: "user",
          content: `Today's habit progress: ${habitsCompletedToday}/${habitsTotal} habits completed.
Active habits: ${habitSummary || "none yet"}.
Active goals: ${goalSummary || "none yet"}.`,
        },
      ],
    });

    const message = response.choices[0]?.message?.content?.trim() ?? "";
    res.json({ message });
  } catch (err) {
    req.log.error(err, "AI coaching generation failed");
    res.status(500).json({ error: "Failed to generate coaching message" });
  }
});

export default router;
