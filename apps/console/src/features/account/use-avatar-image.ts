import { useCallback, useSyncExternalStore } from 'react';
import { useQuery } from '@tanstack/react-query';
import { avatarPath, type AvatarTarget } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';

export type AvatarMetadata = { has_avatar?: boolean; avatar_revision?: string };

// URLs are external browser resources. Share them while subscribed and revoke
// after the final consumer leaves, including StrictMode and account changes.
const urls = new WeakMap<Blob, { url: string; users: number }>();

export function useAvatarImage(target: AvatarTarget | undefined, metadata: AvatarMetadata | undefined) {
  const { account, client, authorization_revision, phase } = useIdentity();
  const path = target ? avatarPath(target) : '';
  const active = phase === 'authenticated' && !!account && !!target && metadata?.has_avatar === true;
  const query = useQuery({
    queryKey: ['avatar', account?.id, path, metadata?.avatar_revision, authorization_revision],
    queryFn: ({ signal }) => client.avatarContent(target!, metadata?.avatar_revision ?? '', signal),
    enabled: active, retry: false, staleTime: 30_000, gcTime: 0, refetchInterval: 30_000,
  });
  const data = active && !query.error ? query.data : undefined;
  const subscribe = useCallback(() => {
    if (!data) return () => {};
    const resource = urls.get(data) ?? { url: URL.createObjectURL(data), users: 0 };
    resource.users++;
    urls.set(data, resource);
    return () => { if (--resource.users === 0) { URL.revokeObjectURL(resource.url); urls.delete(data); } };
  }, [data]);
  const snapshot = useCallback(() => data ? urls.get(data)?.url : undefined, [data]);
  return useSyncExternalStore(subscribe, snapshot, () => undefined);
}
