import { index, pgTable, text, timestamp, unique } from "drizzle-orm/pg-core";
import { createInsertSchema } from "drizzle-zod";
import { z } from "zod/v4";

export const providerConnectionsTable = pgTable(
  "provider_connections",
  {
    id: text("id").primaryKey(),
    userId: text("user_id").notNull(),
    providerAccountId: text("provider_account_id").notNull(),
    provider: text("provider").notNull(),
    status: text("status").notNull().default("active"),
    grantedScopes: text("granted_scopes").array().notNull().default([]),
    capabilities: text("capabilities").array().notNull().default([]),
    revokedAt: timestamp("revoked_at", { withTimezone: true }),
    createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
    updatedAt: timestamp("updated_at", { withTimezone: true })
      .notNull()
      .defaultNow()
      .$onUpdate(() => new Date()),
  },
  (table) => [
    unique("provider_connections_user_account_unique").on(table.userId, table.providerAccountId),
    index("provider_connections_user_id_idx").on(table.userId),
    index("provider_connections_account_id_idx").on(table.providerAccountId),
  ],
);

export const insertProviderConnectionSchema = createInsertSchema(providerConnectionsTable).omit({
  createdAt: true,
  updatedAt: true,
});
export type InsertProviderConnection = z.infer<typeof insertProviderConnectionSchema>;
export type ProviderConnection = typeof providerConnectionsTable.$inferSelect;