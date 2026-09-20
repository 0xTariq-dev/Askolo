import { index, pgTable, text, timestamp, unique } from "drizzle-orm/pg-core";
import { createInsertSchema } from "drizzle-zod";
import { z } from "zod/v4";

export const providerAccountsTable = pgTable(
  "provider_accounts",
  {
    id: text("id").primaryKey(),
    userId: text("user_id").notNull(),
    provider: text("provider").notNull(),
    externalSubject: text("external_subject").notNull(),
    email: text("email"),
    displayName: text("display_name"),
    avatarUrl: text("avatar_url"),
    createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
    updatedAt: timestamp("updated_at", { withTimezone: true })
      .notNull()
      .defaultNow()
      .$onUpdate(() => new Date()),
  },
  (table) => [
    unique("provider_accounts_provider_subject_unique").on(table.provider, table.externalSubject),
    index("provider_accounts_user_id_idx").on(table.userId),
  ],
);

export const insertProviderAccountSchema = createInsertSchema(providerAccountsTable).omit({
  createdAt: true,
  updatedAt: true,
});
export type InsertProviderAccount = z.infer<typeof insertProviderAccountSchema>;
export type ProviderAccount = typeof providerAccountsTable.$inferSelect;