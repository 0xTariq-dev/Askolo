import { relations } from "drizzle-orm";
import { pgTable, timestamp, varchar } from "drizzle-orm/pg-core";
import { usersTable } from "./auth";

export const voicePreferencesTable = pgTable("voice_preferences", {
  userId: varchar("user_id")
    .primaryKey()
    .references(() => usersTable.id, { onDelete: "cascade" }),
  // Kept only so the consent migration remains additive and does not drop
  // existing retention preferences during the Publish schema diff.
  legacyRetention: varchar("retention", { length: 32 })
    .notNull()
    .default("delete_immediately"),
  consentAt: timestamp("consent_at", { withTimezone: true }),
  consentVersion: varchar("consent_version", { length: 32 }),
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