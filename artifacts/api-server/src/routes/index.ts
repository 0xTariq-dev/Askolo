import { Router, type IRouter } from 'express';
import { requireAuth } from '../middlewares/requireAuth';
import healthRouter from './health';
import authRouter from './auth';
import habitsRouter from './habits';
import goalsRouter from './goals';
import dailyPlansRouter from './daily_plans';
import eventsRouter from './events';
import choresRouter from './chores';
import notesRouter from './notes';
import actionItemsRouter from './action_items';
import dashboardRouter from './dashboard';
import aiRouter from './ai';
import googleRouter from './google';
import userRouter from './user';
import { googleGoProxy } from '../lib/googleGoProxy';

const router: IRouter = Router();

// Health is public — mount before requireAuth.
router.use(healthRouter);
// Go owns authentication and provider integrations. The TypeScript server only
// forwards those routes when the Go service is configured.
router.use(googleGoProxy);
// Session inspection/logout remain as a temporary compatibility adapter.
router.use(authRouter);

// All feature routes require a Go-owned native session + a local user row.
router.use(requireAuth);
router.use(habitsRouter);
router.use(goalsRouter);
router.use(dailyPlansRouter);
router.use(eventsRouter);
router.use(choresRouter);
router.use(notesRouter);
router.use(actionItemsRouter);
router.use(dashboardRouter);
router.use(aiRouter);
router.use(googleRouter);
router.use(userRouter);

export default router;
