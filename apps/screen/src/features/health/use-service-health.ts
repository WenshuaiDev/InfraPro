import { createApiClient } from '@infrapro/api-client';
import { useHealth } from '@infrapro/api-client/react';

const client = createApiClient({ baseUrl: import.meta.env.VITE_API_BASE_URL || '/api/v1' });

export function useServiceHealth() {
  return useHealth(client);
}
