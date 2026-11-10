import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import { Link } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, type TenantContext } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { ErrorState } from '@infrapro/ui/patterns/feedback';

const CurrentTenant = createContext<(TenantContext & { epoch: number; available: boolean }) | null>(null);
let nextEpoch = 0;
export function TenantBoundary({ id, children }: { id: string; children: ReactNode }) {
  const { client, account, authorization_revision } = useIdentity();
  const cache = useQueryClient();
  const [epoch] = useState(() => ++nextEpoch);
  const [checking, setChecking] = useState(false);
  const prefix = ['tenant', account?.id, id];
  const context = useQuery({ queryKey: [...prefix, 'context'], queryFn: ({ signal }) => client.tenantContext(id, signal), staleTime: 0, refetchInterval: 30_000, refetchIntervalInBackground: false });
  useEffect(() => { void cache.invalidateQueries({ queryKey: ['tenant', account?.id, id, 'context'] }); void cache.invalidateQueries({ queryKey: ['account', account?.id, 'my-tenants'] }); }, [authorization_revision, cache, account?.id, id]);
  useEffect(() => {
    const verify = () => { if (document.visibilityState !== 'visible') return; setChecking(true); void cache.refetchQueries({ queryKey: ['tenant', account?.id, id, 'context'] }).finally(() => setChecking(false)); };
    window.addEventListener('focus', verify); document.addEventListener('visibilitychange', verify);
    return () => { window.removeEventListener('focus', verify); document.removeEventListener('visibilitychange', verify); void cache.cancelQueries({ queryKey: ['tenant', account?.id, id] }); cache.removeQueries({ queryKey: ['tenant', account?.id, id] }); };
  }, [cache, account?.id, id]);
  const lost = context.error instanceof ApiError && [403, 404].includes(context.error.status ?? 0);
  useEffect(() => { if (lost) { void cache.cancelQueries({ predicate: (q) => q.queryKey[0] === 'tenant' && q.queryKey[2] === id && q.queryKey[3] !== 'context' }); cache.removeQueries({ predicate: (q) => q.queryKey[0] === 'tenant' && q.queryKey[2] === id && q.queryKey[3] !== 'context' }); } }, [lost, id, cache]);
  if (lost) return <section className="space-y-4"><h1 className="text-2xl font-semibold">当前租户不可访问</h1><p>成员关系或权限已变化，请返回我的租户重新选择。</p><Link to="/tenants" className="underline">返回我的租户</Link></section>;
  if (context.error && !context.data) return <ErrorState title="服务暂时不可用" message={context.error.message} onRetry={() => void context.refetch()} />;
  if (!context.data) return <p role="status">正在核验租户身份…</p>;
  const value = context.data;
  return <CurrentTenant value={{ ...value, epoch, available: !checking && !context.error }}>{context.error && <ErrorState title="服务暂时不可用，输入已保留" message={context.error.message} onRetry={() => void context.refetch()} />}<div key={`${value.membership_id}:${value.permissions.join(',')}`} hidden={checking}>{children}</div>{checking && <p role="status">正在核验租户身份…</p>}</CurrentTenant>;
}
// eslint-disable-next-line react-refresh/only-export-components
export function useTenant() { const value = useContext(CurrentTenant); if (!value) throw new Error('Tenant context required'); return value; }

// eslint-disable-next-line react-refresh/only-export-components
export function useOptionalTenant() { return useContext(CurrentTenant); }
