import express, { type Express } from 'express';
import cors from 'cors';
import pinoHttp from 'pino-http';
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

app.use(cors({ credentials: true, origin: true }));
// Voice fallback uploads are sent as base64 JSON. Keep the global parser
// bounded while allowing the route to enforce its smaller decoded-audio cap.
app.use(express.json({ limit: '12mb' }));
app.use(express.urlencoded({ extended: true }));

app.use('/api', router);

export default app;
