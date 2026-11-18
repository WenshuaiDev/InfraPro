import type { components } from './schema';
import type { MonitoringRequestOptions } from './monitoring';
type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];
export type LoginResult = Schema<'LoginResult'>;
export type LoginChallenge = Schema<'LoginChallenge'>;
export type EnrollmentSetup = Schema<'EnrollmentSetup'>;
export type MFAStatus = Schema<'MFAStatus'>;
export type MFAResult = Schema<'MFAResult'>;
export type SecurityPolicy = Schema<'SecurityPolicy'>;
export type AccountMFA = Schema<'AccountMFA'>;
type Request = <T>(path: string, method?: string, body?: unknown, notifyUnauthorized?: boolean, signal?: AbortSignal, options?: MonitoringRequestOptions) => Promise<T>;
type Command = <T>(path: string, method: string, input: object) => Promise<T>;
export function mfaMethods(request: Request, command: Command) {
  // OTPs and bearer credentials only pass through request. They must never
  // enter command intent keys, persistent queries, analytics or replay stores.
  return {
    mfaChallenge: (signal?: AbortSignal) => request<LoginChallenge>('/auth/mfa/challenge', 'GET', undefined, false, signal),
    mfaVerify: (input: Schema<'MFAVerifyInput'>, signal?: AbortSignal) => request<Schema<'LoginAuthenticated'>>('/auth/mfa/verify', 'POST', input, false, signal),
    mfaLoginEnrollmentStart: (challenge_id: string, signal?: AbortSignal) => request<EnrollmentSetup>('/auth/mfa/enrollment/start', 'POST', { challenge_id }, false, signal),
    mfaLoginEnrollmentConfirm: (input: Schema<'MFALoginConfirmInput'>, signal?: AbortSignal) => request<Schema<'LoginAuthenticated'>>('/auth/mfa/enrollment/confirm', 'POST', input, false, signal),
    mfaRecoveryClaim: (input: Schema<'MFARecoveryClaimInput'>, signal?: AbortSignal) => request<LoginChallenge>('/auth/mfa/recovery/claim', 'POST', input, false, signal),
    mfaCancel: (challenge_id: string) => request<void>('/auth/mfa/cancel', 'POST', { challenge_id }, false),
    mfa: () => request<MFAStatus>('/account/mfa'),
    mfaEnrollmentStart: (input: Schema<'MFAEnrollmentStartInput'>) => request<EnrollmentSetup>('/account/mfa/enrollment/start', 'POST', input),
    mfaEnrollmentConfirm: (input: Schema<'MFAEnrollmentConfirmInput'>) => request<MFAResult>('/account/mfa/enrollment/confirm', 'POST', input),
    mfaEnrollmentCancel: (enrollment_id: string) => request<void>('/account/mfa/enrollment/cancel', 'POST', { enrollment_id }),
    mfaDisable: (input: Schema<'MFADisableInput'>) => command<MFAResult>('/account/mfa/disable', 'POST', input),
    mfaRegenerate: (input: Schema<'MFARegenerateInput'>) => command<MFAResult>('/account/mfa/recovery-codes/regenerate', 'POST', input),
    securityPolicy: () => request<SecurityPolicy>('/platform/security-policy'),
    securityPolicyImpact: () => request<Schema<'MFAImpact'>>('/platform/security-policy/mfa-impact'),
    updateSecurityPolicy: (input: Schema<'SecurityPolicyInput'>) => command<Schema<'SecurityPolicyResult'>>('/platform/security-policy', 'PUT', input),
    accountMFA: (id: string) => request<AccountMFA>(`/access/accounts/${encodeURIComponent(id)}/mfa`),
    resetAccountMFA: (id: string, input: Schema<'MFAResetInput'>) => command<MFAResult>(`/access/accounts/${encodeURIComponent(id)}/mfa-reset`, 'POST', input),
  };
}
