// The bearer token is memory-only; consume URL fragments before any page is rendered.
const listeners = new Set<() => void>();
type Purpose = 'invitation' | 'reset' | 'tenant';
const credentials: Record<Purpose, string> = { invitation: '', reset: '', tenant: '' };
const generations: Record<Purpose, number> = { invitation: 0, reset: 0, tenant: 0 };
let lastAccount: string | undefined;
export function observeCredentialIdentity(account: string | undefined) {
  if (lastAccount && lastAccount !== account) forgetCredential('tenant');
  lastAccount = account;
}
function captureCredential() {
  const fragment = new URLSearchParams(window.location.hash.slice(1));
  const token = fragment.get('token');
  if (token !== null) {
    const path = window.location.pathname;
    const purpose: Purpose = path.endsWith('/tenant-invite') ? 'tenant' : path.endsWith('/reset-password') ? 'reset' : 'invitation';
    credentials[purpose] = token;
    generations[purpose]++;
    window.history.replaceState(null, '', window.location.pathname + window.location.search);
    listeners.forEach((listener) => listener());
  }
}
captureCredential();
window.addEventListener('hashchange', captureCredential);
export function getCredential(purpose: Purpose = 'invitation') { return credentials[purpose]; }
export function getCredentialGeneration(purpose: Purpose) { return generations[purpose]; }
export function subscribeCredential(listener: () => void) { listeners.add(listener); return () => { listeners.delete(listener); }; }
export function forgetCredential(purpose: Purpose = 'invitation') { credentials[purpose] = ''; listeners.forEach((listener) => listener()); }
