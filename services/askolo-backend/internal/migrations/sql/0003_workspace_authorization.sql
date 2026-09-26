-- Workspace and authorization resources were present in the archived
-- application schema but were not part of the original executable migrations.
-- Keep this forward migration safe for verified databases where these tables
-- already exist.
CREATE TABLE IF NOT EXISTS "workspaces" (
  "id" text PRIMARY KEY NOT NULL,
  "name" text NOT NULL,
  "owner_user_id" text NOT NULL,
  "status" text DEFAULT 'active' NOT NULL,
  "is_personal" boolean DEFAULT false NOT NULL,
  "created_at" timestamp with time zone DEFAULT now() NOT NULL,
  "updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE INDEX IF NOT EXISTS "workspaces_owner_user_id_idx"
  ON "workspaces" ("owner_user_id");
--> statement-breakpoint
CREATE INDEX IF NOT EXISTS "workspaces_status_idx"
  ON "workspaces" ("status");
--> statement-breakpoint
CREATE UNIQUE INDEX IF NOT EXISTS "workspaces_personal_owner_unique"
  ON "workspaces" ("owner_user_id")
  WHERE "is_personal" = true;
--> statement-breakpoint
CREATE TABLE IF NOT EXISTS "workspace_memberships" (
  "id" text PRIMARY KEY NOT NULL,
  "workspace_id" text NOT NULL,
  "user_id" text NOT NULL,
  "status" text DEFAULT 'active' NOT NULL,
  "role" text DEFAULT 'member' NOT NULL,
  "permissions" text[] DEFAULT '{}'::text[] NOT NULL,
  "revoked_at" timestamp with time zone,
  "created_at" timestamp with time zone DEFAULT now() NOT NULL,
  "updated_at" timestamp with time zone DEFAULT now() NOT NULL,
  CONSTRAINT "workspace_memberships_workspace_user_unique"
    UNIQUE ("workspace_id", "user_id")
);
--> statement-breakpoint
CREATE INDEX IF NOT EXISTS "workspace_memberships_user_id_idx"
  ON "workspace_memberships" ("user_id");
--> statement-breakpoint
CREATE INDEX IF NOT EXISTS "workspace_memberships_workspace_id_idx"
  ON "workspace_memberships" ("workspace_id");
--> statement-breakpoint
CREATE INDEX IF NOT EXISTS "workspace_memberships_status_idx"
  ON "workspace_memberships" ("status");
--> statement-breakpoint
CREATE TABLE IF NOT EXISTS "authorization_resources" (
  "id" text PRIMARY KEY NOT NULL,
  "workspace_id" text NOT NULL,
  "resource_type" text NOT NULL,
  "resource_id" text NOT NULL,
  "owner_user_id" text NOT NULL,
  "status" text DEFAULT 'active' NOT NULL,
  "created_at" timestamp with time zone DEFAULT now() NOT NULL,
  "updated_at" timestamp with time zone DEFAULT now() NOT NULL,
  CONSTRAINT "authorization_resources_scope_unique"
    UNIQUE ("workspace_id", "resource_type", "resource_id")
);
--> statement-breakpoint
CREATE INDEX IF NOT EXISTS "authorization_resources_workspace_idx"
  ON "authorization_resources" ("workspace_id");
--> statement-breakpoint
CREATE INDEX IF NOT EXISTS "authorization_resources_owner_idx"
  ON "authorization_resources" ("owner_user_id");