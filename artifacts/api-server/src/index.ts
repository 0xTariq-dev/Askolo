import app from "./app";
import { expireStaleReservations } from "./lib/ai-credit-ledger";
import { logger } from "./lib/logger";

const rawPort = process.env["PORT"];

if (!rawPort) {
  throw new Error(
    "PORT environment variable is required but was not provided.",
  );
}

const port = Number(rawPort);

if (Number.isNaN(port) || port <= 0) {
  throw new Error(`Invalid PORT value: "${rawPort}"`);
}

app.listen(port, (err) => {
  if (err) {
    logger.error({ err }, "Error listening on port");
    process.exit(1);
  }

  logger.info({ port }, "Server listening");

  const reconciliationTimer = setInterval(() => {
    expireStaleReservations()
      .then((settled) => {
        if (settled > 0) logger.info({ settled }, "Expired stale AI credit reservations");
      })
      .catch((reconciliationError: unknown) => {
        logger.error({ err: reconciliationError }, "AI credit reconciliation failed");
      });
  }, 30_000);
  reconciliationTimer.unref();
});
