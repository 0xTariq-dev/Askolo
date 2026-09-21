export type AskoloEnvironment = 'development' | 'staging' | 'production';

const name = (import.meta.env.VITE_ASKOLO_ENVIRONMENT || 'development') as AskoloEnvironment;
if (!['development', 'staging', 'production'].includes(name)) {
  throw new Error('VITE_ASKOLO_ENVIRONMENT must be development, staging, or production.');
}

const configuredCanonicalOrigin = import.meta.env.VITE_CANONICAL_ORIGIN?.trim();
const canonicalOrigin = configuredCanonicalOrigin || (typeof window === 'undefined' ? undefined : window.location.origin);

if (name !== 'development' && !configuredCanonicalOrigin) {
  throw new Error('VITE_CANONICAL_ORIGIN is required outside development.');
}

export const runtimeEnvironment = {
  name,
  canonicalOrigin,
  publicOrigin: import.meta.env.VITE_PUBLIC_ORIGIN?.trim() || canonicalOrigin,
  appOrigin: import.meta.env.VITE_APP_ORIGIN?.trim() || canonicalOrigin,
  buildCommit: import.meta.env.VITE_COMMIT_SHA?.trim() || 'local',
  releaseTag: import.meta.env.VITE_RELEASE_TAG?.trim() || 'unreleased',
  isProduction: name === 'production',
} as const;

export function provenanceLabel(): string {
  const version = runtimeEnvironment.releaseTag !== 'unreleased'
    ? runtimeEnvironment.releaseTag
    : runtimeEnvironment.buildCommit.slice(0, 12);
  return `${runtimeEnvironment.name} · ${version}`;
}