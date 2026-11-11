import { useInfiniteQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';

export function useAccountRoleOptions() {
  const { client, account } = useIdentity();
  return useInfiniteQuery({ queryKey: ['access', account?.id, 'account-role-options'], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.roles({ cursor: pageParam, limit: 100 }, signal), getNextPageParam: (page) => page.next_cursor || undefined });
}
