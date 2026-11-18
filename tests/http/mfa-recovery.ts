import { execFileSync } from 'node:child_process';
import { expect, type APIRequestContext } from '@playwright/test';
import { syntheticTOTP } from '../helpers/totp';

export async function recoverHTTPMFA(request: APIRequestContext, origin: string, username: string, password: string, pending = false) {
  // The maintenance command emits the grant into this private pipe only. Tests
  // never attach its result, secret setup or recovery-code response to artifacts.
  const grant = JSON.parse(execFileSync('python3', ['tests/http/cli.py', 'mfa-reset', username], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] })).url as string;
  const headers = { Origin: origin, 'X-CSRF-Token': (await (await request.get(origin + '/api/v1/auth/csrf')).json()).token as string };
  if (pending) await request.post(origin + '/api/v1/auth/logout', { headers });
  const login = await request.post(origin + '/api/v1/auth/login', { headers, data: { username, password } }); expect(login.status()).toBe(202);
  const challenge = await login.json(); expect(challenge.status).toBe('mfa_recovery_required');
  const claimed = await request.post(origin + '/api/v1/auth/mfa/recovery/claim', { headers, data: { challenge_id: challenge.challenge_id, recovery_token: new URLSearchParams(new URL(grant).hash.slice(1)).get('token') } }); expect(claimed.status()).toBe(202);
  const setupResponse = await request.post(origin + '/api/v1/auth/mfa/enrollment/start', { headers, data: { challenge_id: challenge.challenge_id } }); expect(setupResponse.status()).toBe(200);
  const setup = await setupResponse.json();
  const confirm = await request.post(origin + '/api/v1/auth/mfa/enrollment/confirm', { headers, data: { challenge_id: challenge.challenge_id, new_totp_code: syntheticTOTP(setup.secret, -1) } }); expect(confirm.status()).toBe(200); expect((await confirm.json()).status).toBe('authenticated');
}
