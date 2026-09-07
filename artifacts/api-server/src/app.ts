import express, { type Express } from 'express';
import cors from 'cors';
import pinoHttp from 'pino-http';
import { clerkMiddleware } from '@clerk/express';
import { publishableKeyFromHost } from '@clerk/shared/keys';
import {
  CLERK_PROXY_PATH,
  clerkProxyMiddleware,
  getClerkProxyHost,
} from './middlewares/clerkProxyMiddleware';
import router from './routes';
import { logger } from './lib/logger';

const app: Express = express();

app.use(
  pinoHttp({
    logger,
    serializers: {
      req(req) {
        return {
          id: req.id,
          method: req.method,
          url: req.url?.split('?')[0],
        };
      },
      res(res) {
        return {
          statusCode: res.statusCode,
        };
      },
    },
  }),
);

// Clerk proxy must be mounted before body parsers (it streams raw bytes).
app.use(CLERK_PROXY_PATH, clerkProxyMiddleware());

app.use(cors({ credentials: true, origin: true }));
// Voice fallback uploads are sent as base64 JSON. Keep the global parser
// bounded while allowing the route to enforce its smaller decoded-audio cap.
app.use(express.json({ limit: '12mb' }));
app.use(express.urlencoded({ extended: true }));

// Resolve the publishable key from the incoming request host so the same
// server can serve multiple Clerk custom domains.
app.use(
  clerkMiddleware((req) => ({
    publishableKey: publishableKeyFromHost(
      getClerkProxyHost(req) ?? '',
      process.env.CLERK_PUBLISHABLE_KEY,
    ),
  })),
);

app.use('/api', router);

export default app;
