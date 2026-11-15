import type { components } from './generated';
import { reasons } from './reasons';
export { reasons } from './reasons';
export type { components } from './generated';
export type Meta = components['schemas']['Meta'];
export type Session = components['schemas']['Session'];
export type Bootstrap = components['schemas']['Bootstrap'];
export type Overview = components['schemas']['Overview'];
export type ComponentView = components['schemas']['ComponentView'];
export type ComponentDetail = components['schemas']['ComponentDetail'];
export type NodeView = components['schemas']['NodeView'];
export type NodeDetail = components['schemas']['NodeDetail'];
export type ReplicaView = components['schemas']['ReplicaView'];
export type Check = components['schemas']['Check'];
export type Transition = components['schemas']['Transition'];
export type Metric = components['schemas']['Metric'];
export type MetricSeries = components['schemas']['MetricSeries'];
export type Self = components['schemas']['Self'];
export type Coverage = components['schemas']['Coverage'];
export type Envelope<T> = { meta: Meta; data: T };
export type Page<T> = { items: T[]; total: number; next_cursor: string; snapshot_id: string; expires_at: string };
export function reason(code: string): { title: string; hint: string } {
  return Object.hasOwn(reasons, code) ? reasons[code as keyof typeof reasons] : { title: '不支持的状态', hint: '请核对诊断协议与客户端版本。' };
}
export class DiagnosticsError extends Error {
  constructor(readonly code: string, readonly status: number, readonly retryAfterMS = 0) { super(reason(code).title); this.name = 'DiagnosticsError'; }
}
export function accessFailure(error: unknown) { return error instanceof DiagnosticsError && (error.status === 401 || error.status === 403 || error.code === 'auth_configuration_unavailable'); }
const id = (value: string) => { if (!/^[a-z][a-z0-9_.-]{0,63}$/.test(value)) throw new DiagnosticsError('invalid_request', 400); return encodeURIComponent(value); };
export class DiagnosticsClient {
  private csrf = '';
  private active = 0;
  private waiting: Array<() => void> = [];
  private denied = new Set<(error: DiagnosticsError) => void>();
  constructor(private readonly fetcher: typeof fetch = (...args) => fetch(...args)) {}
  onDenied(listener: (error: DiagnosticsError) => void) { this.denied.add(listener); return () => { this.denied.delete(listener); }; }
  clear() { this.csrf = ''; }
  private async slot(signal?: AbortSignal) {
    if (signal?.aborted) throw new DOMException('Aborted', 'AbortError');
    if (this.active >= 3) {
      if (this.waiting.length >= 32) throw new DiagnosticsError('rate_limited', 429, 5000);
      await new Promise<void>((resolve, reject) => {
        const abort = () => { this.waiting = this.waiting.filter((item) => item !== resume); reject(new DOMException('Aborted', 'AbortError')); };
        const resume = () => { signal?.removeEventListener('abort', abort); resolve(); };
        this.waiting.push(resume); signal?.addEventListener('abort', abort, { once: true });
      });
    } else this.active++;
  }
  private async request<T>(path: string, options: { signal?: AbortSignal; method?: 'POST'; body?: unknown } = {}): Promise<Envelope<T>> {
    await this.slot(options.signal);
    try {
      const signal = options.signal ? AbortSignal.any([options.signal, AbortSignal.timeout(8000)]) : AbortSignal.timeout(8000);
      const response = await this.fetcher(`/api/diagnostics/v1${path}`, { method: options.method ?? 'GET', credentials: 'same-origin', cache: 'no-store', redirect: 'error', signal, headers: options.method ? { 'Content-Type': 'application/json', 'X-CSRF-Token': this.csrf } : {}, body: options.method ? JSON.stringify(options.body ?? {}) : undefined });
      if (response.status === 204) return { meta: {} as Meta, data: undefined as T };
      const payload = await response.json() as Envelope<T> & { error?: { code?: string; retry_after_ms?: number } };
      if (!response.ok) {
        const error = new DiagnosticsError(payload.error?.code ?? 'adapter_error', response.status, payload.error?.retry_after_ms ?? 0);
        if (accessFailure(error) && path !== '/session/login') { this.clear(); for (const listener of this.denied) listener(error); }
        throw error;
      }
      return payload;
    } finally { const next = this.waiting.shift(); if (next) next(); else this.active--; }
  }
  async bootstrap(signal?: AbortSignal) { const value = await this.request<Bootstrap>('/session/bootstrap', { signal }); this.csrf = value.data.csrf_token; return value; }
  async login(username: string, password: string) { const value = await this.request<Session>('/session/login', { method: 'POST', body: { username, password } }); this.csrf = value.data.csrf_token; return value; }
  async session(signal?: AbortSignal) { const value = await this.request<Session>('/session', { signal }); this.csrf = value.data.csrf_token; return value; }
  activity() { return this.request<{ idle_expires_at: string }>('/session/activity', { method: 'POST' }); }
  async logout() { try { await this.request<void>('/session/logout', { method: 'POST' }); } finally { this.clear(); } }
  get<T>(path: string, params: Record<string, string | number | undefined> = {}, signal?: AbortSignal) {
    if (!/^\/(overview|self|nodes|components|replicas|checks|transitions|metrics)(\/[a-z][a-z0-9_.-]{0,63})?$/.test(path)) throw new DiagnosticsError('invalid_request', 400);
    const query = new URLSearchParams(); for (const [key, value] of Object.entries(params)) if (value !== undefined && value !== '') query.set(key, String(value));
    return this.request<T>(`${path}${query.size ? `?${query}` : ''}`, { signal });
  }
  check(probeID: string, signal?: AbortSignal) { return this.get<Check>(`/checks/${id(probeID)}`, {}, signal); }
}
