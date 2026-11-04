import assert from 'node:assert/strict';
import test from 'node:test';
import { ensureDeviceFingerprint, getDeviceFingerprint } from './device-fingerprint';

const validFingerprint = '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef';

function withBrowser(
  initialCookies: Record<string, string>,
  callback: (jar: Map<string, string>, writes: string[]) => void,
): void {
  const originalDocument = Object.getOwnPropertyDescriptor(globalThis, 'document');
  const originalWindow = Object.getOwnPropertyDescriptor(globalThis, 'window');
  const jar = new Map(Object.entries(initialCookies));
  const writes: string[] = [];
  const document = {
    get cookie() {
      return Array.from(jar.entries(), ([name, value]) => `${name}=${value}`).join('; ');
    },
    set cookie(value: string) {
      writes.push(value);
      const [cookiePair, ...attributes] = value.split('; ');
      const separator = cookiePair.indexOf('=');
      const name = cookiePair.slice(0, separator);
      const cookieValue = cookiePair.slice(separator + 1);
      const maxAge = attributes.find((attribute) => attribute.startsWith('Max-Age='));
      if (maxAge === 'Max-Age=0') {
        jar.delete(name);
      } else {
        jar.set(name, cookieValue);
      }
    },
  };

  Object.defineProperty(globalThis, 'document', { configurable: true, value: document });
  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    value: { location: { protocol: 'https:' } },
  });
  try {
    callback(jar, writes);
  } finally {
    if (originalDocument) Object.defineProperty(globalThis, 'document', originalDocument);
    else Reflect.deleteProperty(globalThis, 'document');
    if (originalWindow) Object.defineProperty(globalThis, 'window', originalWindow);
    else Reflect.deleteProperty(globalThis, 'window');
  }
}

test('migrates the existing identifier once and deletes its legacy cookie', () => {
  withBrowser({ askolo_device_fingerprint: validFingerprint }, (jar, writes) => {
    assert.equal(getDeviceFingerprint(), validFingerprint);
    assert.equal(jar.get('askolo_device_fingerprint_v2'), validFingerprint);
    assert.equal(jar.has('askolo_device_fingerprint'), false);
    assert.ok(writes.some((cookie) => cookie.includes('Max-Age=2592000')));
    assert.ok(writes.some((cookie) => cookie.startsWith('askolo_device_fingerprint=; Max-Age=0')));

    const writeCountAfterMigration = writes.length;
    ensureDeviceFingerprint();
    assert.equal(writes.length, writeCountAfterMigration, 'existing cookie expiry must not slide forward');
  });
});

test('creates a secure 30-day identifier when no browser cookie exists', () => {
  withBrowser({}, (jar, writes) => {
    const fingerprint = getDeviceFingerprint();

    assert.match(fingerprint ?? '', /^[a-f0-9]{64}$/);
    assert.equal(jar.get('askolo_device_fingerprint_v2'), fingerprint);
    assert.equal(writes.length, 1);
    assert.match(writes[0], /Max-Age=2592000/);
    assert.match(writes[0], /Path=\/; SameSite=Lax; Secure$/);
  });
});

test('rotates an invalid or expired identifier without sliding trusted authorization', () => {
  withBrowser({}, (jar, writes) => {
    jar.set('askolo_device_fingerprint_v2', 'invalid');

    const fingerprint = getDeviceFingerprint();

    assert.match(fingerprint ?? '', /^[a-f0-9]{64}$/);
    assert.notEqual(fingerprint, 'invalid');
    assert.equal(writes.length, 1);
    assert.match(writes[0], /Max-Age=2592000/);
  });
});
