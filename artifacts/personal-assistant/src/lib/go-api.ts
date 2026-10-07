import {
  ApiError,
  coaching as coachingRequest,
  confirmMFA as confirmMFARequest,
  deleteUserAccount as deleteUserAccountRequest,
  deleteUserData as deleteUserDataRequest,
  disableMFA as disableMFARequest,
  disconnectGoogle as disconnectGoogleRequest,
  enrollMFA as enrollMFARequest,
  enrollRecoveryEmail as enrollRecoveryEmailRequest,
  generatePlan as generatePlanRequest,
  getCurrentAuthUser as getCurrentAuthUserRequest,
  getMFAStatus as getMFAStatusRequest,
  listTrustedDevices as listTrustedDevicesRequest,
  logoutPasswordSession as logoutPasswordSessionRequest,
  passwordLogin as passwordLoginRequest,
  passwordSignup as passwordSignupRequest,
  regenerateMFARecoveryCodes as regenerateMFARecoveryCodesRequest,
  requestMFARecovery as requestMFARecoveryRequest,
  requestPasswordRecovery as requestPasswordRecoveryRequest,
  resendEmailVerification as resendEmailVerificationRequest,
  revokeTrustedDevice as revokeTrustedDeviceRequest,
  resetPassword as resetPasswordRequest,
  setPassword as setPasswordRequest,
  synthesizeAssistantSpeech as synthesizeAssistantSpeechRequest,
  updateUserProfile as updateUserProfileRequest,
  verifyEmail as verifyEmailRequest,
  verifyMFA as verifyMFARequest,
  verifyMFARecovery as verifyMFARecoveryRequest,
  verifyPasswordRecovery as verifyPasswordRecoveryRequest,
  verifyRecoveryEmail as verifyRecoveryEmailRequest,
} from '@workspace/api-client-react';
import type {
  CoachingInput,
  DisconnectGoogleScope,
  EmailInput,
  EmailVerificationResendInput,
  EmailVerificationInput,
  GeneratePlanInput,
  MFAFreshCodeInput,
  MFARecoveryRequestInput,
  MFARecoveryVerificationInput,
  MFARecoveryCodeManagementInput,
  MFACodeInput,
  MFAReauthenticationInput,
  PasswordLoginInput,
  PasswordRecoveryRequestInput,
  PasswordRecoveryResetInput,
  PasswordRecoveryVerificationInput,
  PasswordSetInput,
  PasswordSignupInput,
  RecoveryEmailEnrollmentInput,
  RecoveryEmailVerificationInput,
  UserProfileUpdate,
} from '@workspace/api-client-react';

const fingerprintCookieName = 'askolo_device_fingerprint';

function getCookie(name: string): string | null {
  if (typeof document === 'undefined') return null;
  const prefix = `${name}=`;
  const cookie = document.cookie
    .split('; ')
    .find((value) => value.startsWith(prefix));
  return cookie ? decodeURIComponent(cookie.slice(prefix.length)) : null;
}

function createDeviceFingerprint(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('');
}

/**
 * Keep the browser identifier in the browser only. It is deliberately sent
 * through the request header and cookie, never in an API body or a log.
 */
function getDeviceFingerprint(): string | null {
  if (typeof window === 'undefined' || typeof document === 'undefined') return null;
  const existing = getCookie(fingerprintCookieName);
  if (existing) return existing;

  const fingerprint = createDeviceFingerprint();
  const secure = window.location.protocol === 'https:' ? '; Secure' : '';
  document.cookie = `${fingerprintCookieName}=${encodeURIComponent(fingerprint)}; Max-Age=31536000; Path=/; SameSite=Lax${secure}`;
  return fingerprint;
}

export function ensureDeviceFingerprint(): void {
  getDeviceFingerprint();
}

function sessionRequestOptions(signal?: AbortSignal) {
  const fingerprint = getDeviceFingerprint();
  return {
    credentials: 'include' as const,
    ...(fingerprint ? { headers: { 'X-Askolo-Device-Fingerprint': fingerprint } } : {}),
    ...(signal ? { signal } : {}),
  };
}

export const goApi = {
  currentUser: (signal?: AbortSignal) =>
    getCurrentAuthUserRequest(sessionRequestOptions(signal)),
  logout: () => logoutPasswordSessionRequest(sessionRequestOptions()),
  updateProfile: (input: UserProfileUpdate) =>
    updateUserProfileRequest(input, sessionRequestOptions()),
  setPassword: (input: PasswordSetInput) =>
    setPasswordRequest(input, sessionRequestOptions()),
  passwordLogin: (input: PasswordLoginInput) =>
    passwordLoginRequest(input, sessionRequestOptions()),
  passwordSignup: (input: PasswordSignupInput) =>
    passwordSignupRequest(input, sessionRequestOptions()),
  verifyEmail: (input: EmailVerificationInput) =>
    verifyEmailRequest(input, sessionRequestOptions()),
  resendEmailVerification: (input: EmailVerificationResendInput) =>
    resendEmailVerificationRequest(input, sessionRequestOptions()),
  requestPasswordRecovery: (input: PasswordRecoveryRequestInput) =>
    requestPasswordRecoveryRequest(input, sessionRequestOptions()),
  verifyPasswordRecovery: (input: PasswordRecoveryVerificationInput) =>
    verifyPasswordRecoveryRequest(input, sessionRequestOptions()),
  resetPassword: (input: PasswordRecoveryResetInput) =>
    resetPasswordRequest(input, sessionRequestOptions()),
  requestMFARecovery: (input: MFARecoveryRequestInput) =>
    requestMFARecoveryRequest(input, sessionRequestOptions()),
  verifyMFARecovery: (input: MFARecoveryVerificationInput) =>
    verifyMFARecoveryRequest(input, sessionRequestOptions()),
  enrollRecoveryEmail: (input: RecoveryEmailEnrollmentInput & { totpCode?: string }) =>
    enrollRecoveryEmailRequest(input, sessionRequestOptions()),
  verifyRecoveryEmail: (input: RecoveryEmailVerificationInput) =>
    verifyRecoveryEmailRequest(input, sessionRequestOptions()),
  mfaStatus: (signal?: AbortSignal) =>
    getMFAStatusRequest(sessionRequestOptions(signal)),
  enrollMFA: (input: MFAReauthenticationInput) =>
    enrollMFARequest(input, sessionRequestOptions()),
  confirmMFA: (input: MFACodeInput) =>
    confirmMFARequest(input, sessionRequestOptions()),
  verifyMFA: (input: MFACodeInput) =>
    verifyMFARequest(input, sessionRequestOptions()),
  regenerateMFARecoveryCodes: (input: MFARecoveryCodeManagementInput) =>
    regenerateMFARecoveryCodesRequest(input, sessionRequestOptions()),
  disableMFA: (input: MFARecoveryCodeManagementInput) =>
    disableMFARequest(input, sessionRequestOptions()),
  listTrustedDevices: (signal?: AbortSignal) =>
    listTrustedDevicesRequest(sessionRequestOptions(signal)),
  revokeTrustedDevice: (deviceId: string, input: MFAFreshCodeInput) =>
    revokeTrustedDeviceRequest(deviceId, input, sessionRequestOptions()),
  disconnectGoogle: (scope: DisconnectGoogleScope) =>
    disconnectGoogleRequest({ scope }, sessionRequestOptions()),
  deleteUserData: () => deleteUserDataRequest(sessionRequestOptions()),
  deleteUserAccount: () => deleteUserAccountRequest(sessionRequestOptions()),
  coaching: (input: CoachingInput, signal?: AbortSignal) =>
    coachingRequest(input, sessionRequestOptions(signal)),
  generatePlan: (input: GeneratePlanInput, signal?: AbortSignal) =>
    generatePlanRequest(input, sessionRequestOptions(signal)),
  synthesizeAssistantSpeech: (runId: string, signal?: AbortSignal) =>
    synthesizeAssistantSpeechRequest(runId, {
      ...sessionRequestOptions(signal),
      responseType: 'blob',
    }),
};

export function getApiErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiError) {
    const data = error.data;
    if (
      data &&
      typeof data === 'object' &&
      'error' in data &&
      typeof data.error === 'string' &&
      data.error.trim()
    ) {
      return data.error;
    }
  }

  return error instanceof Error ? error.message : fallback;
}

export function getRetryAfterSeconds(error: unknown): number {
  if (!(error instanceof ApiError)) return 0;

  const value = Number.parseInt(error.headers.get('Retry-After') ?? '', 10);
  return Number.isFinite(value) && value > 0 ? value : 0;
}