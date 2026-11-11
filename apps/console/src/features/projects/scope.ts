import type { QueryClient } from '@tanstack/react-query';
export const projectContextKey = (account: string | undefined, tenant: string, project: string) => ['project-context', account, tenant, project] as const;
export function clearProject(cache: QueryClient, account: string | undefined, tenant: string, project?: string) {
  const predicate = (q: { queryKey: readonly unknown[] }) => q.queryKey[0] === 'project' && q.queryKey[1] === account && q.queryKey[2] === tenant && (!project || q.queryKey[4] === project);
  void cache.cancelQueries({ predicate }); cache.removeQueries({ predicate });
}
export function refreshProjects(cache: QueryClient, account: string | undefined, tenant: string, project?: string) {
  for (const key of [['account', account, 'my-projects'], ['tenant', account, tenant, 'projects'], ['project-governance', account, tenant]]) void cache.invalidateQueries({ queryKey: key });
  void cache.invalidateQueries({ queryKey: ['project-context', account, tenant, ...(project ? [project] : [])] });
  void cache.invalidateQueries({ predicate: (q) => q.queryKey[0] === 'project' && q.queryKey[1] === account && q.queryKey[2] === tenant && (!project || q.queryKey[4] === project) });
}

// Scope disposal cancels in-flight reads (including React development effect replay).
// Only repeat this safe read when it is still observed; never retry mutations or HTTP denials.
export const retryScopeRead = (attempt: number, error: Error) => attempt < 1 && error.name === 'AbortError';
