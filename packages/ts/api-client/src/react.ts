import { useEffect, useState } from 'react';
import type { ApiClient, Health } from './index';

type HealthState =
  | { phase: 'loading'; health?: never; message?: never }
  | { phase: 'success'; health: Health; message?: never }
  | { phase: 'error'; health?: never; message: string };

// The core client stays framework independent; React consumers opt in to this adapter.
export function useHealth(client: ApiClient) {
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<HealthState>({ phase: 'loading' });

  useEffect(() => {
    const controller = new AbortController();
    client.getHealth({ signal: controller.signal }).then(
      (health) => { if (!controller.signal.aborted) setState({ phase: 'success', health }); },
      (error: unknown) => {
        if (!controller.signal.aborted) {
          setState({ phase: 'error', message: error instanceof Error ? error.message : '检查失败，请重试。' });
        }
      },
    );
    return () => controller.abort();
  }, [client, attempt]);

  function refresh() {
    setState({ phase: 'loading' });
    setAttempt((value) => value + 1);
  }

  return { ...state, refresh };
}
