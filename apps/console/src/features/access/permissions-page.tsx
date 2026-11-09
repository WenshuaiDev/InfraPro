import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Badge } from '@infrapro/ui/components/badge';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
export function PermissionsPage() {
  const { client } = useIdentity(); const [search, setSearch] = useState('');
  const query = useQuery({ queryKey: ['access', 'permissions'], queryFn: ({ signal }) => client.permissions(signal) });
  const items = query.data?.filter((p) => `${p.name} ${p.description} ${p.code}`.toLowerCase().includes(search.toLowerCase())) ?? [];
  return <><PageHeader title="权限目录" description="系统能力的只读说明。权限由系统定义，通过角色分配给账号。" /><div className="mb-6 max-w-md space-y-2"><Label htmlFor="permission-search">搜索权限名称、说明或编码</Label><Input id="permission-search" value={search} onChange={(e) => setSearch(e.target.value)} /></div>{query.isPending && <LoadingState />}{query.error && <ErrorState message={query.error.message} onRetry={() => void query.refetch()} />}
    <div className="space-y-8">{[...new Set(items.map((p) => p.group))].map((group) => <section key={group} className="space-y-3"><h2 className="text-lg font-semibold">{group}</h2><div className="grid gap-3 lg:grid-cols-2">{items.filter((p) => p.group === group).map((p) => <article key={p.code} className="space-y-2 rounded-lg border p-4"><div className="flex flex-wrap items-center gap-2"><h3 className="font-medium">{p.name}</h3><Badge variant="outline">{p.scope === 'self' ? '本人' : '平台'}</Badge>{p.risk === 'high' && <Badge variant="secondary">敏感操作</Badge>}{p.lifecycle !== 'active' && <Badge variant="outline">已退役</Badge>}</div><p className="text-sm text-muted-foreground">{p.description}</p><code className="block break-all text-xs">{p.code}</code></article>)}</div></section>)}</div>{!query.isPending && !query.error && !items.length && <p>没有匹配的权限。</p>}</>;
}
