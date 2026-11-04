const fingerprintCookieName = 'askolo_device_fingerprint_v2';
const legacyFingerprintCookieName = 'askolo_device_fingerprint';
const fingerprintMaxAgeSeconds = 30 * 24 * 60 * 60;

function getCookie(name: string): string | null {
  if (typeof document === 'undefined') return null;
  const prefix = `${name}=`;
  const cookie = document.cookie
    .split('; ')
    .find((value) => value.startsWith(prefix));
  if (!cookie) return null;

  try {
    return decodeURIComponent(cookie.slice(prefix.length));
  } catch {
    return null;
  }
}

function isValidFingerprint(value: string | null): value is string {
  return value !== null && /^[a-f0-9]{64}$/i.test(value);
}

function createDeviceFingerprint(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('');
}

function setFingerprintCookie(name: string, value: string, maxAgeSeconds: number): void {
  const secure = window.location.protocol === 'https:' ? '; Secure' : '';
  document.cookie = `${name}=${encodeURIComponent(value)}; Max-Age=${maxAgeSeconds}; Path=/; SameSite=Lax${secure}`;
}

function deleteLegacyFingerprintCookie(): void {
  if (!getCookie(legacyFingerprintCookieName)) return;
  const secure = window.location.protocol === 'https:' ? '; Secure' : '';
  document.cookie = `${legacyFingerprintCookieName}=; Max-Age=0; Path=/; SameSite=Lax${secure}`;
}

/**
 * This random browser identifier is not an authenticator. Keep it for at most
 * the trusted-device period, migrate old identifiers once without sliding
 * their expiry, and never refresh its lifetime while it is present.
 */
export function getDeviceFingerprint(): string | null {
  if (typeof window === 'undefined' || typeof document === 'undefined') return null;

  const existing = getCookie(fingerprintCookieName);
  if (isValidFingerprint(existing)) {
    deleteLegacyFingerprintCookie();
    return existing;
  }

  const legacy = getCookie(legacyFingerprintCookieName);
  const fingerprint = isValidFingerprint(legacy) ? legacy : createDeviceFingerprint();
  setFingerprintCookie(fingerprintCookieName, fingerprint, fingerprintMaxAgeSeconds);
  deleteLegacyFingerprintCookie();
  return fingerprint;
}

export function ensureDeviceFingerprint(): void {
  getDeviceFingerprint();
}
