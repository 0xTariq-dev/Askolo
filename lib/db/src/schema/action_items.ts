import { pgTable, serial, text, integer, boolean, timestamp } from "drizzle-orm/pg-core";
import { createInsertSchema } from "drizzle-zod";
import { z } from "zod/v4";

export const actionItemsTable = pgTable("action_items", {
  id: serial("id").primaryKey(),
  userId: text("user_id").notNull(),
  title: text("title").notNull(),
  sourceType: text("source_type"), // meeting | manual
  sourceId: integer("source_id"),
  dueDate: text("due_date"), // YYYY-MM-DD
  completed: boolean("completed").notNull().default(false),
  completedAt: timestamp("completed_at", { withTimezone: true }),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow().$onUpdate(() => new Date()),
});

export const insertActionItemSchema = createInsertSchema(actionItemsTable).omit({ id: true, completed: true, completedAt: true, createdAt: true, updatedAt: true });
export type InsertActionItem = z.infer<typeof insertActionItemSchema>;
export type ActionItem = typeof actionItemsTable.$inferSelect;
