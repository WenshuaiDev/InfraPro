import { createContext, useContext, useEffect, useMemo, useRef, type ReactNode } from 'react';
import { Link } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, type ProjectContext, type PermissionCode } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { WorkspacePickerButton } from '../../app/workspace-picker-context';
import { Badge } from '@infrapro/ui/components/badge';
import { clearProject, projectContextKey, retryScopeRead } from './scope';

type Current = ProjectContext & { available: boolean; key: readonly unknown[]; can: (permission: PermissionCode) => boolean; allows: (operation: string) => boolean };
const CurrentProject = createContext<Current | null>(null);
export function ProjectBoundary({ tenant, project, children }: { tenant: string; project: string; children: ReactNode }) {
  const { account, client, authorization_revision, phase } = useIdentity(); const cache = useQueryClient();
  const state = useQuery({ queryKey: projectContextKey(account?.id, tenant, project), queryFn: ({ signal }) => client.projectContext(tenant, project, signal), refetchInterval: 30_000, staleTime: 0, retry: retryScopeRead, retryDelay: 0 });
  useEffect(() => { void cache.invalidateQueries({ queryKey: projectContextKey(account?.id, tenant, project) }); }, [authorization_revision, account?.id, tenant, project, cache]);
  useEffect(() => { const verify = () => { if (document.visibilityState === 'visible') void cache.refetchQueries({ queryKey: projectContextKey(account?.id, tenant, project) }); }; window.addEventListener('focus', verify); document.addEventListener('visibilitychange', verify); return () => { window.removeEventListener('focus', verify); document.removeEventListener('visibilitychange', verify); }; }, [account?.id, cache, tenant, project]);
  useEffect(() => client.onProjectRestriction((tid, pid) => { if (tid === tenant && pid === project) { clearProject(cache, account?.id, tenant, project); void cache.invalidateQueries({ queryKey: projectContextKey(account?.id, tenant, project) }); void cache.invalidateQueries({ queryKey: ['account', account?.id, 'my-projects'] }); } }), [account?.id, cache, client, tenant, project]);
  useEffect(() => () => { client.invalidateProjectScope(tenant, project); clearProject(cache, account?.id, tenant, project); }, [account?.id, cache, client, tenant, project]);
  const stamp = state.data ? [state.data.authorization.membership_id, state.data.authorization.membership_status, state.data.authorization.tenant_status, state.data.project.status, state.data.authorization.permissions.join(',')].join(':') : '';
  const previousStamp = useRef('');
  useEffect(() => { const old = previousStamp.current; previousStamp.current = stamp; if (old && old !== stamp) { const predicate = (q: {queryKey: readonly unknown[]}) => q.queryKey[0] === 'project' && q.queryKey[1] === account?.id && q.queryKey[2] === tenant && q.queryKey[4] === project && q.queryKey[6] === old; void cache.cancelQueries({predicate}); cache.removeQueries({predicate}); } }, [stamp, account?.id, cache, tenant, project]);
  const value = useMemo<Current | null>(() => state.data ? { ...state.data, available: !state.error && !state.isFetching && phase === 'authenticated', key: ['project', account?.id, tenant, state.data.authorization.tenant_membership_id, project, state.data.authorization.membership_id, stamp], can: (p) => state.data.authorization.permissions.includes(p), allows: (op) => state.data.authorization.allowed_operations.includes(op) } : null, [state.data, state.error, state.isFetching, phase, account?.id, tenant, project, stamp]);
  if (state.error instanceof ApiError && [403, 404].includes(state.error.status ?? 0)) return <section className="space-y-4"><h1 className="text-2xl font-semibold">当前项目不可访问</h1><p>项目或上层成员资格已变化，请重新选择项目。</p><WorkspacePickerButton filter="projects">重新选择项目</WorkspacePickerButton><Link to="/" className="underline">返回首页</Link></section>;
  if (!value) return state.error ? <ErrorState message={state.error.message} onRetry={() => void state.refetch()} /> : <p role="status">正在核验项目身份…</p>;
  return <CurrentProject value={value}><section className="mb-6 space-y-3 border-b pb-4"><div className="flex flex-wrap items-center gap-3"><p className="min-w-0 break-words text-lg font-semibold">{value.project.tenant_name} / {value.project.name}</p><Badge variant="outline">{value.project.status === 'active' ? '运行中' : '已归档'}</Badge><WorkspacePickerButton filter="projects">切换项目</WorkspacePickerButton></div>{value.project.status === 'archived' && <FormNotice>项目已归档：可按权限查看、撤权及交接管理；恢复后才能编辑或扩权。</FormNotice>}{state.error && <ErrorState title="服务暂时不可用，输入已保留" message={state.error.message} onRetry={() => void state.refetch()} />}</section><div key={stamp}>{children}</div></CurrentProject>;
}
// eslint-disable-next-line react-refresh/only-export-components
export function useProject() { const value = useContext(CurrentProject); if (!value) throw new Error('Project context required'); return value; }
// eslint-disable-next-line react-refresh/only-export-components
export function useOptionalProject() { return useContext(CurrentProject); }
