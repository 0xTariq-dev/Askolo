import { relations } from "drizzle-orm";
import { pgTable, timestamp, varchar } from "drizzle-orm/pg-core";
import { usersTable } from "./auth";

export const voiceRetentionValues = [
  "delete_immediately",
  "until_review",
  "keep_24_hours",
] as const;

export type VoiceRetention = (typeof voiceRetentionValues)[number];

export const voicePreferencesTable = pgTable("voice_preferences", {
  userId: varchar("user_id")
    .primaryKey()
    .references(() => usersTable.id, { onDelete: "cascade" }),
  retention: varchar("retention", { length: 32 })
    .$type<VoiceRetention>()
    .notNull()
    .default("delete_immediately"),
  updatedAt: timestamp("updated_at", { withTimezone: true })
    .notNull()
    .defaultNow(),
});

export const voicePreferencesRelations = relations(voicePreferencesTable, ({ one }) => ({
  user: one(usersTable, {
    fields: [voicePreferencesTable.userId],
    references: [usersTable.id],
  }),
}));

export type VoicePreferences = typeof voicePreferencesTable.$inferSelect;