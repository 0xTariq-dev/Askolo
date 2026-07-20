import { Router, type IRouter } from "express";
import { openai } from "@workspace/integrations-openai-ai-server";
import { db, dailyPlansTable, actionItemsTable } from "@workspace/db";

const router: IRouter = Router();

interface PlanItem {
  title: string;
  timeBlock: string | null;
  priority: "high" | "medium" | "low";
  notes: string | null;
}

interface MeetingActionItem {
  title: string;
  dueDate?: string | null;
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

// POST /ai/assistant
// Chat with a personal AI assistant that has context about the user's life
router.post("/ai/assistant", async (req, res): Promise<void> => {
  if (!req.isAuthenticated()) {
    res.status(401).json({ error: "Unauthorized" });
    return;
  }

  const { messages = [], context } = req.body as {
    messages?: Array<{ role: "user" | "assistant"; content: string }>;
    context?: {
      habits?: Array<{ name: string; currentStreak: number; completedToday: boolean }>;
      goals?: Array<{ title: string; status: string; progress: number }>;
      todayPlan?: Array<{ title: string; completed: boolean; priority: string }>;
    };
  };

  if (messages.length === 0 || messages[messages.length - 1].role !== "user") {
    res.status(400).json({ error: "messages must end with a user message" });
    return;
  }

  const habitSummary = (context?.habits || [])
    .slice(0, 5)
    .map((h) => `${h.name}${h.completedToday ? " (done today)" : ""}, streak ${h.currentStreak}`)
    .join("; ");

  const goalSummary = (context?.goals || [])
    .filter((g) => g.status === "active")
    .slice(0, 3)
    .map((g) => `${g.title} (${g.progress}% complete)`)
    .join("; ");

  const planSummary = (context?.todayPlan || [])
    .map((p) => `${p.title}${p.completed ? " [done]" : ""} (${p.priority})`)
    .join("; ");

  const systemPrompt = `You are Aura, a warm, focused personal assistant for the user. You know their habits, goals, and today's plan. Use this context to give specific, actionable advice. Keep responses concise and helpful. No emojis. If they ask what to focus on, consider their overdue goals, incomplete habits, and today's plan. If they ask you to decide between options, help them reason through it. If you don't know something, say so.

Context:
- Habits: ${habitSummary || "none tracked yet"}
- Active goals: ${goalSummary || "none set yet"}
- Today's plan: ${planSummary || "nothing planned yet"}`;

  try {
    const response = await openai.chat.completions.create({
      model: "gpt-5.6-luna",
      max_completion_tokens: 1024,
      messages: [
        { role: "system", content: systemPrompt },
        ...messages.map((m) => ({ role: m.role, content: m.content })),
      ],
    });

    const message = response.choices[0]?.message?.content?.trim() ?? "";
    res.json({ message });
  } catch (err) {
    req.log.error(err, "AI assistant chat failed");
    res.status(500).json({ error: "Failed to get assistant response" });
  }
});

// POST /ai/voice-to-plan
// Convert a voice transcript into a structured daily plan and persist it
router.post("/ai/voice-to-plan", async (req, res): Promise<void> => {
  if (!req.isAuthenticated()) {
    res.status(401).json({ error: "Unauthorized" });
    return;
  }

  const { transcript, date } = req.body as { transcript?: string; date?: string };
  if (!transcript || typeof transcript !== "string" || transcript.trim().length === 0) {
    res.status(400).json({ error: "transcript is required" });
    return;
  }

  const targetDate = date || new Date().toISOString().split("T")[0];

  try {
    const response = await openai.chat.completions.create({
      model: "gpt-5.6-luna",
      max_completion_tokens: 2048,
      messages: [
        {
          role: "system",
          content: `You are a personal productivity coach. The user spoke the following voice note. Convert it into a clean, prioritized, time-blocked daily plan. Return ONLY valid JSON — no markdown, no code fences, just raw JSON.

The response must be a JSON array of plan items. Each item has these fields:
- title: string (concise task name, max 60 chars)
- timeBlock: string or null (e.g. "9:00 AM - 10:00 AM", "Morning", "Afternoon")
- priority: "high" | "medium" | "low"
- notes: string or null

Rules:
- Extract the 3-6 most important tasks from the voice note.
- Order by time block then priority.
- Keep titles action-oriented.`,
        },
        {
          role: "user",
          content: `Voice note for ${targetDate}:\n\n${transcript.trim()}`,
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

    const validPriorities = new Set(["high", "medium", "low"]);
    const toInsert = items
      .filter((item) => typeof item.title === "string" && item.title.trim().length > 0)
      .slice(0, 8)
      .map((item) => ({
        userId: req.user.id,
        date: targetDate,
        title: String(item.title).trim().slice(0, 120),
        timeBlock: item.timeBlock ? String(item.timeBlock).slice(0, 60) : null,
        priority: validPriorities.has(item.priority) ? item.priority : ("medium" as const),
        notes: item.notes ? String(item.notes).slice(0, 500) : null,
        completed: false,
      }));

    const created = toInsert.length > 0
      ? await db.insert(dailyPlansTable).values(toInsert).returning()
      : [];

    res.json({ items: created, date: targetDate });
  } catch (err) {
    req.log.error(err, "AI voice-to-plan failed");
    res.status(500).json({ error: "Failed to convert voice note to plan" });
  }
});

// POST /ai/meeting-extract
// Extract summary, decisions, and action items from meeting notes; persist action items
router.post("/ai/meeting-extract", async (req, res): Promise<void> => {
  if (!req.isAuthenticated()) {
    res.status(401).json({ error: "Unauthorized" });
    return;
  }

  const { notes } = req.body as { notes?: string };
  if (!notes || typeof notes !== "string" || notes.trim().length === 0) {
    res.status(400).json({ error: "notes is required" });
    return;
  }

  try {
    const response = await openai.chat.completions.create({
      model: "gpt-5.6-luna",
      max_completion_tokens: 2048,
      messages: [
        {
          role: "system",
          content: `You are a meeting-intelligence assistant. Given raw meeting notes, extract a structured summary. Return ONLY valid JSON — no markdown, no code fences, just raw JSON.

The response must be a JSON object with these fields:
- summary: string (2-3 sentence summary of the meeting)
- decisions: array of strings (key decisions made)
- actionItems: array of objects with { title: string, dueDate: string | null } (YYYY-MM-DD or null)

Rules:
- Be concise and accurate.
- Only include action items that are clearly assigned or implied.
- If no due date is mentioned, set dueDate to null.
- No emojis.`,
        },
        {
          role: "user",
          content: `Meeting notes:\n\n${notes.trim()}`,
        },
      ],
    });

    const content = response.choices[0]?.message?.content ?? "{}";
    let extracted: { summary?: string; decisions?: string[]; actionItems?: MeetingActionItem[] };
    try {
      const parsed = JSON.parse(content);
      extracted = parsed && typeof parsed === "object" ? parsed : {};
    } catch {
      extracted = {};
    }

    const summary = typeof extracted.summary === "string" ? extracted.summary.trim() : "";
    const decisions = Array.isArray(extracted.decisions)
      ? extracted.decisions.filter((d) => typeof d === "string").map(String)
      : [];
    const actionItems = Array.isArray(extracted.actionItems)
      ? extracted.actionItems.filter((a) => typeof a.title === "string")
      : [];

    const dateRegex = /^\d{4}-\d{2}-\d{2}$/;
    const toInsert = actionItems.map((a) => ({
      userId: req.user.id,
      title: String(a.title).trim().slice(0, 200),
      sourceType: "meeting" as const,
      dueDate: a.dueDate && dateRegex.test(a.dueDate) ? a.dueDate : null,
      completed: false,
    }));

    const createdActionItems = toInsert.length > 0
      ? await db.insert(actionItemsTable).values(toInsert).returning()
      : [];

    res.json({
      summary,
      decisions,
      actionItems: createdActionItems,
    });
  } catch (err) {
    req.log.error(err, "AI meeting extract failed");
    res.status(500).json({ error: "Failed to extract meeting intelligence" });
  }
});

export default router;
