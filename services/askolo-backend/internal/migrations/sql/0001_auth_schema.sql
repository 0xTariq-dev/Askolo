CREATE TABLE "auth_email_challenges" (
"id" text PRIMARY KEY NOT NULL,
"user_id" text,
"email" text NOT NULL,
"purpose" text NOT NULL,
"code_hash" text NOT NULL,
"expires_at" timestamp with time zone NOT NULL,
"consumed_at" timestamp with time zone,
"attempt_count" integer DEFAULT 0 NOT NULL,
"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "auth_passwords" (
"user_id" text PRIMARY KEY NOT NULL,
"password_hash" text NOT NULL,
"hash_version" text DEFAULT 'argon2id-v1' NOT NULL,
"created_at" timestamp with time zone DEFAULT now() NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "auth_recovery_codes" (
"id" text PRIMARY KEY NOT NULL,
"user_id" text NOT NULL,
"code_hash" text NOT NULL,
"used_at" timestamp with time zone,
"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "auth_recovery_methods" (
"id" text PRIMARY KEY NOT NULL,
"user_id" text NOT NULL,
"kind" text NOT NULL,
"address" text,
"secret_encrypted" text,
"verified_at" timestamp with time zone,
"revoked_at" timestamp with time zone,
"created_at" timestamp with time zone DEFAULT now() NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
CONSTRAINT "auth_recovery_methods_user_kind_address_unique" UNIQUE("user_id","kind","address")
);
--> statement-breakpoint
CREATE TABLE "auth_security_events" (
"id" text PRIMARY KEY NOT NULL,
"user_id" text,
"event_type" text NOT NULL,
"provider" text,
"request_id" text,
"metadata" jsonb DEFAULT '{}'::jsonb NOT NULL,
"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "auth_totp" (
"user_id" text PRIMARY KEY NOT NULL,
"secret_encrypted" text NOT NULL,
"enabled_at" timestamp with time zone,
"last_used_step" integer,
"created_at" timestamp with time zone DEFAULT now() NOT NULL,
"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
ALTER TABLE "users" ADD COLUMN "status" text DEFAULT 'active' NOT NULL;--> statement-breakpoint
ALTER TABLE "users" ADD COLUMN "email_verified_at" timestamp with time zone;--> statement-breakpoint
ALTER TABLE "users" ADD COLUMN "account_created_via" text;--> statement-breakpoint
ALTER TABLE "provider_accounts" ADD COLUMN "login_enabled" boolean DEFAULT false NOT NULL;--> statement-breakpoint
ALTER TABLE "provider_accounts" ADD COLUMN "email_verified" boolean DEFAULT false NOT NULL;--> statement-breakpoint
ALTER TABLE "provider_accounts" ADD COLUMN "linked_at" timestamp with time zone;--> statement-breakpoint
CREATE INDEX "auth_email_challenges_email_purpose_idx" ON "auth_email_challenges" USING btree ("email","purpose");--> statement-breakpoint
CREATE INDEX "auth_email_challenges_expires_at_idx" ON "auth_email_challenges" USING btree ("expires_at");--> statement-breakpoint
CREATE INDEX "auth_recovery_codes_user_id_idx" ON "auth_recovery_codes" USING btree ("user_id");--> statement-breakpoint
CREATE INDEX "auth_recovery_methods_user_id_idx" ON "auth_recovery_methods" USING btree ("user_id");--> statement-breakpoint
CREATE INDEX "auth_security_events_user_created_idx" ON "auth_security_events" USING btree ("user_id","created_at");--> statement-breakpoint
CREATE INDEX "auth_security_events_type_created_idx" ON "auth_security_events" USING btree ("event_type","created_at");