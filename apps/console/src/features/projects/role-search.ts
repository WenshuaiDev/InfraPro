export type ProjectRoleSearch = { q?: string; status?: 'active' | 'disabled'; source?: 'custom' | 'preset'; limit?: number };
export function projectRoleSearch(value: Record<string, unknown>): ProjectRoleSearch {
  return {
    q: typeof value.q === 'string' ? Array.from(value.q).slice(0, 100).join('') || undefined : undefined,
    status: value.status === 'active' || value.status === 'disabled' ? value.status : undefined,
    source: value.source === 'custom' || value.source === 'preset' ? value.source : undefined,
    limit: [10, 20, 50].includes(Number(value.limit)) ? Number(value.limit) : undefined,
  };
}
