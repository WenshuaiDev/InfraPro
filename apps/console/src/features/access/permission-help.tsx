import type { Permission, PermissionCode } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
import { Switch } from '@infrapro/ui/components/switch';
import { groupPermissions, tenantPermissionGroup, tenantPermissionSections } from './permission-groups';

const scopeNames: Record<string, string> = { self: '本人', platform: '平台', tenant: '当前租户', project: '当前项目' };
type PermissionSummary = Pick<Permission, 'code' | 'name' | 'scope' | 'risk' | 'lifecycle' | 'description'>;
type GroupedPermission = PermissionSummary & Pick<Permission, 'group' | 'tenant_custom_role_assignable'>;

export function RolePermissionGroups({ codes, catalog = [], scope, showCodes }: { codes: PermissionCode[]; catalog?: GroupedPermission[]; scope: 'platform' | 'tenant' | 'project'; showCodes: boolean }) {
  const selected = new Set(codes);
  const permissions = catalog.filter((permission) => selected.has(permission.code));
  const groups = groupPermissions(permissions, scope === 'tenant' ? tenantPermissionGroup : undefined, scope === 'tenant' ? [...tenantPermissionSections.map((group) => group.title), '管理员保留能力'] : []);
  const missing = codes.filter((code) => !catalog.some((permission) => permission.code === code));
  return <div className="space-y-4" aria-label="按分组查看角色权限">
    {groups.map((group) => <section key={group.name} aria-label={`${group.name}权限`} className="rounded-xl border p-3">
      <div className="flex items-center justify-between gap-3 px-1 pb-2"><h4 className="text-sm font-semibold">{group.name}</h4><Badge variant="secondary" className="font-normal">{group.items.length} 项</Badge></div>
      <ul className="divide-y">{group.items.map((permission) => <li key={permission.code} className="min-w-0 space-y-2 px-1 py-3 text-sm"><PermissionInfo permission={permission} showCode={showCodes} />{scope === 'tenant' && !permission.tenant_custom_role_assignable && <Badge variant="outline">管理员保留能力</Badge>}</li>)}</ul>
    </section>)}
    {missing.length > 0 && <section aria-label="其他权限" className="rounded-xl border p-4"><h4 className="mb-3 text-sm font-semibold">其他权限（{missing.length}）</h4><p className="mb-3 text-sm text-muted-foreground">以下权限暂无可用说明。</p><ul className="space-y-2 text-sm">{missing.map((code) => <li key={code}><code className="break-all">{code}</code></li>)}</ul></section>}
  </div>;
}

export function PermissionInfo({ permission, showCode = false }: { permission: PermissionSummary; showCode?: boolean }) {
  return <span className="block min-w-0 space-y-1">
    <span className="flex flex-wrap items-center gap-2 font-medium">
      {permission.name}<Badge variant="outline">{scopeNames[permission.scope] ?? '未标明范围'}</Badge>
      {permission.risk === 'high' && <Badge variant="secondary">敏感操作</Badge>}
      {permission.lifecycle !== 'active' && <Badge variant="outline">已退役</Badge>}
    </span>
    <span className="block text-sm text-muted-foreground">{permission.description}</span>
    {showCode && <code className="block break-all text-xs text-muted-foreground">{permission.code}</code>}
  </span>;
}

export function PermissionCodeToggle({ checked, onChange }: { checked: boolean; onChange: (checked: boolean) => void }) {
  return <label className="flex min-h-11 w-fit cursor-pointer items-center gap-3 text-sm">
    <Switch checked={checked} onCheckedChange={onChange} />显示权限编码
  </label>;
}

export function RolePermissionHelp({ scope }: { scope: 'platform' | 'tenant' }) {
  return <details className="rounded-lg border bg-muted/30 px-3 text-sm">
    <summary className="min-h-11 cursor-pointer py-3 font-medium">权限说明</summary>
    <div className="space-y-2 pb-3 text-muted-foreground">
      <p>{scope === 'platform'
        ? '角色将一组操作权限授予绑定的账号。平台角色可配置平台范围及本人账号权限；租户和项目职责在对应工作空间配置。'
        : '角色将当前租户的操作权限授予绑定成员，不能据此访问其他租户。管理员保留能力可查看说明，不能加入自定义角色。'}</p>
      <p>每项权限下方说明用途和作用范围。部分操作还需要配套查看权限，请留意配置提示；敏感操作应按实际职责授予。</p>
      <p>勾选后需保存配置才会生效。权限编码可按需显示，搜索时也可以直接输入编码。</p>
    </div>
  </details>;
}
