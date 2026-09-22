import express, { type Express } from 'express';
import cors from 'cors';
import pinoHttp from 'pino-http';
import router from './routes';
import { logger } from './lib/logger';
import { runtimeEnvironment } from './lib/environment';

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

const allowedOrigins = new Set(
  [runtimeEnvironment.canonicalOrigin, ...(process.env.ASKOLO_ALLOWED_ORIGINS || '').split(',')]
    .map((value) => value?.trim())
    .filter(Boolean),
);

app.use(
  cors({
    credentials: true,
    origin: (requestOrigin, callback) => {
      if (!requestOrigin || runtimeEnvironment.name === 'development' || allowedOrigins.has(requestOrigin)) {
        callback(null, true);
        return;
      }
      callback(null, false);
    },
  }),
);
// Voice fallback uploads are sent as base64 JSON. Keep the global parser
// bounded while allowing the route to enforce its smaller decoded-audio cap.
app.use(express.json({ limit: '12mb' }));
app.use(express.urlencoded({ extended: true }));

app.use('/api', router);

export default app;
