import { clearProject, retryScopeRead } from '../projects/scope';
import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import { Link } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, type TenantContext } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
import { Button } from '@infrapro/ui/components/button';

const CurrentTenant = createContext<(TenantContext & { epoch: number; available: boolean }) | null>(null);
let nextEpoch = 0;
export function TenantBoundary({ id, children }: { id: string; children: ReactNode }) {
  const { client, account, authorization_revision } = useIdentity();
  const cache = useQueryClient();
  const [epoch, setEpoch] = useState(() => ++nextEpoch);
  const [checking, setChecking] = useState(false);
  const [restricted, setRestricted] = useState<'disabled' | 'lost' | null>(null);
  const context = useQuery({ queryKey: ['tenant', account?.id, id, 'context'], queryFn: ({ signal }) => client.tenantContext(id, signal), enabled: !restricted, staleTime: 0, retry: retryScopeRead, retryDelay: 0, refetchInterval: 30_000, refetchIntervalInBackground: false });
  const { refetch } = context;
  const previousMembership = useRef<string | undefined>(undefined);
  useEffect(() => {
    const next = context.data?.membership_id; const old = previousMembership.current;
    if (next && old && old !== next) {
      clearProject(cache, account?.id, id);
      for (const key of [['tenant', account?.id, id, old], ['project-governance', account?.id, id, old], ['project-context', account?.id, id]]) { void cache.cancelQueries({ queryKey: key }); cache.removeQueries({ queryKey: key }); }
    }
    if (next) previousMembership.current = next;
  }, [account?.id, cache, client, id, context.data?.membership_id]);
  useEffect(() => { void cache.invalidateQueries({ queryKey: ['tenant', account?.id, id, 'context'] }); void cache.invalidateQueries({ queryKey: ['account', account?.id, 'my-tenants'] }); }, [authorization_revision, cache, account?.id, id]);
  useEffect(() => {
    const verify = () => {
      if (document.visibilityState !== 'visible' || restricted === 'disabled') return;
      setChecking(true);
      // An ended relation keeps its data cleared, but a later explicit rejoin
      // must be discoverable. refetchQueries skips disabled queries; refetch
      // performs this single current-qualification read without enabling a loop.
      void refetch().finally(() => setChecking(false));
    };
    window.addEventListener('focus', verify); document.addEventListener('visibilitychange', verify);
    return () => { window.removeEventListener('focus', verify); document.removeEventListener('visibilitychange', verify); };
  }, [refetch, restricted]);
  useEffect(() => () => { client.invalidateTenantScope(id); clearProject(cache, account?.id, id); for (const key of [['tenant', account?.id, id], ['project-governance', account?.id, id], ['project-context', account?.id, id]]) { void cache.cancelQueries({ queryKey: key }); cache.removeQueries({ queryKey: key }); } }, [cache, client, account?.id, id]);
  useEffect(() => client.onTenantRestriction((tenant, code) => {
    if (tenant !== id) return;
    if (code === 'active') {
      setRestricted((current) => current === 'lost' ? null : current);
      return;
    }
    if (code === 'tenant_disabled' || code === 'tenant_not_found') {
      setRestricted(code === 'tenant_disabled' ? 'disabled' : 'lost');
      clearProject(cache, account?.id, id);
      for (const key of [['project-context',account?.id,id],['project-governance',account?.id,id]]) {void cache.cancelQueries({queryKey:key});cache.removeQueries({queryKey:key});}
      void cache.invalidateQueries({queryKey:['account',account?.id,'my-projects']});
      void cache.cancelQueries({ queryKey: ['tenant', account?.id, id] });
      cache.removeQueries({ queryKey: ['tenant', account?.id, id] });
    } else {
      setChecking(true);
      void cache.cancelQueries({ queryKey: ['project-governance', account?.id, id] });
      cache.removeQueries({ queryKey: ['project-governance', account?.id, id] });
      void cache.cancelQueries({ predicate: (q) => q.queryKey[0] === 'tenant' && q.queryKey[1] === account?.id && q.queryKey[2] === id && q.queryKey[3] !== 'context' });
      cache.removeQueries({ predicate: (q) => q.queryKey[0] === 'tenant' && q.queryKey[1] === account?.id && q.queryKey[2] === id && q.queryKey[3] !== 'context' });
      void cache.refetchQueries({ queryKey: ['tenant', account?.id, id, 'context'] }).finally(() => setChecking(false));
    }
  }), [client, cache, account?.id, id]);
  const errorCode = context.error instanceof ApiError ? context.error.code : undefined;
  const disabled = restricted === 'disabled' || errorCode === 'tenant_disabled';
  const lost = restricted === 'lost' || (!disabled && context.error instanceof ApiError && [403, 404].includes(context.error.status ?? 0));
  if (disabled) return <DisabledTenant id={id} onActive={() => { setEpoch(++nextEpoch); setChecking(true); setRestricted(null); void context.refetch().finally(() => setChecking(false)); }} />;
  if (lost) return <section className="space-y-4"><h1 className="text-2xl font-semibold">当前租户不可访问</h1><p>成员关系或权限已变化，请返回我的租户重新选择。</p><div className="flex flex-wrap items-center gap-3"><Link to="/tenants" className="underline">返回我的租户</Link><Button variant="outline" disabled={context.isFetching} onClick={() => void refetch()}>重新检查租户资格</Button></div></section>;
  if (context.error && !context.data) return <ErrorState title="服务暂时不可用" message={context.error.message} onRetry={() => void context.refetch()} />;
  if (!context.data) return <p role="status">正在核验租户身份…</p>;
  const value = context.data;
  return <CurrentTenant value={{ ...value, epoch, available: !checking && !context.error }}>{context.error && <ErrorState title="服务暂时不可用，输入已保留" message={context.error.message} onRetry={() => void context.refetch()} />}<div key={`${epoch}:${value.membership_id}:${value.permissions.join(',')}`} hidden={checking}>{children}</div>{checking && <p role="status">正在核验租户身份…</p>}</CurrentTenant>;
}
function DisabledTenant({ id, onActive }: { id: string; onActive: () => void }) {
  const { client, account } = useIdentity();
  const state = useQuery({ queryKey: ['tenant-availability', account?.id, id], queryFn: ({ signal }) => client.tenantAvailability(id, signal), staleTime: 0, retry: false, refetchInterval: 30_000, refetchOnWindowFocus: 'always' });
  const { refetch } = state;
  const active = !state.isFetching && !state.error && state.data?.status === 'active';
  const resumed = useRef(false);
  useEffect(() => { if (active && !resumed.current) { resumed.current = true; onActive(); } }, [active, onActive]);
  useEffect(() => { const check = () => { if (document.visibilityState === 'visible') void refetch(); }; window.addEventListener('focus', check); document.addEventListener('visibilitychange', check); return () => { window.removeEventListener('focus', check); document.removeEventListener('visibilitychange', check); }; }, [refetch]);
  if (state.error instanceof ApiError && [403, 404].includes(state.error.status ?? 0)) return <section className="space-y-4"><h1 className="text-2xl font-semibold">当前租户不可访问</h1><p>成员关系已变化，请返回我的租户。</p><Link to="/tenants" className="underline">返回我的租户</Link></section>;
  return <section className="max-w-2xl space-y-4 rounded-xl border p-5"><h1 className="text-2xl font-semibold">租户已停用</h1>{state.data && <><p className="break-words">{state.data.name} · {state.data.code}</p><p className="whitespace-pre-wrap break-words">{state.data.status_notice}</p>{state.data.status_changed_at && <p className="text-sm text-muted-foreground">停用时间：{new Date(state.data.status_changed_at).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' })}</p>}</>}<p>请联系维护人员了解处理进展。其他租户和个人账号功能仍可使用。</p>{state.error && <ErrorState title="服务暂时不可用，无法重新确认状态" message={state.error.message} />}<div className="flex flex-wrap gap-3"><Button asChild variant="outline"><Link to="/tenants">返回我的租户</Link></Button><Button disabled={state.isFetching} onClick={() => void state.refetch()}>重新检查</Button></div></section>;
}
// eslint-disable-next-line react-refresh/only-export-components
export function useTenant() { const value = useContext(CurrentTenant); if (!value) throw new Error('Tenant context required'); return value; }
// eslint-disable-next-line react-refresh/only-export-components
export function useOptionalTenant() { return useContext(CurrentTenant); }
