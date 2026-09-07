import { Router, type IRouter, type Request, type Response } from "express";
import { createHash } from "node:crypto";
import { openai } from "@workspace/integrations-openai-ai-server";
import { and, desc, eq } from "drizzle-orm";
import {
  db,
  dailyPlansTable,
  actionItemsTable,
  voicePreferencesTable,
  voiceRecordingsTable,
  voiceRetentionValues,
  type VoiceRetention,
  type VoiceRecording,
} from "@workspace/db";
import {
  CreditLedgerError,
  CreditLimitError,
  estimateCredits,
  getCreditBalance,
  withPricedCreditReservation,
  type CreditPricingKey,
} from "../lib/ai-credit-ledger";
import {
  AssemblyAiError,
  createRealtimeToken,
  transcribeRecordedAudio,
  type AssemblyAiTranscript,
} from "../lib/assemblyai";
import {
  createPrivateAudioPath,
  deletePrivateAudio,
  downloadPrivateAudio,
  uploadPrivateAudio,
} from "../lib/privateAudioStorage";

const router: IRouter = Router();

const MAX_VOICE_TRANSCRIPT_LENGTH = 20_000;
const MAX_AUDIO_BYTES = 8 * 1024 * 1024;
const AUDIO_TRANSCRIPTION_WINDOW_MS = 10 * 60 * 1000;
const AUDIO_TRANSCRIPTION_MAX_REQUESTS = 5;
const AUDIO_TRANSCRIPTION_TIMEOUT_MS = 45_000;
const MAX_AUDIO_DURATION_MS = 2 * 60 * 1000;
const maxAudioBase64Length = Math.ceil(MAX_AUDIO_BYTES / 3) * 4 + 16;
const audioTranscriptionRateLimits = new Map<string, { count: number; windowStartedAt: number }>();

const supportedAudioMimeTypes = new Set([
  "audio/webm",
  "audio/mp4",
  "audio/m4a",
  "audio/wav",
  "audio/ogg",
  "audio/mpeg",
]);

const DEFAULT_VOICE_RETENTION: VoiceRetention = "delete_immediately";
const VOICE_CONSENT_VERSION = "voice-consent-2026-09-07-v1";
const LOW_CONFIDENCE_THRESHOLD = 0.78;

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

function isIsoDate(value: unknown): value is string {
  return typeof value === "string" && /^\d{4}-\d{2}-\d{2}$/.test(value);
}

function isStrictBase64(value: string): boolean {
  return value.length % 4 === 0 && /^[A-Za-z0-9+/]*={0,2}$/.test(value);
}

function fingerprint(value: string | Buffer): string {
  return createHash("sha256").update(value).digest("hex");
}

function requestIdempotencyKey(
  req: Pick<Request, "get">,
  operation: string,
  requestFingerprint: string,
): string {
  const header = req.get("Idempotency-Key")?.trim();
  if (header && !/^[A-Za-z0-9._:-]{8,160}$/.test(header)) {
    throw new CreditLedgerError("Idempotency-Key must be 8-160 safe characters");
  }
  return `${header || `auto:${requestFingerprint}`}:${operation}`.slice(0, 200);
}

async function billModelCall<T>(
  req: Request,
  pricingKey: Extract<CreditPricingKey, `model.${string}`>,
  callback: () => Promise<T>,
): Promise<T> {
  const operation = pricingKey.slice("model.".length);
  const requestFingerprint = fingerprint(`${operation}:${JSON.stringify(req.body)}`);
  const idempotencyKey = requestIdempotencyKey(req, operation, requestFingerprint);
  return withPricedCreditReservation(
    {
      userId: req.dbUser.id,
      pricingKey,
      units: 1,
      idempotencyKey,
      requestFingerprint,
      expiresInSeconds: 120,
      metadata: { operation },
    },
    async () => callback(),
    { evidence: { operation } },
  );
}

function respondToCreditError(
  req: Request,
  res: Response,
  error: unknown,
): boolean {
  if (error instanceof CreditLimitError) {
    res.status(402).json({
      error: error.message,
      code: error.code,
      balance: error.balance,
    });
    return true;
  }
  if (error instanceof CreditLedgerError) {
    req.log.warn({ err: error, requestId: req.id, userId: req.dbUser.id }, "AI credit validation failed");
    res.status(409).json({ error: error.message, code: "AI_CREDIT_LEDGER_CONFLICT" });
    return true;
  }
  return false;
}

// Returns accounting state without exposing prompts, transcripts, or provider payloads.
router.get("/ai/credits", async (req, res): Promise<void> => {
  const balance = await getCreditBalance(req.dbUser.id);
  res.json({ balance, enforcement: "strict" });
});

router.get("/ai/credits/estimate", async (req, res): Promise<void> => {
  const pricingKey = typeof req.query.pricingKey === "string" ? req.query.pricingKey : "";
  const units = typeof req.query.units === "string" ? Number(req.query.units) : 1;
  const publicKeys = new Set<CreditPricingKey>([
    "model.generate-plan",
    "model.coaching",
    "model.assistant",
    "model.voice-to-plan",
    "model.meeting-extract",
    "model.gmail-draft",
    "model.email-priority",
    "transcription.recorded",
    "transcription.realtime",
    "voice.managed-session",
    "tool.read",
    "tool.write",
    "tool.external",
  ]);
  if (!publicKeys.has(pricingKey as CreditPricingKey)) {
    res.status(400).json({ error: "Unknown AI credit pricing key." });
    return;
  }
  try {
    const estimate = estimateCredits(pricingKey as CreditPricingKey, units);
    const balance = await getCreditBalance(req.dbUser.id);
    res.json({
      pricingKey,
      units: estimate.units,
      estimatedCredits: estimate.estimatedCredits,
      unit: estimate.pricing.unit,
      creditsPerUnit: estimate.pricing.creditsPerUnit,
      hardCapCredits: estimate.pricing.maximumCredits,
      availableCredits: balance.availableCredits,
      canReserve: balance.availableCredits >= estimate.estimatedCredits,
    });
  } catch (error) {
    if (respondToCreditError(req, res, error)) return;
    res.status(400).json({ error: "Invalid usage estimate." });
  }
});

function consumeAudioRateLimit(key: string): number | null {
  const now = Date.now();
  const existing = audioTranscriptionRateLimits.get(key);
  if (!existing || now - existing.windowStartedAt >= AUDIO_TRANSCRIPTION_WINDOW_MS) {
    audioTranscriptionRateLimits.set(key, { count: 1, windowStartedAt: now });
    return null;
  }

  if (existing.count >= AUDIO_TRANSCRIPTION_MAX_REQUESTS) {
    return Math.ceil((AUDIO_TRANSCRIPTION_WINDOW_MS - (now - existing.windowStartedAt)) / 1000);
  }

  existing.count += 1;
  return null;
}

async function getVoiceRetention(userId: string): Promise<VoiceRetention> {
  const [preference] = await db
    .select({ retention: voicePreferencesTable.retention })
    .from(voicePreferencesTable)
    .where(eq(voicePreferencesTable.userId, userId))
    .limit(1);
  return preference?.retention ?? DEFAULT_VOICE_RETENTION;
}

async function cleanupExpiredVoiceRecordings(userId: string): Promise<void> {
  const now = new Date();
  const expired = await db
    .select()
    .from(voiceRecordingsTable)
    .where(eq(voiceRecordingsTable.userId, userId));
  for (const recording of expired) {
    if (!recording.expiresAt || recording.expiresAt > now) continue;
    await db.delete(voiceRecordingsTable).where(eq(voiceRecordingsTable.id, recording.id));
    await deletePrivateAudio(recording.objectPath).catch(() => undefined);
  }
}

function serializeVoiceRecording(recording: VoiceRecording) {
  return {
    id: recording.id,
    planDate: recording.planDate,
    mimeType: recording.mimeType,
    durationMs: recording.durationMs,
    transcript: recording.transcript,
    audioUrl: `/api/ai/voice-recordings/${recording.id}/audio`,
    createdAt: recording.createdAt.toISOString(),
    expiresAt: recording.expiresAt?.toISOString() ?? null,
  };
}

function isVoiceRetention(value: unknown): value is VoiceRetention {
  return typeof value === "string" && (voiceRetentionValues as readonly string[]).includes(value);
}

function createReviewSignals(transcript: AssemblyAiTranscript) {
  return (transcript.words ?? [])
    .filter((word) => {
      const text = word.text?.trim() ?? "";
      return Boolean(text) &&
        typeof word.confidence === "number" &&
        word.confidence < LOW_CONFIDENCE_THRESHOLD &&
        /[\d@#$%]|(?:am|pm)$/i.test(text);
    })
    .slice(0, 20)
    .map((word) => ({
      kind: "low_confidence_entity" as const,
      text: word.text?.trim() ?? "",
      confidence: word.confidence ?? 0,
      startMs: word.start ?? null,
      endMs: word.end ?? null,
    }));
}

function respondToAssemblyAiError(req: Request, res: Response, error: unknown): boolean {
  if (!(error instanceof AssemblyAiError)) return false;
  const status = error.code === "cancelled"
    ? 499
    : error.code === "provider_rejected"
      ? 422
      : error.code === "not_configured"
        ? 503
        : 502;
  if (status >= 500) {
    req.log.error({
      requestId: req.id,
      userId: req.dbUser.id,
      code: error.code,
      failureReason: error.failureReason,
    }, "AssemblyAI transcription failed");
  }
  res.status(status).json({ error: error.message, code: `ASSEMBLYAI_${error.code.toUpperCase()}` });
  return true;
}

// POST /ai/generate-plan
// Takes raw notes text and returns a structured daily plan
router.post("/ai/generate-plan", async (req, res): Promise<void> => {

  const { notes, date } = req.body as { notes?: string; date?: string };
  if (!notes || typeof notes !== "string" || notes.trim().length === 0) {
    res.status(400).json({ error: "notes is required" });
    return;
  }
  if (notes.length > 20_000) {
    res.status(413).json({ error: "Notes are too long for AI planning." });
    return;
  }

  const today = date || new Date().toISOString().split("T")[0];

  try {
    const response = await billModelCall(req, "model.generate-plan", () => openai.chat.completions.create({
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
    }));

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
        userId: req.dbUser.id,
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
    if (respondToCreditError(req, res, err)) return;
    req.log.error(err, "AI plan generation failed");
    res.status(500).json({ error: "Failed to generate plan" });
  }
});

// POST /ai/coaching
// Returns a short daily coaching nudge based on habit + goal data
router.post("/ai/coaching", async (req, res): Promise<void> => {

  const { habits = [], goals = [], habitsCompletedToday = 0, habitsTotal = 0 } = req.body as {
    habits?: Array<{ name: string; currentStreak: number; completedToday: boolean }>;
    goals?: Array<{ title: string; status: string; progress: number }>;
    habitsCompletedToday?: number;
    habitsTotal?: number;
  };
  if (JSON.stringify(req.body).length > 50_000 || habits.length > 100 || goals.length > 100) {
    res.status(413).json({ error: "Coaching context is too large." });
    return;
  }

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

    const response = await billModelCall(req, "model.coaching", () => openai.chat.completions.create({
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
    }));

    const message = response.choices[0]?.message?.content?.trim() ?? "";
    res.json({ message });
  } catch (err) {
    if (respondToCreditError(req, res, err)) return;
    req.log.error(err, "AI coaching generation failed");
    res.status(500).json({ error: "Failed to generate coaching message" });
  }
});

// POST /ai/assistant
// Chat with a personal AI assistant that has context about the user's life
router.post("/ai/assistant", async (req, res): Promise<void> => {

  const { messages = [], context } = req.body as {
    messages?: Array<{ role: "user" | "assistant"; content: string }>;
    context?: {
      habits?: Array<{ name: string; currentStreak: number; completedToday: boolean }>;
      goals?: Array<{ title: string; status: string; progress: number }>;
      todayPlan?: Array<{ title: string; completed: boolean; priority: string }>;
      upcomingEvents?: Array<{ title: string; startDate: string; startTime?: string | null; allDay: boolean }>;
      recentEmails?: Array<{ subject: string; from: string; priority: string }>;
    };
  };

  if (messages.length === 0 || messages[messages.length - 1].role !== "user") {
    res.status(400).json({ error: "messages must end with a user message" });
    return;
  }
  const messageCharacters = messages.reduce(
    (total, message) => total + (typeof message.content === "string" ? message.content.length : 0),
    0,
  );
  if (
    messages.length > 40 ||
    messageCharacters > 60_000 ||
    messages.some((message) => !message.content || message.content.length > 20_000) ||
    JSON.stringify(context ?? {}).length > 60_000
  ) {
    res.status(413).json({ error: "Assistant context is too large." });
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

  const eventSummary = (context?.upcomingEvents || [])
    .slice(0, 7)
    .map((e) => {
      const when = e.allDay ? e.startDate : `${e.startDate}${e.startTime ? " at " + e.startTime : ""}`;
      return `${e.title} (${when})`;
    })
    .join("; ");

  const emailSummary = (context?.recentEmails || [])
    .slice(0, 10)
    .map((e) => `[${e.priority}] "${e.subject}" from ${e.from}`)
    .join("; ");

  const today = new Date().toISOString().split("T")[0];

  const systemPrompt = `You are Askolo, a warm, focused personal assistant. Today is ${today}. You have full context about the user's habits, goals, schedule, and emails. Use this context to give specific, actionable advice. Keep responses concise. No emojis. If asked what to focus on, consider their overdue goals, incomplete habits, and today's plan. If asked about their schedule or emails, reference the actual data. If you don't know something, say so.

Context:
- Habits: ${habitSummary || "none tracked yet"}
- Active goals: ${goalSummary || "none set yet"}
- Today's plan: ${planSummary || "nothing planned yet"}
- Upcoming events (next 7 days): ${eventSummary || "no events"}
- Recent emails: ${emailSummary || "no emails or Gmail not connected"}

NOTIFICATIONS: When you want to surface a reminder or important alert (e.g. an upcoming deadline, an overdue habit, or an important email), append a JSON block on its own line at the very end of your response using this exact format:
[NOTIFY:{"type":"info","title":"Short title","body":"One-sentence explanation","action":{"label":"Button label","href":"/relevant-route"}}]
The "action" field is optional. Valid types: "info", "warning", "action". Only emit a notification when there is something genuinely urgent or worth highlighting — not on every response.`;

  try {
    const response = await billModelCall(req, "model.assistant", () => openai.chat.completions.create({
      model: "gpt-5.6-luna",
      max_completion_tokens: 1024,
      messages: [
        { role: "system", content: systemPrompt },
        ...messages.map((m) => ({ role: m.role, content: m.content })),
      ],
    }));

    let raw = response.choices[0]?.message?.content?.trim() ?? "";

    // Parse optional notification block from end of response.
    type NotificationPayload = {
      type?: string;
      title?: string;
      body?: string;
      action?: { label?: string; href?: string };
    };
    let notification: NotificationPayload | undefined;
    const notifyMatch = raw.match(/\[NOTIFY:(\{.*?\})\]\s*$/s);
    if (notifyMatch) {
      try {
        notification = JSON.parse(notifyMatch[1]);
      } catch { /* ignore malformed blocks */ }
      raw = raw.slice(0, notifyMatch.index).trim();
    }

    res.json({ message: raw, ...(notification ? { notification } : {}) });
  } catch (err) {
    if (respondToCreditError(req, res, err)) return;
    req.log.error(err, "AI assistant chat failed");
    res.status(500).json({ error: "Failed to get assistant response" });
  }
});

// POST /ai/transcribe-audio
// Transcribe a short, user-recorded voice note for client-side review.
router.post("/ai/transcribe-audio", async (req, res): Promise<void> => {
  const startedAt = Date.now();
  const userRetryAfter = consumeAudioRateLimit(`user:${req.dbUser.id}`);
  const ipRetryAfter = consumeAudioRateLimit(`ip:${req.ip ?? "unknown"}`);
  const retryAfter = Math.max(userRetryAfter ?? 0, ipRetryAfter ?? 0);

  if (retryAfter > 0) {
    res.setHeader("Retry-After", retryAfter);
    res.status(429).json({ error: "Too many voice transcription attempts. Try again shortly." });
    return;
  }

  const body = req.body as {
    audioBase64?: unknown;
    mimeType?: unknown;
    durationMs?: unknown;
    language?: unknown;
    retention?: unknown;
    planDate?: unknown;
  };
  const audioBase64 = body?.audioBase64;
  const mimeType = body?.mimeType;
  const durationMs = body?.durationMs;
  const language = typeof body?.language === "string" ? body.language.slice(0, 20) : "en-US";
  const retention = isVoiceRetention(body?.retention)
    ? body.retention
    : await getVoiceRetention(req.dbUser.id);
  const planDate = isIsoDate(body?.planDate)
    ? body.planDate
    : new Date().toISOString().slice(0, 10);

  if (
    typeof audioBase64 !== "string" ||
    audioBase64.length === 0 ||
    audioBase64.length > maxAudioBase64Length ||
    !isStrictBase64(audioBase64)
  ) {
    res.status(400).json({ error: "A valid audio recording is required." });
    return;
  }

  if (typeof mimeType !== "string") {
    res.status(400).json({ error: "Audio format is required." });
    return;
  }
  if (
    durationMs !== undefined &&
    (typeof durationMs !== "number" ||
      !Number.isSafeInteger(durationMs) ||
      durationMs <= 0 ||
      durationMs > MAX_AUDIO_DURATION_MS)
  ) {
    res.status(400).json({ error: "Audio duration is invalid." });
    return;
  }

  const normalizedMimeType = mimeType.split(";")[0].trim().toLowerCase();
  if (!supportedAudioMimeTypes.has(normalizedMimeType)) {
    res.status(415).json({ error: "This recording format is not supported." });
    return;
  }

  let audioBuffer: Buffer;
  try {
    audioBuffer = Buffer.from(audioBase64, "base64");
  } catch {
    res.status(400).json({ error: "The audio recording could not be read." });
    return;
  }

  if (audioBuffer.length === 0 || audioBuffer.length > MAX_AUDIO_BYTES) {
    res.status(413).json({ error: "The recording is too large. Keep voice notes under 2 minutes." });
    return;
  }

  const controller = new AbortController();
  const abortRequest = () => controller.abort();
  req.once("close", abortRequest);
  try {
    // Client duration is only used to reserve a bounded amount of credit. The
    // provider's duration is authoritative when the reservation settles.
    const estimatedCredits = Math.max(1, Math.ceil((durationMs as number) / 1000));
    const requestFingerprint = fingerprint(audioBuffer);
    const idempotencyKey = requestIdempotencyKey(req, "transcribe-audio", requestFingerprint);
    const providerTranscript: AssemblyAiTranscript = await withPricedCreditReservation(
      {
        userId: req.dbUser.id,
        pricingKey: "transcription.recorded",
        units: estimatedCredits,
        expiresInSeconds: Math.ceil(AUDIO_TRANSCRIPTION_TIMEOUT_MS / 1000) + 15,
        idempotencyKey,
        requestFingerprint,
        metadata: {
          route: "transcribe-audio",
          audioFormat: normalizedMimeType,
          retention,
          provider: "assemblyai",
          region: "us",
        },
      },
      async () => {
        return transcribeRecordedAudio({
          audio: audioBuffer,
          language,
          signal: controller.signal,
        });
      },
      {
        actualUnits: estimatedCredits,
        connectedDurationMs: durationMs as number,
        evidence: {
          route: "transcribe-audio",
          provider: "assemblyai",
          region: "us",
        },
      },
    );

    const transcript = providerTranscript;
    const spokenText = transcript.text?.trim().slice(0, MAX_VOICE_TRANSCRIPT_LENGTH) ?? "";
    if (!spokenText) {
      res.status(422).json({ error: "No speech was detected in the recording." });
      return;
    }

    let recording: ReturnType<typeof serializeVoiceRecording> | null = null;
    if (retention !== "delete_immediately") {
      try {
        const objectPath = createPrivateAudioPath(req.dbUser.id);
        await uploadPrivateAudio({
          objectPath,
          audio: audioBuffer,
          contentType: normalizedMimeType,
        });
        const [savedRecording] = await db
          .insert(voiceRecordingsTable)
          .values({
            userId: req.dbUser.id,
            planDate,
            objectPath,
            mimeType: normalizedMimeType,
            durationMs: durationMs as number,
            transcript: spokenText,
            expiresAt: retention === "keep_24_hours"
              ? new Date(Date.now() + 24 * 60 * 60 * 1000)
              : null,
          })
          .returning();
        if (savedRecording) recording = serializeVoiceRecording(savedRecording);
      } catch (storageError) {
        req.log.error({
          requestId: req.id,
          userId: req.dbUser.id,
          error: storageError instanceof Error ? storageError.message : "unknown",
        }, "Retained voice recording could not be stored");
      }
    }

    req.log.info({
      requestId: req.id,
      userId: req.dbUser.id,
      audioBytes: audioBuffer.length,
      durationMs: Date.now() - startedAt,
      provider: "assemblyai",
      region: "us",
      retention,
    }, "Voice transcription completed");
    res.json({
      transcript: spokenText,
      confidence: typeof transcript.confidence === "number" ? transcript.confidence : null,
      reviewSignals: createReviewSignals(transcript),
      retention,
      deletion: {
        rawAudio: recording ? "stored_until_expiry" : "not_stored",
        providerTranscript: "deleted",
        marker: retention === "delete_immediately"
          ? "Audio and provider transcript deleted after transcription."
          : recording
            ? "Audio is stored privately until the selected retention period."
            : "Audio storage was unavailable; provider transcript deleted after transcription.",
      },
      recording,
    });
  } catch (err) {
    if (respondToCreditError(req, res, err)) return;
    if (respondToAssemblyAiError(req, res, err)) return;
    req.log.error({ requestId: req.id, userId: req.dbUser.id }, "Voice transcription failed");
    res.status(502).json({ error: "Voice transcription is temporarily unavailable. Try again or type your plan." });
  } finally {
    req.off("close", abortRequest);
  }
});

router.get("/ai/transcription-preferences", async (req, res): Promise<void> => {
  res.json({
    retention: await getVoiceRetention(req.dbUser.id),
    options: [
      {
        value: "delete_immediately",
        label: "Delete after transcription",
        description: "Recommended. Raw audio is never stored and the provider transcript is deleted after processing.",
      },
      {
        value: "until_review",
        label: "Keep until review",
        description: "Keep the reviewed text in this session only; raw audio is never stored.",
      },
      {
        value: "keep_24_hours",
        label: "Keep for 24 hours",
        description: "Keep the reviewed text in this session for up to 24 hours; raw audio is never stored.",
      },
    ],
  });
});

router.patch("/ai/transcription-preferences", async (req, res): Promise<void> => {
  const retention = (req.body as { retention?: unknown })?.retention;
  if (!isVoiceRetention(retention)) {
    res.status(400).json({ error: "A valid voice-data retention choice is required." });
    return;
  }
  const [preference] = await db
    .insert(voicePreferencesTable)
    .values({ userId: req.dbUser.id, retention })
    .onConflictDoUpdate({
      target: voicePreferencesTable.userId,
      set: { retention, updatedAt: new Date() },
    })
    .returning({ retention: voicePreferencesTable.retention });
  res.json({ retention: preference?.retention ?? retention });
});

router.post("/ai/realtime-token", async (req, res): Promise<void> => {
  try {
    const token = await createRealtimeToken();
    res.json({
      ...token,
      retention: await getVoiceRetention(req.dbUser.id),
      speechModel: "universal-3-5-pro",
      redaction: "provider_pii_redaction",
    });
  } catch (err) {
    if (respondToAssemblyAiError(req, res, err)) return;
    res.status(502).json({ error: "A real-time transcription session could not be started." });
  }
});

// POST /ai/voice-to-plan
// Convert a voice transcript into a structured daily plan and persist it
router.post("/ai/voice-to-plan", async (req, res): Promise<void> => {

  const { transcript, date } = req.body as { transcript?: string; date?: string };
  if (!transcript || typeof transcript !== "string" || transcript.trim().length === 0) {
    res.status(400).json({ error: "transcript is required" });
    return;
  }
  if (transcript.length > MAX_VOICE_TRANSCRIPT_LENGTH) {
    res.status(413).json({ error: "Voice transcript is too long." });
    return;
  }

  const targetDate = date || new Date().toISOString().split("T")[0];

  try {
    const response = await billModelCall(req, "model.voice-to-plan", () => openai.chat.completions.create({
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
    }));

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
        userId: req.dbUser.id,
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
    if (respondToCreditError(req, res, err)) return;
    req.log.error(err, "AI voice-to-plan failed");
    res.status(500).json({ error: "Failed to convert voice note to plan" });
  }
});

// POST /ai/meeting-extract
// Extract summary, decisions, and action items from meeting notes; persist action items
router.post("/ai/meeting-extract", async (req, res): Promise<void> => {

  const { notes } = req.body as { notes?: string };
  if (!notes || typeof notes !== "string" || notes.trim().length === 0) {
    res.status(400).json({ error: "notes is required" });
    return;
  }
  if (notes.length > 20_000) {
    res.status(413).json({ error: "Meeting notes are too long." });
    return;
  }

  try {
    const response = await billModelCall(req, "model.meeting-extract", () => openai.chat.completions.create({
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
    }));

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
      userId: req.dbUser.id,
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
    if (respondToCreditError(req, res, err)) return;
    req.log.error(err, "AI meeting extract failed");
    res.status(500).json({ error: "Failed to extract meeting intelligence" });
  }
});

export default router;
