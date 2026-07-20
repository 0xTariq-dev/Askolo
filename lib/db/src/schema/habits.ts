import { pgTable, serial, text, integer, timestamp, boolean } from "drizzle-orm/pg-core";
import { createInsertSchema } from "drizzle-zod";
import { z } from "zod/v4";

export const habitsTable = pgTable("habits", {
  id: serial("id").primaryKey(),
  userId: text("user_id").notNull(),
  name: text("name").notNull(),
  description: text("description"),
  frequency: text("frequency").notNull().default("daily"), // daily | weekly
  color: text("color"),
  icon: text("icon"),
  currentStreak: integer("current_streak").notNull().default(0),
  longestStreak: integer("longest_streak").notNull().default(0),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow().$onUpdate(() => new Date()),
});

export const habitCompletionsTable = pgTable("habit_completions", {
  id: serial("id").primaryKey(),
  habitId: integer("habit_id").notNull().references(() => habitsTable.id, { onDelete: "cascade" }),
  date: text("date").notNull(), // YYYY-MM-DD
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
});

export const insertHabitSchema = createInsertSchema(habitsTable).omit({ id: true, currentStreak: true, longestStreak: true, createdAt: true, updatedAt: true });
export const insertHabitCompletionSchema = createInsertSchema(habitCompletionsTable).omit({ id: true, createdAt: true });

export type InsertHabit = z.infer<typeof insertHabitSchema>;
export type Habit = typeof habitsTable.$inferSelect;
export type HabitCompletion = typeof habitCompletionsTable.$inferSelect;
