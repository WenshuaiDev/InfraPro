import type { components } from './schema';

export type Health = components['schemas']['Health'];

export class ApiError extends Error {
  constructor(message: string, public readonly status?: number) {
    super(message);
    this.name = 'ApiError';
  }
}

function isHealth(value: unknown): value is Health {
  if (typeof value !== 'object' || value === null) return false;
  return 'status' in value && value.status === 'ok'
    && 'service' in value && value.service === 'infrapro-api'
    && 'version' in value && typeof value.version === 'string' && value.version.length > 0
    && 'time' in value && typeof value.time === 'string' && Number.isFinite(Date.parse(value.time));
}

export function createApiClient({ baseUrl = '/api/v1', timeoutMs = 5000 } = {}) {
  const base = baseUrl.replace(/\/+$/, '');
  return {
    async getHealth(options: { signal?: AbortSignal } = {}): Promise<Health> {
      const timeout = AbortSignal.timeout(timeoutMs);
      const signal = options.signal ? AbortSignal.any([options.signal, timeout]) : timeout;
      let response: Response;
      try {
        response = await fetch(`${base}/health`, {
          signal, cache: 'no-store', headers: { Accept: 'application/json' },
        });
      } catch (error) {
        if (options.signal?.aborted) throw error;
        throw new ApiError(timeout.aborted ? '连接超时，请稍后重试。' : '无法连接服务，请确认 API 已启动后重试。');
      }
      if (!response.ok) throw new ApiError(`服务暂时不可用（HTTP ${response.status}），请稍后重试。`, response.status);
      let body: unknown;
      try { body = await response.json(); } catch { throw new ApiError('服务返回了无效的数据，请检查 API 地址。'); }
      if (!isHealth(body)) throw new ApiError('服务响应不符合健康检查契约，请检查 API 版本。');
      return body;
    },
  };
}

export type ApiClient = ReturnType<typeof createApiClient>;
