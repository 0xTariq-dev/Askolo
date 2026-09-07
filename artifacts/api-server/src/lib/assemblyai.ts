import { AssemblyAI } from "assemblyai";

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
  words?: AssemblyAiWord[] | null;
  error?: string | null;
};

export class AssemblyAiError extends Error {
  readonly code: "not_configured" | "provider_unavailable" | "provider_rejected" | "cancelled";

  constructor(
    code: AssemblyAiError["code"],
    message: string,
  ) {
    super(message);
    this.name = "AssemblyAiError";
    this.code = code;
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
  const apiKey = process.env.ASSEMBLYAI_API_KEY;
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
  return new AssemblyAiError(
    "provider_unavailable",
    "AssemblyAI transcription is temporarily unavailable.",
  );
}

export async function transcribeRecordedAudio(params: {
  audio: Buffer;
  language?: string;
  signal?: AbortSignal;
}): Promise<AssemblyAiTranscript> {
  const client = getAssemblyAiClient();
  let providerTranscriptId: string | null = null;

  try {
    throwIfAborted(params.signal);
    const submitted = await client.transcripts.submit({
      audio: params.audio,
      speech_models: ["universal-3-5-pro"],
      language_code: params.language?.split("-")[0] || "en",
      punctuate: true,
      format_text: true,
      redact_pii: true,
      redact_pii_sub: "entity_name",
      redact_pii_return_unredacted: false,
    });
    providerTranscriptId = submitted.id;

    const deadline = Date.now() + 45_000;
    while (Date.now() < deadline) {
      throwIfAborted(params.signal);
      const transcript = await client.transcripts.get(submitted.id) as AssemblyAiTranscript;
      if (transcript.status === "completed") return transcript;
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
      } catch {
        // Provider transcript deletion is best effort. The response never logs
        // or returns the provider payload, and no raw audio is stored locally.
      }
    }
  }
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