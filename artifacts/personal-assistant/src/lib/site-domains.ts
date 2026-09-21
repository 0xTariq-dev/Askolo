import { runtimeEnvironment } from './runtime-environment';

export const PUBLIC_ORIGIN = runtimeEnvironment.publicOrigin || 'http://localhost';
export const APP_ORIGIN = runtimeEnvironment.appOrigin || PUBLIC_ORIGIN;
const PUBLIC_HOSTS = new Set([new URL(PUBLIC_ORIGIN).hostname, 'www.askolo.app']);

function withPath(origin: string, path: string): string {
  const normalizedPath = path.startsWith('/') ? path : `/${path}`;
  return `${origin}${normalizedPath}`;
}

export function toPublicUrl(path = '/'): string {
  return withPath(PUBLIC_ORIGIN, path);
}

export function toAppUrl(path = '/'): string {
  return withPath(APP_ORIGIN, path);
}

export function isPublicProductionHost(hostname = window.location.hostname): boolean {
  return PUBLIC_HOSTS.has(hostname.toLowerCase());
}

export function isAppProductionHost(hostname = window.location.hostname): boolean {
  return hostname.toLowerCase() === new URL(APP_ORIGIN).hostname;
}