import { pgTable, serial, text, boolean, timestamp } from "drizzle-orm/pg-core";
import { createInsertSchema } from "drizzle-zod";
import { z } from "zod/v4";

export const dailyPlansTable = pgTable("daily_plans", {
  id: serial("id").primaryKey(),
  userId: text("user_id").notNull(),
  date: text("date").notNull(), // YYYY-MM-DD
  title: text("title").notNull(),
  timeBlock: text("time_block"),
  priority: text("priority").notNull().default("medium"), // high | medium | low
  completed: boolean("completed").notNull().default(false),
  notes: text("notes"),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow().$onUpdate(() => new Date()),
});

export const insertDailyPlanSchema = createInsertSchema(dailyPlansTable).omit({ id: true, createdAt: true, updatedAt: true });
export type InsertDailyPlan = z.infer<typeof insertDailyPlanSchema>;
export type DailyPlan = typeof dailyPlansTable.$inferSelect;
