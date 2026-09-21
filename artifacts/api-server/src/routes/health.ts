import { Router, type IRouter } from "express";
import { HealthCheckResponse } from "@workspace/api-zod";
import { runtimeEnvironment } from "../lib/environment";

const router: IRouter = Router();

router.get("/healthz", (_req, res) => {
  const data = HealthCheckResponse.parse({
    status: "ok",
    environment: runtimeEnvironment.name,
    commit: runtimeEnvironment.buildCommit,
    release: runtimeEnvironment.releaseTag,
  });
  res.setHeader("X-Askolo-Environment", runtimeEnvironment.name);
  res.setHeader("X-Askolo-Commit", runtimeEnvironment.buildCommit);
  res.setHeader("X-Askolo-Release", runtimeEnvironment.releaseTag);
  res.json(data);
});

export default router;
