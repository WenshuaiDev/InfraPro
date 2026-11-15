import { readJSON, sleep, privateJSON } from './diagnostics-lab.mjs';
import { join } from 'node:path';

// Lab credentials stay in the protected account file and this memory-only jar.
export async function diagnosticClient(lab) {
  const account = readJSON(lab.accounts_file), jar = new Map(); let csrf = '';
  async function request(path, body) {
    const response = await fetch(lab.origin + '/api/diagnostics/v1' + path, { method: body === undefined ? 'GET' : 'POST', redirect: 'error', signal: AbortSignal.timeout(10000), headers: { Origin: lab.origin, Cookie: [...jar].map(([k,v]) => `${k}=${v}`).join('; '), ...(body === undefined ? {} : { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf }) }, body: body === undefined ? undefined : JSON.stringify(body) });
    for (const cookie of response.headers.getSetCookie()) { const pair = cookie.split(';')[0]; const eq = pair.indexOf('='); jar.set(pair.slice(0,eq), pair.slice(eq+1)); }
    const value = response.status === 204 ? null : await response.json();
    if (!response.ok) throw Error(`diagnostics HTTP ${response.status}: ${value?.error?.code ?? 'unavailable'}`);
    if (value?.data?.csrf_token) csrf = value.data.csrf_token;
    return value;
  }
  await request('/session/bootstrap'); await request('/session/login', { username: account.username, password: account.password });
  return { get: request, logout: () => request('/session/logout', {}) };
}
export async function eventually(fn, timeout = 45000) {
  const deadline = Date.now() + timeout; let last;
  while (Date.now() < deadline) { try { const value = await fn(); if (value) return value; } catch (error) { last = error; } await sleep(1000); }
  throw Error(`Diagnostics condition not reached${last ? ': ' + last.message : ''}`);
}
export function evidence(lab, name, value) {
  privateJSON(join(lab.directory, 'evidence', name + '.json'), { run_id: lab.run_id, recorded_at: new Date().toISOString(), ...value });
}
