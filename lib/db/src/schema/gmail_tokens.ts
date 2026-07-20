import { pgTable, text, timestamp } from "drizzle-orm/pg-core";
import { createInsertSchema } from "drizzle-zod";
import { z } from "zod/v4";

export const gmailTokensTable = pgTable("gmail_tokens", {
  userId: text("user_id").primaryKey(),
  accessToken: text("access_token").notNull(),
  refreshToken: text("refresh_token").notNull(),
  expiresAt: timestamp("expires_at", { withTimezone: true }).notNull(),
  scope: text("scope").notNull(),
  updatedAt: timestamp("updated_at", { withTimezone: true })
    .notNull()
    .defaultNow()
    .$onUpdate(() => new Date()),
});

export const insertGmailTokenSchema = createInsertSchema(gmailTokensTable).omit({
  updatedAt: true,
});
export type InsertGmailToken = z.infer<typeof insertGmailTokenSchema>;
export type GmailToken = typeof gmailTokensTable.$inferSelect;
