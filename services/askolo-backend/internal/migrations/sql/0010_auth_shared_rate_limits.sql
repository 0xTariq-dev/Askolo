CREATE TABLE "auth_mfa_recovery_rate_limits" (
  "bucket_hash" text PRIMARY KEY NOT NULL,
  "window_started_at" timestamp with time zone NOT NULL,
  "request_count" integer NOT NULL
);