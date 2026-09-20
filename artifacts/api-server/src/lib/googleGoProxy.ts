import type { NextFunction, Request, Response } from "express";
import { logger } from "./logger";

const proxyBaseURL =
  process.env.ASKOLO_GOOGLE_BACKEND_URL?.trim() ||
  (process.env.NODE_ENV === "development" ? "http://127.0.0.1:8090" : undefined);
const proxyTimeoutMs = 20_000;

const GOOGLE_PATHS = [
  /^\/auth\/google(?:\/callback)?$/,
  /^\/auth\/google\/link(?:\/callback)?$/,
  /^\/auth\/github(?:\/callback)?$/,
  /^\/auth\/github\/link(?:\/callback)?$/,
  /^\/integrations\/google(?:\/callback|\/status|\/accounts)?$/,
  /^\/integrations\/google\/[A-Za-z0-9._@-]+$/,
  /^\/integrations\/google\/calendar\/(?:calendars|events)(?:\/[A-Za-z0-9._@-]+)?$/,
  /^\/integrations\/google\/gmail\/send$/,
];

const AUTH_PATHS = [
  /^\/auth\/(?:user|session|logout)$/,
  /^\/auth\/password\/(?:login|set|signup)$/,
];

function isGoogleGoPath(path: string): boolean {
  return GOOGLE_PATHS.some((pattern) => pattern.test(path)) ||
    AUTH_PATHS.some((pattern) => pattern.test(path));
}

function relativeAPIPath(req: Request): string {
  const pathWithoutQuery = req.originalUrl.split("?")[0] ?? "";
  return pathWithoutQuery.startsWith("/api/")
    ? pathWithoutQuery.slice("/api".length)
    : pathWithoutQuery;
}

export async function googleGoProxy(req: Request, res: Response, next: NextFunction): Promise<void> {
  if (!proxyBaseURL || !isGoogleGoPath(relativeAPIPath(req))) {
    next();
    return;
  }

  let target: URL;
  try {
    target = new URL(`/api${relativeAPIPath(req)}`, proxyBaseURL);
    const queryStart = req.originalUrl.indexOf("?");
    if (queryStart >= 0) target.search = req.originalUrl.slice(queryStart);
  } catch {
    res.status(503).json({ code: "GOOGLE_PROXY_NOT_CONFIGURED", error: "Google service is unavailable." });
    return;
  }

  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), proxyTimeoutMs);
  try {
    const headers = new Headers();
    const cookie = req.headers.cookie;
    const contentType = req.headers["content-type"];
    const requestID = req.headers["x-request-id"];
    if (cookie) headers.set("cookie", cookie);
    if (typeof contentType === "string") headers.set("content-type", contentType);
    if (typeof requestID === "string") headers.set("x-request-id", requestID);
    const body = ["GET", "HEAD"].includes(req.method)
      ? undefined
      : JSON.stringify(req.body ?? {});

    const response = await fetch(target, {
      method: req.method,
      headers,
      body,
      redirect: "manual",
      signal: controller.signal,
    });

    const setCookie = (response.headers as Headers & { getSetCookie?: () => string[] }).getSetCookie?.();
    if (setCookie?.length) {
      res.setHeader("Set-Cookie", setCookie);
    } else {
      const singleCookie = response.headers.get("set-cookie");
      if (singleCookie) res.setHeader("Set-Cookie", singleCookie);
    }
    const location = response.headers.get("location");
    if (location) res.setHeader("Location", location);
    const responseType = response.headers.get("content-type");
    if (responseType) res.setHeader("Content-Type", responseType);
    res.status(response.status);
    if (response.status === 204) {
      res.end();
      return;
    }
    const payload = Buffer.from(await response.arrayBuffer());
    res.send(payload);
  } catch (error) {
    logger.warn({ err: error, route: relativeAPIPath(req) }, "Go Google proxy request failed");
    res.status(502).json({ code: "GOOGLE_PROXY_FAILED", error: "Google service is unavailable." });
  } finally {
    clearTimeout(timeout);
  }
}