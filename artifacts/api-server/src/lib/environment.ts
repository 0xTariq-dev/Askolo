export type AskoloEnvironment = 'development' | 'staging' | 'production';
export type ReleaseMode = 'development' | 'normal' | 'hotfix';

function required(name: string, value: string | undefined): string {
  const normalized = value?.trim();
  if (!normalized) throw new Error(`${name} is required for this environment.`);
  return normalized;
}

function origin(value: string, name: string, allowHttp: boolean): string {
  const normalized = required(name, value);
  let parsed: URL;
  try {
    parsed = new URL(normalized);
  } catch {
    throw new Error(`${name} must be an absolute URL.`);
  }
  if ((!allowHttp && parsed.protocol !== 'https:') || (allowHttp && !['http:', 'https:'].includes(parsed.protocol))) {
    throw new Error(`${name} must use HTTPS outside development.`);
  }
  if (parsed.pathname !== '/' || parsed.search || parsed.hash || parsed.username || parsed.password) {
    throw new Error(`${name} must contain only scheme and host.`);
  }
  return parsed.origin;
}

const explicitEnvironment = process.env.ASKOLO_ENVIRONMENT?.trim();
const inferredEnvironment = process.env.NODE_ENV === 'production' ? undefined : 'development';
const name = (explicitEnvironment || inferredEnvironment) as AskoloEnvironment | undefined;

if (!name || !['development', 'staging', 'production'].includes(name)) {
  throw new Error('ASKOLO_ENVIRONMENT must be development, staging, or production.');
}
if (process.env.NODE_ENV === 'production' && !explicitEnvironment) {
  throw new Error('ASKOLO_ENVIRONMENT is required in production.');
}

const isDevelopment = name === 'development';
const canonicalOrigin = isDevelopment
  ? process.env.ASKOLO_CANONICAL_ORIGIN?.trim() || undefined
  : origin(process.env.ASKOLO_CANONICAL_ORIGIN || '', 'ASKOLO_CANONICAL_ORIGIN', false);
const databaseIdentity = isDevelopment
  ? process.env.ASKOLO_DATABASE_ID?.trim() || 'development-database'
  : required('ASKOLO_DATABASE_ID', process.env.ASKOLO_DATABASE_ID);
const cookieNamespace = isDevelopment
  ? process.env.ASKOLO_COOKIE_NAMESPACE?.trim() || 'askolo_dev'
  : required('ASKOLO_COOKIE_NAMESPACE', process.env.ASKOLO_COOKIE_NAMESPACE);

if (!/^[a-z][a-z0-9_-]{1,31}$/.test(cookieNamespace)) {
  throw new Error('ASKOLO_COOKIE_NAMESPACE must be 2-32 lowercase characters.');
}

export const runtimeEnvironment = {
  name,
  canonicalOrigin,
  databaseIdentity,
  cookieNamespace,
  sessionCookieName: `${cookieNamespace}_sid`,
  buildCommit: process.env.ASKOLO_COMMIT_SHA?.trim() || 'local',
  releaseTag: process.env.ASKOLO_RELEASE_TAG?.trim() || 'unreleased',
  releaseMode: (process.env.ASKOLO_RELEASE_MODE?.trim() || (isDevelopment ? 'development' : 'normal')) as ReleaseMode,
  parentProductionTag: process.env.ASKOLO_PARENT_PRODUCTION_TAG?.trim() || undefined,
  isNonProduction: name !== 'production',
} as const;

if (!isDevelopment) {
  required('ASKOLO_COMMIT_SHA', process.env.ASKOLO_COMMIT_SHA);
  required('ASKOLO_RELEASE_TAG', process.env.ASKOLO_RELEASE_TAG);
  if (!['normal', 'hotfix'].includes(runtimeEnvironment.releaseMode)) {
    throw new Error('ASKOLO_RELEASE_MODE must be normal or hotfix outside development.');
  }
  if (runtimeEnvironment.releaseMode === 'hotfix') {
    required('ASKOLO_PARENT_PRODUCTION_TAG', runtimeEnvironment.parentProductionTag);
  }
  required('ASKOLO_INTERNAL_TOKEN', process.env.ASKOLO_INTERNAL_TOKEN);
}