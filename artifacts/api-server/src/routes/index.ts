import { Router, type IRouter } from "express";
import healthRouter from "./health";
import authRouter from "./auth";
import habitsRouter from "./habits";
import goalsRouter from "./goals";
import dailyPlansRouter from "./daily_plans";
import eventsRouter from "./events";
import choresRouter from "./chores";
import notesRouter from "./notes";
import actionItemsRouter from "./action_items";
import dashboardRouter from "./dashboard";

const router: IRouter = Router();

router.use(healthRouter);
router.use(authRouter);
router.use(habitsRouter);
router.use(goalsRouter);
router.use(dailyPlansRouter);
router.use(eventsRouter);
router.use(choresRouter);
router.use(notesRouter);
router.use(actionItemsRouter);
router.use(dashboardRouter);

export default router;
