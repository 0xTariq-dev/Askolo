import { pgTable, text, boolean, timestamp } from "drizzle-orm/pg-core";
import { createInsertSchema } from "drizzle-zod";
import { z } from "zod/v4";

export const googleConnectionsTable = pgTable("google_connections", {
  userId: text("user_id").primaryKey(),
  connected: boolean("connected").notNull().default(false),
  scopes: text("scopes").array(),
  updatedAt: timestamp("updated_at", { withTimezone: true })
    .notNull()
    .defaultNow()
    .$onUpdate(() => new Date()),
});

export const insertGoogleConnectionSchema = createInsertSchema(googleConnectionsTable).omit({
  updatedAt: true,
});
export type InsertGoogleConnection = z.infer<typeof insertGoogleConnectionSchema>;
export type GoogleConnection = typeof googleConnectionsTable.$inferSelect;
