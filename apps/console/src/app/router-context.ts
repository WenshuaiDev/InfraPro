import type { QueryClient } from '@tanstack/react-query';
import type { IdentityValue } from '@infrapro/api-client/react';

export type RouterContext = { identity: IdentityValue; queryClient: QueryClient };
