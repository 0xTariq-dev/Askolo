import { pgTable, text, timestamp, primaryKey } from "drizzle-orm/pg-core";
import { createInsertSchema } from "drizzle-zod";
import { z } from "zod/v4";

export const providerDefaultsTable = pgTable(
  "provider_defaults",
  {
    userId: text("user_id").notNull(),
    service: text("service").notNull(),
    connectionId: text("connection_id").notNull(),
    updatedAt: timestamp("updated_at", { withTimezone: true })
      .notNull()
      .defaultNow()
      .$onUpdate(() => new Date()),
  },
  (table) => [primaryKey({ columns: [table.userId, table.service] })],
);

export const insertProviderDefaultSchema = createInsertSchema(providerDefaultsTable);
export type InsertProviderDefault = z.infer<typeof insertProviderDefaultSchema>;
export type ProviderDefault = typeof providerDefaultsTable.$inferSelect;