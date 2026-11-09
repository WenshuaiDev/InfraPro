// The bearer token is memory-only; consume URL fragments before any page is rendered.
const listeners = new Set<() => void>();
let credential = '';
function captureCredential() {
  const fragment = new URLSearchParams(window.location.hash.slice(1));
  const token = fragment.get('token');
  if (token !== null) {
    credential = token;
    window.history.replaceState(null, '', window.location.pathname + window.location.search);
    listeners.forEach((listener) => listener());
  }
}
captureCredential();
window.addEventListener('hashchange', captureCredential);
export function getCredential() { return credential; }
export function subscribeCredential(listener: () => void) { listeners.add(listener); return () => { listeners.delete(listener); }; }
export function forgetCredential() { credential = ''; listeners.forEach((listener) => listener()); }
