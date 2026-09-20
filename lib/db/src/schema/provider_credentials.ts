import { pgTable, text, timestamp } from "drizzle-orm/pg-core";
import { createInsertSchema } from "drizzle-zod";
import { z } from "zod/v4";

export const providerCredentialsTable = pgTable("provider_credentials", {
  connectionId: text("connection_id").primaryKey(),
  accessTokenEncrypted: text("access_token_encrypted").notNull(),
  refreshTokenEncrypted: text("refresh_token_encrypted").notNull(),
  expiresAt: timestamp("expires_at", { withTimezone: true }).notNull(),
  tokenType: text("token_type").notNull().default("Bearer"),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp("updated_at", { withTimezone: true })
    .notNull()
    .defaultNow()
    .$onUpdate(() => new Date()),
});

export const insertProviderCredentialSchema = createInsertSchema(providerCredentialsTable).omit({
  createdAt: true,
  updatedAt: true,
});
export type InsertProviderCredential = z.infer<typeof insertProviderCredentialSchema>;
export type ProviderCredential = typeof providerCredentialsTable.$inferSelect;