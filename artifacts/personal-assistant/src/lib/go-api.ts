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
  logoutPasswordSession as logoutPasswordSessionRequest,
  passwordLogin as passwordLoginRequest,
  passwordSignup as passwordSignupRequest,
  regenerateMFARecoveryCodes as regenerateMFARecoveryCodesRequest,
  requestMFARecoverySupport as requestMFARecoverySupportRequest,
  requestPasswordRecovery as requestPasswordRecoveryRequest,
  resendEmailVerification as resendEmailVerificationRequest,
  resetPassword as resetPasswordRequest,
  setPassword as setPasswordRequest,
  updateUserProfile as updateUserProfileRequest,
  verifyEmail as verifyEmailRequest,
  verifyMFA as verifyMFARequest,
  verifyMFARecoverySupport as verifyMFARecoverySupportRequest,
  verifyPasswordRecovery as verifyPasswordRecoveryRequest,
  verifyRecoveryEmail as verifyRecoveryEmailRequest,
} from '@workspace/api-client-react';
import type {
  CoachingInput,
  DisconnectGoogleScope,
  EmailInput,
  EmailVerificationInput,
  GeneratePlanInput,
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
  UserProfileUpdate,
} from '@workspace/api-client-react';

function sessionRequestOptions(signal?: AbortSignal) {
  return {
    credentials: 'include' as const,
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
  resendEmailVerification: (input: EmailInput) =>
    resendEmailVerificationRequest(input, sessionRequestOptions()),
  requestPasswordRecovery: (input: PasswordRecoveryRequestInput) =>
    requestPasswordRecoveryRequest(input, sessionRequestOptions()),
  verifyPasswordRecovery: (input: PasswordRecoveryVerificationInput) =>
    verifyPasswordRecoveryRequest(input, sessionRequestOptions()),
  resetPassword: (input: PasswordRecoveryResetInput) =>
    resetPasswordRequest(input, sessionRequestOptions()),
  requestMFARecoverySupport: (input: EmailInput) =>
    requestMFARecoverySupportRequest(input, sessionRequestOptions()),
  verifyMFARecoverySupport: (input: EmailVerificationInput) =>
    verifyMFARecoverySupportRequest(input, sessionRequestOptions()),
  enrollRecoveryEmail: (input: RecoveryEmailEnrollmentInput) =>
    enrollRecoveryEmailRequest(input, sessionRequestOptions()),
  verifyRecoveryEmail: (input: EmailVerificationInput) =>
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
  disconnectGoogle: (scope: DisconnectGoogleScope) =>
    disconnectGoogleRequest({ scope }, sessionRequestOptions()),
  deleteUserData: () => deleteUserDataRequest(sessionRequestOptions()),
  deleteUserAccount: () => deleteUserAccountRequest(sessionRequestOptions()),
  coaching: (input: CoachingInput, signal?: AbortSignal) =>
    coachingRequest(input, sessionRequestOptions(signal)),
  generatePlan: (input: GeneratePlanInput, signal?: AbortSignal) =>
    generatePlanRequest(input, sessionRequestOptions(signal)),
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