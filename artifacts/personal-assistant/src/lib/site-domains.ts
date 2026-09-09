export const PUBLIC_ORIGIN = 'https://askolo.app';
export const APP_ORIGIN = 'https://web.askolo.app';

const PUBLIC_HOSTS = new Set(['askolo.app', 'www.askolo.app']);

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
  return hostname.toLowerCase() === 'web.askolo.app';
}