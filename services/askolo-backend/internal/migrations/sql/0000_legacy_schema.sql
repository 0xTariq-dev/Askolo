CREATE TABLE "sessions" (
"sid" varchar PRIMARY KEY NOT NULL,
"sess" jsonb NOT NULL,
"expire" timestamp NOT NULL
);
--> statement-breakpoint
CREATE TABLE "users" (
"id" varchar PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
"email" varchar,
"first_name" varchar,
"last_name" varchar,
"profile_image_url" varchar,
"created_at" timestamp with time zone DEFAULT now() NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
CONSTRAINT "users_email_unique" UNIQUE("email")
);
--> statement-breakpoint
CREATE TABLE "habit_completions" (
"id" serial PRIMARY KEY NOT NULL,
"habit_id" integer NOT NULL,
"date" text NOT NULL,
"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "habits" (
"id" serial PRIMARY KEY NOT NULL,
"user_id" text NOT NULL,
"name" text NOT NULL,
"description" text,
"frequency" text DEFAULT 'daily' NOT NULL,
"color" text,
"icon" text,
"current_streak" integer DEFAULT 0 NOT NULL,
"longest_streak" integer DEFAULT 0 NOT NULL,
"created_at" timestamp with time zone DEFAULT now() NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "goals" (
"id" serial PRIMARY KEY NOT NULL,
"user_id" text NOT NULL,
"title" text NOT NULL,
"description" text,
"target_date" text,
"status" text DEFAULT 'active' NOT NULL,
"progress" integer DEFAULT 0 NOT NULL,
"category" text,
"created_at" timestamp with time zone DEFAULT now() NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "daily_plans" (
"id" serial PRIMARY KEY NOT NULL,
"user_id" text NOT NULL,
"date" text NOT NULL,
"title" text NOT NULL,
"time_block" text,
"priority" text DEFAULT 'medium' NOT NULL,
"completed" boolean DEFAULT false NOT NULL,
"notes" text,
"created_at" timestamp with time zone DEFAULT now() NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "events" (
"id" serial PRIMARY KEY NOT NULL,
"user_id" text NOT NULL,
"title" text NOT NULL,
"description" text,
"start_date" text NOT NULL,
"start_time" text,
"end_date" text,
"end_time" text,
"all_day" boolean DEFAULT false NOT NULL,
"location" text,
"color" text,
"attendees" text,
"google_event_id" text,
"created_at" timestamp with time zone DEFAULT now() NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "chores" (
"id" serial PRIMARY KEY NOT NULL,
"user_id" text NOT NULL,
"title" text NOT NULL,
"description" text,
"assigned_to" text,
"frequency" text DEFAULT 'once' NOT NULL,
"due_date" text,
"completed" boolean DEFAULT false NOT NULL,
"completed_at" timestamp with time zone,
"created_at" timestamp with time zone DEFAULT now() NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "notes" (
"id" serial PRIMARY KEY NOT NULL,
"user_id" text NOT NULL,
"title" text NOT NULL,
"content" text DEFAULT '' NOT NULL,
"tags" text,
"created_at" timestamp with time zone DEFAULT now() NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "action_items" (
"id" serial PRIMARY KEY NOT NULL,
"user_id" text NOT NULL,
"title" text NOT NULL,
"source_type" text,
"source_id" integer,
"due_date" text,
"completed" boolean DEFAULT false NOT NULL,
"completed_at" timestamp with time zone,
"created_at" timestamp with time zone DEFAULT now() NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "conversations" (
"id" serial PRIMARY KEY NOT NULL,
"title" text NOT NULL,
"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "messages" (
"id" serial PRIMARY KEY NOT NULL,
"conversation_id" integer NOT NULL,
"role" text NOT NULL,
"content" text NOT NULL,
"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "google_connections" (
"user_id" text PRIMARY KEY NOT NULL,
"connected" boolean DEFAULT false NOT NULL,
"scopes" text[],
"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "gmail_tokens" (
"user_id" text PRIMARY KEY NOT NULL,
"access_token" text NOT NULL,
"refresh_token" text NOT NULL,
"expires_at" timestamp with time zone NOT NULL,
"scope" text NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "ai_credit_accounts" (
"user_id" varchar PRIMARY KEY NOT NULL,
"granted_credits" integer DEFAULT 0 NOT NULL,
"adjustment_credits" integer DEFAULT 0 NOT NULL,
"reserved_credits" integer DEFAULT 0 NOT NULL,
"spent_credits" integer DEFAULT 0 NOT NULL,
"refunded_credits" integer DEFAULT 0 NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "ai_credit_adjustments" (
"id" serial PRIMARY KEY NOT NULL,
"user_id" varchar NOT NULL,
"amount_credits" integer NOT NULL,
"reason" varchar(160) NOT NULL,
"idempotency_key" varchar(200) NOT NULL,
"metadata" jsonb DEFAULT '{}'::jsonb NOT NULL,
"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "ai_credit_grants" (
"id" serial PRIMARY KEY NOT NULL,
"user_id" varchar NOT NULL,
"source_type" varchar(40) NOT NULL,
"amount_credits" integer NOT NULL,
"entitlement_key" varchar(160),
"idempotency_key" varchar(200) NOT NULL,
"metadata" jsonb DEFAULT '{}'::jsonb NOT NULL,
"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "ai_credit_reservation_events" (
"id" serial PRIMARY KEY NOT NULL,
"reservation_id" text NOT NULL,
"user_id" varchar NOT NULL,
"event_type" varchar(24) NOT NULL,
"credits" integer DEFAULT 0 NOT NULL,
"duration_ms" integer,
"idempotency_key" varchar(240) NOT NULL,
"details" jsonb DEFAULT '{}'::jsonb NOT NULL,
"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "ai_credit_reservations" (
"id" text PRIMARY KEY NOT NULL,
"user_id" varchar NOT NULL,
"operation_type" varchar(40) NOT NULL,
"provider" varchar(40) NOT NULL,
"mode" varchar(40) NOT NULL,
"model" varchar(120),
"status" varchar(24) NOT NULL,
"idempotency_key" varchar(200) NOT NULL,
"request_fingerprint" varchar(64),
"reserved_credits" integer NOT NULL,
"settled_credits" integer DEFAULT 0 NOT NULL,
"refunded_credits" integer DEFAULT 0 NOT NULL,
"max_credits" integer,
"unit_rate" integer DEFAULT 1 NOT NULL,
"duration_seconds" integer,
"connected_duration_ms" integer,
"expires_at" timestamp with time zone,
"started_at" timestamp with time zone DEFAULT now() NOT NULL,
"closed_at" timestamp with time zone,
"provider_token_hash" text,
"provider_token_used_at" timestamp with time zone,
"metadata" jsonb DEFAULT '{}'::jsonb NOT NULL,
"created_at" timestamp with time zone DEFAULT now() NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "ai_provider_usage_evidence" (
"id" serial PRIMARY KEY NOT NULL,
"reservation_id" text NOT NULL,
"user_id" varchar NOT NULL,
"provider" varchar(40) NOT NULL,
"mode" varchar(40) NOT NULL,
"region" varchar(80),
"model" varchar(120),
"duration_ms" integer,
"input_units" integer,
"output_units" integer,
"provider_request_id" varchar(200),
"idempotency_key" varchar(240) NOT NULL,
"evidence" jsonb DEFAULT '{}'::jsonb NOT NULL,
"recorded_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "voice_preferences" (
"user_id" varchar PRIMARY KEY NOT NULL,
"retention" varchar(32) DEFAULT 'delete_immediately' NOT NULL,
"consent_at" timestamp with time zone,
"consent_version" varchar(32),
"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "provider_accounts" (
"id" text PRIMARY KEY NOT NULL,
"user_id" text NOT NULL,
"provider" text NOT NULL,
"external_subject" text NOT NULL,
"email" text,
"display_name" text,
"avatar_url" text,
"created_at" timestamp with time zone DEFAULT now() NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
CONSTRAINT "provider_accounts_provider_subject_unique" UNIQUE("provider","external_subject")
);
--> statement-breakpoint
CREATE TABLE "provider_connections" (
"id" text PRIMARY KEY NOT NULL,
"user_id" text NOT NULL,
"provider_account_id" text NOT NULL,
"provider" text NOT NULL,
"status" text DEFAULT 'active' NOT NULL,
"granted_scopes" text[] DEFAULT '{}' NOT NULL,
"capabilities" text[] DEFAULT '{}' NOT NULL,
"revoked_at" timestamp with time zone,
"created_at" timestamp with time zone DEFAULT now() NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
CONSTRAINT "provider_connections_user_account_unique" UNIQUE("user_id","provider_account_id")
);
--> statement-breakpoint
CREATE TABLE "provider_credentials" (
"connection_id" text PRIMARY KEY NOT NULL,
"access_token_encrypted" text NOT NULL,
"refresh_token_encrypted" text NOT NULL,
"expires_at" timestamp with time zone NOT NULL,
"token_type" text DEFAULT 'Bearer' NOT NULL,
"created_at" timestamp with time zone DEFAULT now() NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "provider_defaults" (
"user_id" text NOT NULL,
"service" text NOT NULL,
"connection_id" text NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
CONSTRAINT "provider_defaults_user_id_service_pk" PRIMARY KEY("user_id","service")
);
--> statement-breakpoint
CREATE TABLE "provider_sync_state" (
"connection_id" text NOT NULL,
"service" text NOT NULL,
"cursor" text,
"last_sync_at" timestamp with time zone,
"status" text DEFAULT 'idle' NOT NULL,
"last_error" text,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
CONSTRAINT "provider_sync_state_connection_id_service_pk" PRIMARY KEY("connection_id","service")
);
--> statement-breakpoint
CREATE TABLE "google_oauth_states" (
"nonce_hash" text PRIMARY KEY NOT NULL,
"flow" text NOT NULL,
"user_id" text,
"return_to" text NOT NULL,
"expires_at" timestamp with time zone NOT NULL,
"consumed_at" timestamp with time zone
);
--> statement-breakpoint
ALTER TABLE "habit_completions" ADD CONSTRAINT "habit_completions_habit_id_habits_id_fk" FOREIGN KEY ("habit_id") REFERENCES "public"."habits"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "messages" ADD CONSTRAINT "messages_conversation_id_conversations_id_fk" FOREIGN KEY ("conversation_id") REFERENCES "public"."conversations"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "ai_credit_accounts" ADD CONSTRAINT "ai_credit_accounts_user_id_users_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."users"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "ai_credit_adjustments" ADD CONSTRAINT "ai_credit_adjustments_user_id_users_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."users"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "ai_credit_grants" ADD CONSTRAINT "ai_credit_grants_user_id_users_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."users"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "ai_credit_reservation_events" ADD CONSTRAINT "ai_credit_reservation_events_reservation_id_ai_credit_reservations_id_fk" FOREIGN KEY ("reservation_id") REFERENCES "public"."ai_credit_reservations"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "ai_credit_reservation_events" ADD CONSTRAINT "ai_credit_reservation_events_user_id_users_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."users"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "ai_credit_reservations" ADD CONSTRAINT "ai_credit_reservations_user_id_users_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."users"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "ai_provider_usage_evidence" ADD CONSTRAINT "ai_provider_usage_evidence_reservation_id_ai_credit_reservations_id_fk" FOREIGN KEY ("reservation_id") REFERENCES "public"."ai_credit_reservations"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "ai_provider_usage_evidence" ADD CONSTRAINT "ai_provider_usage_evidence_user_id_users_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."users"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "voice_preferences" ADD CONSTRAINT "voice_preferences_user_id_users_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."users"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE INDEX "IDX_session_expire" ON "sessions" USING btree ("expire");--> statement-breakpoint
CREATE UNIQUE INDEX "ai_credit_adjustments_idempotency_idx" ON "ai_credit_adjustments" USING btree ("user_id","idempotency_key");--> statement-breakpoint
CREATE INDEX "ai_credit_adjustments_user_idx" ON "ai_credit_adjustments" USING btree ("user_id","created_at");--> statement-breakpoint
CREATE UNIQUE INDEX "ai_credit_grants_idempotency_idx" ON "ai_credit_grants" USING btree ("user_id","idempotency_key");--> statement-breakpoint
CREATE INDEX "ai_credit_grants_user_idx" ON "ai_credit_grants" USING btree ("user_id","created_at");--> statement-breakpoint
CREATE UNIQUE INDEX "ai_credit_reservation_events_idempotency_idx" ON "ai_credit_reservation_events" USING btree ("reservation_id","idempotency_key");--> statement-breakpoint
CREATE INDEX "ai_credit_reservation_events_reservation_idx" ON "ai_credit_reservation_events" USING btree ("reservation_id","created_at");--> statement-breakpoint
CREATE UNIQUE INDEX "ai_credit_reservations_idempotency_idx" ON "ai_credit_reservations" USING btree ("user_id","idempotency_key");--> statement-breakpoint
CREATE INDEX "ai_credit_reservations_user_status_idx" ON "ai_credit_reservations" USING btree ("user_id","status");--> statement-breakpoint
CREATE INDEX "ai_credit_reservations_expiry_idx" ON "ai_credit_reservations" USING btree ("status","expires_at");--> statement-breakpoint
CREATE UNIQUE INDEX "ai_provider_usage_evidence_idempotency_idx" ON "ai_provider_usage_evidence" USING btree ("reservation_id","idempotency_key");--> statement-breakpoint
CREATE INDEX "ai_provider_usage_evidence_reservation_idx" ON "ai_provider_usage_evidence" USING btree ("reservation_id");--> statement-breakpoint
CREATE INDEX "provider_accounts_user_id_idx" ON "provider_accounts" USING btree ("user_id");--> statement-breakpoint
CREATE INDEX "provider_connections_user_id_idx" ON "provider_connections" USING btree ("user_id");--> statement-breakpoint
CREATE INDEX "provider_connections_account_id_idx" ON "provider_connections" USING btree ("provider_account_id");--> statement-breakpoint
CREATE INDEX "google_oauth_states_expires_at_idx" ON "google_oauth_states" USING btree ("expires_at");