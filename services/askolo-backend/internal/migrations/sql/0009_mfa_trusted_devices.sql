CREATE TABLE "auth_trusted_devices" (
  "id" text PRIMARY KEY NOT NULL,
  "user_id" text NOT NULL,
  "credential_hash" text NOT NULL,
  "fingerprint_hash" text NOT NULL,
  "created_at" timestamp with time zone DEFAULT now() NOT NULL,
  "last_used_at" timestamp with time zone,
  "expires_at" timestamp with time zone NOT NULL,
  "revoked_at" timestamp with time zone,
  CONSTRAINT "auth_trusted_devices_credential_hash_unique" UNIQUE("credential_hash")
);
--> statement-breakpoint
CREATE INDEX "auth_trusted_devices_user_id_idx" ON "auth_trusted_devices" USING btree ("user_id");
--> statement-breakpoint
CREATE INDEX "auth_trusted_devices_active_expiry_idx" ON "auth_trusted_devices" USING btree ("user_id", "expires_at") WHERE "revoked_at" IS NULL;