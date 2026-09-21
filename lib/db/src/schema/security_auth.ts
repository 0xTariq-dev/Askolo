import { boolean, index, integer, jsonb, pgTable, text, timestamp, unique, uniqueIndex } from "drizzle-orm/pg-core";
import { sql } from "drizzle-orm";

export const authPasswordsTable = pgTable("auth_passwords", {
  userId: text("user_id").primaryKey(),
  passwordHash: text("password_hash").notNull(),
  hashVersion: text("hash_version").notNull().default("argon2id-v1"),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow(),
});

export const authEmailChallengesTable = pgTable(
  "auth_email_challenges",
  {
    id: text("id").primaryKey(),
    userId: text("user_id"),
    email: text("email").notNull(),
    purpose: text("purpose").notNull(),
    codeHash: text("code_hash").notNull(),
    expiresAt: timestamp("expires_at", { withTimezone: true }).notNull(),
    consumedAt: timestamp("consumed_at", { withTimezone: true }),
    attemptCount: integer("attempt_count").notNull().default(0),
    createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => [
    index("auth_email_challenges_email_purpose_idx").on(table.email, table.purpose),
    index("auth_email_challenges_expires_at_idx").on(table.expiresAt),
  ],
);

export const authRecoveryMethodsTable = pgTable(
  "auth_recovery_methods",
  {
    id: text("id").primaryKey(),
    userId: text("user_id").notNull(),
    kind: text("kind").notNull(),
    address: text("address"),
    secretEncrypted: text("secret_encrypted"),
    verifiedAt: timestamp("verified_at", { withTimezone: true }),
    revokedAt: timestamp("revoked_at", { withTimezone: true }),
    createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
    updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => [
    unique("auth_recovery_methods_user_kind_address_unique").on(
      table.userId,
      table.kind,
      table.address,
    ),
    index("auth_recovery_methods_user_id_idx").on(table.userId),
  ],
);

export const authTotpTable = pgTable("auth_totp", {
  userId: text("user_id").primaryKey(),
  secretEncrypted: text("secret_encrypted").notNull(),
  enabledAt: timestamp("enabled_at", { withTimezone: true }),
  lastUsedStep: integer("last_used_step"),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow(),
});

export const authRecoveryCodesTable = pgTable(
  "auth_recovery_codes",
  {
    id: text("id").primaryKey(),
    userId: text("user_id").notNull(),
    codeHash: text("code_hash").notNull(),
    usedAt: timestamp("used_at", { withTimezone: true }),
    createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => [index("auth_recovery_codes_user_id_idx").on(table.userId)],
);

export const authSecurityEventsTable = pgTable(
  "auth_security_events",
  {
    id: text("id").primaryKey(),
    userId: text("user_id"),
    eventType: text("event_type").notNull(),
    provider: text("provider"),
    requestId: text("request_id"),
    metadata: jsonb("metadata").notNull().default({}),
    createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => [
    index("auth_security_events_user_created_idx").on(table.userId, table.createdAt),
    index("auth_security_events_type_created_idx").on(table.eventType, table.createdAt),
  ],
);

export const workspacesTable = pgTable(
  "workspaces",
  {
    id: text("id").primaryKey(),
    name: text("name").notNull(),
    ownerUserId: text("owner_user_id").notNull(),
    status: text("status").notNull().default("active"),
    isPersonal: boolean("is_personal").notNull().default(false),
    createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
    updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => [
    index("workspaces_owner_user_id_idx").on(table.ownerUserId),
    index("workspaces_status_idx").on(table.status),
    uniqueIndex("workspaces_personal_owner_unique")
      .on(table.ownerUserId)
      .where(sql`${table.isPersonal} = true`),
  ],
);

export const workspaceMembershipsTable = pgTable(
  "workspace_memberships",
  {
    id: text("id").primaryKey(),
    workspaceId: text("workspace_id").notNull(),
    userId: text("user_id").notNull(),
    status: text("status").notNull().default("active"),
    role: text("role").notNull().default("member"),
    permissions: text("permissions").array().notNull().default([]),
    revokedAt: timestamp("revoked_at", { withTimezone: true }),
    createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
    updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => [
    unique("workspace_memberships_workspace_user_unique").on(table.workspaceId, table.userId),
    index("workspace_memberships_user_id_idx").on(table.userId),
    index("workspace_memberships_workspace_id_idx").on(table.workspaceId),
    index("workspace_memberships_status_idx").on(table.status),
  ],
);

export const authorizationResourcesTable = pgTable(
  "authorization_resources",
  {
    id: text("id").primaryKey(),
    workspaceId: text("workspace_id").notNull(),
    resourceType: text("resource_type").notNull(),
    resourceId: text("resource_id").notNull(),
    ownerUserId: text("owner_user_id").notNull(),
    status: text("status").notNull().default("active"),
    createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
    updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => [
    unique("authorization_resources_scope_unique").on(
      table.workspaceId,
      table.resourceType,
      table.resourceId,
    ),
    index("authorization_resources_workspace_idx").on(table.workspaceId),
    index("authorization_resources_owner_idx").on(table.ownerUserId),
  ],
);