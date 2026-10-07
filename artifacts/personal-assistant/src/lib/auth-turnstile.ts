export const TURNSTILE_SITE_KEY = '0x4AAAAAAFP6k2fNH2AoIpHx';

export type TurnstileAction =
  | 'signup'
  | 'login'
  | 'password_recovery'
  | 'email_resend'
  | 'mfa_recovery';

export type AuthFlowMode =
  | 'signin'
  | 'signup'
  | 'verify'
  | 'recovery-request'
  | 'recovery-method'
  | 'recovery-verify'
  | 'recovery-reset'
  | 'mfa'
  | 'mfa-recovery-request'
  | 'mfa-recovery-verify'
  | 'mfa-recovery-complete';

export function turnstileActionForMode(mode: AuthFlowMode): TurnstileAction | null {
  switch (mode) {
    case 'signup':
      return 'signup';
    case 'signin':
      return 'login';
    case 'verify':
      return 'email_resend';
    case 'recovery-request':
    case 'recovery-method':
    case 'recovery-verify':
      return 'password_recovery';
    case 'mfa-recovery-request':
    case 'mfa-recovery-verify':
      return 'mfa_recovery';
    default:
      return null;
  }
}
