import {
  ApiError,
  coaching as coachingRequest,
  confirmMFA as confirmMFARequest,
  deleteUserAccount as deleteUserAccountRequest,
  deleteUserData as deleteUserDataRequest,
  disconnectGoogleConnection as disconnectGoogleConnectionRequest,
  disconnectMicrosoftConnection as disconnectMicrosoftConnectionRequest,
  disableMFA as disableMFARequest,
  disconnectGoogle as disconnectGoogleRequest,
  enrollMFA as enrollMFARequest,
  enrollRecoveryEmail as enrollRecoveryEmailRequest,
  generatePlan as generatePlanRequest,
  getCurrentAuthUser as getCurrentAuthUserRequest,
  getMFAStatus as getMFAStatusRequest,
  getMicrosoftStatus as getMicrosoftStatusRequest,
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
  ProfileUser,
  FileAttachment as FileAttachmentSchema,
} from '@workspace/api-client-react';
import { getDeviceFingerprint } from './device-fingerprint';

const artifactBasePath = (import.meta.env?.BASE_URL ?? '/').replace(/\/$/, '');

export function ensureDeviceFingerprint(): void {
  // Keep the identifier browser-side; send it only in request headers, not API bodies or logs.
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

async function uploadForm<T>(path: string, form: FormData): Promise<T> {
  const fingerprint = getDeviceFingerprint();
  const response = await fetch(`${artifactBasePath}${path}`, {
    method: 'POST',
    credentials: 'include',
    headers: fingerprint ? { 'X-Askolo-Device-Fingerprint': fingerprint } : {},
    body: form,
  });
  if (!response.ok) throw new Error('The upload could not be completed.');
  return response.json() as Promise<T>;
}

async function attachmentRequest<T>(path: string, method = 'GET', form?: FormData): Promise<T> {
  const fingerprint = getDeviceFingerprint();
  const response = await fetch(`${artifactBasePath}${path}`, {
    method,
    credentials: 'include',
    headers: fingerprint ? { 'X-Askolo-Device-Fingerprint': fingerprint } : {},
    ...(form ? { body: form } : {}),
  });
  if (!response.ok) throw new Error('Attachment request failed.');
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export type FileAttachment = FileAttachmentSchema;

function attachmentForm(file: File): FormData {
  const form = new FormData();
  form.append('file', file);
  return form;
}

function appProfileImageURL(url: string | null): string | null {
  if (url?.startsWith('/')) return `${artifactBasePath}${url}`;
  return url;
}

export const goApi = {
  currentUser: async (signal?: AbortSignal) => {
    const response = await getCurrentAuthUserRequest(sessionRequestOptions(signal));
    return {
      ...response,
      user: response.user
        ? { ...response.user, profileImageUrl: appProfileImageURL(response.user.profileImageUrl) }
        : null,
    };
  },
  logout: () => logoutPasswordSessionRequest(sessionRequestOptions()),
  updateProfile: async (input: UserProfileUpdate) => {
    const user = await updateUserProfileRequest(input, sessionRequestOptions());
    return { ...user, profileImageUrl: appProfileImageURL(user.profileImageUrl) };
  },
  uploadProfileImage: async (file: File) => {
    const form = new FormData();
    form.append('file', file);
    const user = await uploadForm<ProfileUser>('/api/user/profile-image', form);
    return { ...user, profileImageUrl: appProfileImageURL(user.profileImageUrl) };
  },
  removeProfileImage: async () => {
    const user = await attachmentRequest<ProfileUser>('/api/user/profile-image', 'DELETE');
    return { ...user, profileImageUrl: appProfileImageURL(user.profileImageUrl) };
  },
  listNoteAttachments: (noteId: number) =>
    attachmentRequest<{ attachments: FileAttachment[] }>(`/api/notes/${noteId}/attachments`),
  uploadNoteAttachment: (noteId: number, file: File) =>
    attachmentRequest<FileAttachment>(`/api/notes/${noteId}/attachments`, 'POST', attachmentForm(file)),
  deleteNoteAttachment: (noteId: number, attachmentId: string) =>
    attachmentRequest<void>(`/api/notes/${noteId}/attachments/${encodeURIComponent(attachmentId)}`, 'DELETE'),
  listTaskAttachments: (workspaceId: string, taskId: string) =>
    attachmentRequest<{ attachments: FileAttachment[] }>(`/api/v2/workspaces/${encodeURIComponent(workspaceId)}/tasks/${encodeURIComponent(taskId)}/attachments`),
  uploadTaskAttachment: (workspaceId: string, taskId: string, file: File) =>
    attachmentRequest<FileAttachment>(`/api/v2/workspaces/${encodeURIComponent(workspaceId)}/tasks/${encodeURIComponent(taskId)}/attachments`, 'POST', attachmentForm(file)),
  deleteTaskAttachment: (workspaceId: string, taskId: string, attachmentId: string) =>
    attachmentRequest<void>(`/api/v2/workspaces/${encodeURIComponent(workspaceId)}/tasks/${encodeURIComponent(taskId)}/attachments/${encodeURIComponent(attachmentId)}`, 'DELETE'),
  attachmentHref: (url: string) => url.startsWith('/') ? `${artifactBasePath}${url}` : url,
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
  disconnectGoogleConnection: (connectionID: string) =>
    disconnectGoogleConnectionRequest(connectionID, sessionRequestOptions()),
  microsoftStatus: (signal?: AbortSignal) =>
    getMicrosoftStatusRequest(sessionRequestOptions(signal)),
  disconnectMicrosoftConnection: (connectionID: string) =>
    disconnectMicrosoftConnectionRequest(connectionID, sessionRequestOptions()),
  deleteUserData: () => deleteUserDataRequest(sessionRequestOptions()),
  deleteUserAccount: () => deleteUserAccountRequest(sessionRequestOptions()),
  coaching: (input: CoachingInput, signal?: AbortSignal) =>
    coachingRequest(input, sessionRequestOptions(signal)),
  generatePlan: (input: GeneratePlanInput, signal?: AbortSignal) =>
    generatePlanRequest(input, sessionRequestOptions(signal)),
  synthesizeAssistantSpeech: (runId: string, idempotencyKey: string, signal?: AbortSignal) => {
    const options = sessionRequestOptions(signal);
    return synthesizeAssistantSpeechRequest(runId, {
      ...options,
      headers: { ...options.headers, 'Idempotency-Key': idempotencyKey },
      responseType: 'blob',
    });
  },
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