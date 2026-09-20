import { pgTable, text, timestamp, index } from "drizzle-orm/pg-core";
import { createInsertSchema } from "drizzle-zod";
import { z } from "zod/v4";

export const googleOAuthStatesTable = pgTable(
  "google_oauth_states",
  {
    nonceHash: text("nonce_hash").primaryKey(),
    flow: text("flow").notNull(),
    userId: text("user_id"),
    returnTo: text("return_to").notNull(),
    expiresAt: timestamp("expires_at", { withTimezone: true }).notNull(),
    consumedAt: timestamp("consumed_at", { withTimezone: true }),
  },
  (table) => [index("google_oauth_states_expires_at_idx").on(table.expiresAt)],
);

export const insertGoogleOAuthStateSchema = createInsertSchema(googleOAuthStatesTable);
export type InsertGoogleOAuthState = z.infer<typeof insertGoogleOAuthStateSchema>;
export type GoogleOAuthState = typeof googleOAuthStatesTable.$inferSelect;