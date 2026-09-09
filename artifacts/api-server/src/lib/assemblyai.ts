import { AssemblyAI } from "assemblyai";
import { logger } from "./logger";

export const ASSEMBLYAI_REGION = "us" as const;
export const ASSEMBLYAI_REST_BASE_URL = "https://api.assemblyai.com";
export const ASSEMBLYAI_STREAMING_BASE_URL = "https://streaming.us.assemblyai.com";
export const ASSEMBLYAI_STREAMING_WEBSOCKET_URL = "wss://streaming.us.assemblyai.com/v3/ws";
export const ASSEMBLYAI_TOKEN_TTL_SECONDS = 60;
export const ASSEMBLYAI_MAX_SESSION_SECONDS = 120;

export type AssemblyAiWord = {
  text?: string | null;
  confidence?: number | null;
  start?: number | null;
  end?: number | null;
};

export type AssemblyAiTranscript = {
  id: string;
  status: string;
  text?: string | null;
  confidence?: number | null;
  audio_duration?: number | null;
  speech_model_used?: string | null;
  words?: AssemblyAiWord[] | null;
  error?: string | null;
};

export type RecordedAudioTranscription = {
  transcript: AssemblyAiTranscript;
  providerTranscriptDeleted: boolean;
};

type AssemblyAiFailureReason =
  | "authentication"
  | "invalid_request"
  | "network"
  | "timeout"
  | "unknown";

export class AssemblyAiError extends Error {
  readonly code: "not_configured" | "provider_unavailable" | "provider_rejected" | "cancelled";
  readonly failureReason?: AssemblyAiFailureReason;

  constructor(
    code: AssemblyAiError["code"],
    message: string,
    failureReason?: AssemblyAiFailureReason,
  ) {
    super(message);
    this.name = "AssemblyAiError";
    this.code = code;
    this.failureReason = failureReason;
  }
}

function assertUsRegion(): void {
  const configuredRegion = (process.env.ASSEMBLYAI_REGION ?? ASSEMBLYAI_REGION).trim().toLowerCase();
  if (configuredRegion !== ASSEMBLYAI_REGION) {
    throw new AssemblyAiError(
      "provider_rejected",
      "AssemblyAI transcription is currently pinned to the US region.",
    );
  }
}

function getAssemblyAiClient(): AssemblyAI {
  assertUsRegion();
  const apiKey = process.env.ASSEMBLY_AI_API_KEY;
  if (!apiKey) {
    throw new AssemblyAiError(
      "not_configured",
      "AssemblyAI transcription is not configured.",
    );
  }

  return new AssemblyAI({
    apiKey,
    baseUrl: ASSEMBLYAI_REST_BASE_URL,
    streamingBaseUrl: ASSEMBLYAI_STREAMING_BASE_URL,
  });
}

function throwIfAborted(signal?: AbortSignal): void {
  if (signal?.aborted) {
    throw new AssemblyAiError("cancelled", "Transcription was canceled.");
  }
}

function waitFor(milliseconds: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(resolve, milliseconds);
    const abort = () => {
      clearTimeout(timeout);
      reject(new AssemblyAiError("cancelled", "Transcription was canceled."));
    };
    if (signal?.aborted) {
      abort();
      return;
    }
    signal?.addEventListener("abort", abort, { once: true });
  });
}

function mapProviderError(error: unknown): AssemblyAiError {
  if (error instanceof AssemblyAiError) return error;
  const message = error instanceof Error ? error.message.toLowerCase() : "";
  const failureReason: AssemblyAiFailureReason =
    /unauthori[sz]ed|forbidden|api[ _-]?key|authentication/.test(message)
      ? "authentication"
      : /invalid|unsupported|required|must|parameter|policy|request/.test(message)
        ? "invalid_request"
        : /timeout|timed out/.test(message)
          ? "timeout"
          : /fetch|network|connect|socket|dns|econn/.test(message)
            ? "network"
            : "unknown";
  return new AssemblyAiError(
    "provider_unavailable",
    "AssemblyAI transcription is temporarily unavailable.",
    failureReason,
  );
}

export async function transcribeRecordedAudio(params: {
  audio: Buffer;
  language?: string;
  signal?: AbortSignal;
}): Promise<RecordedAudioTranscription> {
  const client = getAssemblyAiClient();
  let providerTranscriptId: string | null = null;
  let completedTranscript: AssemblyAiTranscript | null = null;
  let providerTranscriptDeleted = false;

  try {
    throwIfAborted(params.signal);
    const submitted = await client.transcripts.submit({
      audio: params.audio,
      speech_models: ["universal-3-5-pro", "universal-2"],
      language_code: params.language?.split("-")[0] || "en",
      punctuate: true,
      format_text: true,
      redact_pii: true,
      redact_pii_policies: [
        "person_name",
        "email_address",
        "phone_number",
        "account_number",
        "credit_card_number",
        "date_of_birth",
      ],
      redact_pii_sub: "entity_name",
      redact_pii_return_unredacted: false,
    });
    providerTranscriptId = submitted.id;

    const deadline = Date.now() + 45_000;
    while (Date.now() < deadline) {
      throwIfAborted(params.signal);
      const transcript = await client.transcripts.get(submitted.id) as AssemblyAiTranscript;
      if (transcript.status === "completed") {
        completedTranscript = transcript;
        break;
      }
      if (transcript.status === "error") {
        throw new AssemblyAiError(
          "provider_rejected",
          "AssemblyAI could not transcribe this recording.",
        );
      }
      await waitFor(1_500, params.signal);
    }

    throw new AssemblyAiError(
      "provider_unavailable",
      "AssemblyAI transcription took too long. Try a shorter recording.",
    );
  } catch (error) {
    throw mapProviderError(error);
  } finally {
    if (providerTranscriptId) {
      try {
        await client.transcripts.delete(providerTranscriptId);
        providerTranscriptDeleted = true;
      } catch {
        logger.warn(
          { provider: "assemblyai" },
          "AssemblyAI transcript deletion could not be confirmed",
        );
      }
    }
  }

  const transcript = completedTranscript;
  if (!transcript) {
    throw new AssemblyAiError(
      "provider_unavailable",
      "AssemblyAI did not return a completed transcript.",
    );
  }

  return {
    transcript: transcript as AssemblyAiTranscript,
    providerTranscriptDeleted,
  };
}

export async function createRealtimeToken(): Promise<{
  token: string;
  expiresInSeconds: number;
  maxSessionDurationSeconds: number;
  region: typeof ASSEMBLYAI_REGION;
  websocketUrl: string;
}> {
  const client = getAssemblyAiClient();
  try {
    const token = await client.streaming.createTemporaryToken({
      expires_in_seconds: ASSEMBLYAI_TOKEN_TTL_SECONDS,
      max_session_duration_seconds: ASSEMBLYAI_MAX_SESSION_SECONDS,
    });
    return {
      token,
      expiresInSeconds: ASSEMBLYAI_TOKEN_TTL_SECONDS,
      maxSessionDurationSeconds: ASSEMBLYAI_MAX_SESSION_SECONDS,
      region: ASSEMBLYAI_REGION,
      websocketUrl: ASSEMBLYAI_STREAMING_WEBSOCKET_URL,
    };
  } catch {
    throw new AssemblyAiError(
      "provider_unavailable",
      "A real-time transcription session could not be started.",
    );
  }
}