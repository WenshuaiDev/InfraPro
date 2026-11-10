import type { Authorization } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
export function AuthorizationView({ value }: { value: Authorization }) {
  return <section className="space-y-4" aria-label="有效权限及来源"><div><h3 className="font-medium">本人及平台权限 · 已绑定角色</h3><div className="mt-2 flex flex-wrap gap-2">{value.roles.map((r) => <Badge key={r.id} variant="outline">{r.name}{r.status === 'disabled' ? '（已停用，不贡献权限）' : ''}</Badge>)}{!value.roles.length && <p className="text-sm text-muted-foreground">无角色，无业务操作权限。</p>}</div></div><div><h3 className="font-medium">有效权限及来源（{value.permissions.length}）</h3><ul className="mt-2 space-y-2 text-sm">{value.permissions.map((p) => <li key={p} className="break-all"><code>{p}</code><span className="ml-2 text-muted-foreground">来自 {value.sources[p]?.map((id) => value.roles.find((r) => r.id === id)?.name ?? id).join('、')}</span></li>)}</ul></div></section>;
}
