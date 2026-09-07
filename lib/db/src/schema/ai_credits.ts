import { createInsertSchema } from "drizzle-zod";
import {
  index,
  integer,
  jsonb,
  pgTable,
  serial,
  text,
  timestamp,
  uniqueIndex,
  varchar,
} from "drizzle-orm/pg-core";
import { z } from "zod/v4";

import { usersTable } from "./auth";

/**
 * Credits are integer accounting units. Pricing and entitlements are
 * deliberately represented as data so a subscription grant can be attached
 * later without changing reservation or settlement semantics.
 */
export const aiCreditAccountsTable = pgTable("ai_credit_accounts", {
  userId: varchar("user_id")
    .primaryKey()
    .references(() => usersTable.id, { onDelete: "cascade" }),
  grantedCredits: integer("granted_credits").notNull().default(0),
  adjustmentCredits: integer("adjustment_credits").notNull().default(0),
  reservedCredits: integer("reserved_credits").notNull().default(0),
  spentCredits: integer("spent_credits").notNull().default(0),
  refundedCredits: integer("refunded_credits").notNull().default(0),
  updatedAt: timestamp("updated_at", { withTimezone: true })
    .notNull()
    .defaultNow()
    .$onUpdate(() => new Date()),
});

export const aiCreditGrantsTable = pgTable(
  "ai_credit_grants",
  {
    id: serial("id").primaryKey(),
    userId: varchar("user_id")
      .notNull()
      .references(() => usersTable.id, { onDelete: "cascade" }),
    sourceType: varchar("source_type", { length: 40 }).notNull(),
    amountCredits: integer("amount_credits").notNull(),
    entitlementKey: varchar("entitlement_key", { length: 160 }),
    idempotencyKey: varchar("idempotency_key", { length: 200 }).notNull(),
    metadata: jsonb("metadata").$type<Record<string, unknown>>().notNull().default({}),
    createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => [
    uniqueIndex("ai_credit_grants_idempotency_idx").on(table.userId, table.idempotencyKey),
    index("ai_credit_grants_user_idx").on(table.userId, table.createdAt),
  ],
);

export const aiCreditAdjustmentsTable = pgTable(
  "ai_credit_adjustments",
  {
    id: serial("id").primaryKey(),
    userId: varchar("user_id")
      .notNull()
      .references(() => usersTable.id, { onDelete: "cascade" }),
    amountCredits: integer("amount_credits").notNull(),
    reason: varchar("reason", { length: 160 }).notNull(),
    idempotencyKey: varchar("idempotency_key", { length: 200 }).notNull(),
    metadata: jsonb("metadata").$type<Record<string, unknown>>().notNull().default({}),
    createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => [
    uniqueIndex("ai_credit_adjustments_idempotency_idx").on(table.userId, table.idempotencyKey),
    index("ai_credit_adjustments_user_idx").on(table.userId, table.createdAt),
  ],
);

export const aiCreditReservationsTable = pgTable(
  "ai_credit_reservations",
  {
    id: text("id").primaryKey(),
    userId: varchar("user_id")
      .notNull()
      .references(() => usersTable.id, { onDelete: "cascade" }),
    operationType: varchar("operation_type", { length: 40 }).notNull(),
    provider: varchar("provider", { length: 40 }).notNull(),
    mode: varchar("mode", { length: 40 }).notNull(),
    model: varchar("model", { length: 120 }),
    status: varchar("status", { length: 24 }).notNull(),
    idempotencyKey: varchar("idempotency_key", { length: 200 }).notNull(),
    requestFingerprint: varchar("request_fingerprint", { length: 64 }),
    reservedCredits: integer("reserved_credits").notNull(),
    settledCredits: integer("settled_credits").notNull().default(0),
    refundedCredits: integer("refunded_credits").notNull().default(0),
    maxCredits: integer("max_credits"),
    unitRate: integer("unit_rate").notNull().default(1),
    durationSeconds: integer("duration_seconds"),
    connectedDurationMs: integer("connected_duration_ms"),
    expiresAt: timestamp("expires_at", { withTimezone: true }),
    startedAt: timestamp("started_at", { withTimezone: true }).notNull().defaultNow(),
    closedAt: timestamp("closed_at", { withTimezone: true }),
    providerTokenHash: text("provider_token_hash"),
    providerTokenUsedAt: timestamp("provider_token_used_at", { withTimezone: true }),
    metadata: jsonb("metadata").$type<Record<string, unknown>>().notNull().default({}),
    createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
    updatedAt: timestamp("updated_at", { withTimezone: true })
      .notNull()
      .defaultNow()
      .$onUpdate(() => new Date()),
  },
  (table) => [
    uniqueIndex("ai_credit_reservations_idempotency_idx").on(table.userId, table.idempotencyKey),
    index("ai_credit_reservations_user_status_idx").on(table.userId, table.status),
    index("ai_credit_reservations_expiry_idx").on(table.status, table.expiresAt),
  ],
);

export const aiCreditReservationEventsTable = pgTable(
  "ai_credit_reservation_events",
  {
    id: serial("id").primaryKey(),
    reservationId: text("reservation_id")
      .notNull()
      .references(() => aiCreditReservationsTable.id, { onDelete: "cascade" }),
    userId: varchar("user_id")
      .notNull()
      .references(() => usersTable.id, { onDelete: "cascade" }),
    eventType: varchar("event_type", { length: 24 }).notNull(),
    credits: integer("credits").notNull().default(0),
    durationMs: integer("duration_ms"),
    idempotencyKey: varchar("idempotency_key", { length: 240 }).notNull(),
    details: jsonb("details").$type<Record<string, unknown>>().notNull().default({}),
    createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => [
    uniqueIndex("ai_credit_reservation_events_idempotency_idx").on(table.reservationId, table.idempotencyKey),
    index("ai_credit_reservation_events_reservation_idx").on(table.reservationId, table.createdAt),
  ],
);

export const aiProviderUsageEvidenceTable = pgTable(
  "ai_provider_usage_evidence",
  {
    id: serial("id").primaryKey(),
    reservationId: text("reservation_id")
      .notNull()
      .references(() => aiCreditReservationsTable.id, { onDelete: "cascade" }),
    userId: varchar("user_id")
      .notNull()
      .references(() => usersTable.id, { onDelete: "cascade" }),
    provider: varchar("provider", { length: 40 }).notNull(),
    mode: varchar("mode", { length: 40 }).notNull(),
    region: varchar("region", { length: 80 }),
    model: varchar("model", { length: 120 }),
    durationMs: integer("duration_ms"),
    inputUnits: integer("input_units"),
    outputUnits: integer("output_units"),
    providerRequestId: varchar("provider_request_id", { length: 200 }),
    idempotencyKey: varchar("idempotency_key", { length: 240 }).notNull(),
    evidence: jsonb("evidence").$type<Record<string, unknown>>().notNull().default({}),
    recordedAt: timestamp("recorded_at", { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => [
    uniqueIndex("ai_provider_usage_evidence_idempotency_idx").on(table.reservationId, table.idempotencyKey),
    index("ai_provider_usage_evidence_reservation_idx").on(table.reservationId),
  ],
);

export const insertAiCreditGrantSchema = createInsertSchema(aiCreditGrantsTable).omit({
  id: true,
  createdAt: true,
});
export const insertAiCreditReservationSchema = createInsertSchema(aiCreditReservationsTable).omit({
  createdAt: true,
  updatedAt: true,
});

export type AiCreditAccount = typeof aiCreditAccountsTable.$inferSelect;
export type AiCreditGrant = typeof aiCreditGrantsTable.$inferSelect;
export type AiCreditAdjustment = typeof aiCreditAdjustmentsTable.$inferSelect;
export type AiCreditReservation = typeof aiCreditReservationsTable.$inferSelect;
export type AiCreditReservationEvent = typeof aiCreditReservationEventsTable.$inferSelect;
export type AiProviderUsageEvidence = typeof aiProviderUsageEvidenceTable.$inferSelect;
export type InsertAiCreditGrant = z.infer<typeof insertAiCreditGrantSchema>;
export type InsertAiCreditReservation = z.infer<typeof insertAiCreditReservationSchema>;