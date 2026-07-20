import { pgTable, serial, text, boolean, timestamp } from "drizzle-orm/pg-core";
import { createInsertSchema } from "drizzle-zod";
import { z } from "zod/v4";

export const choresTable = pgTable("chores", {
  id: serial("id").primaryKey(),
  userId: text("user_id").notNull(),
  title: text("title").notNull(),
  description: text("description"),
  assignedTo: text("assigned_to"),
  frequency: text("frequency").notNull().default("once"), // daily | weekly | monthly | once
  dueDate: text("due_date"), // YYYY-MM-DD
  completed: boolean("completed").notNull().default(false),
  completedAt: timestamp("completed_at", { withTimezone: true }),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow().$onUpdate(() => new Date()),
});

export const insertChoreSchema = createInsertSchema(choresTable).omit({ id: true, completed: true, completedAt: true, createdAt: true, updatedAt: true });
export type InsertChore = z.infer<typeof insertChoreSchema>;
export type Chore = typeof choresTable.$inferSelect;
