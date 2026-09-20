import { pgTable, text, timestamp, primaryKey } from "drizzle-orm/pg-core";
import { createInsertSchema } from "drizzle-zod";
import { z } from "zod/v4";

export const providerSyncStateTable = pgTable(
  "provider_sync_state",
  {
    connectionId: text("connection_id").notNull(),
    service: text("service").notNull(),
    cursor: text("cursor"),
    lastSyncAt: timestamp("last_sync_at", { withTimezone: true }),
    status: text("status").notNull().default("idle"),
    lastError: text("last_error"),
    updatedAt: timestamp("updated_at", { withTimezone: true })
      .notNull()
      .defaultNow()
      .$onUpdate(() => new Date()),
  },
  (table) => [primaryKey({ columns: [table.connectionId, table.service] })],
);

export const insertProviderSyncStateSchema = createInsertSchema(providerSyncStateTable);
export type InsertProviderSyncState = z.infer<typeof insertProviderSyncStateSchema>;
export type ProviderSyncState = typeof providerSyncStateTable.$inferSelect;