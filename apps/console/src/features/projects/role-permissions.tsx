import { useId, useState } from 'react';
import { LockKeyhole, Search } from 'lucide-react';
import type { Permission, PermissionCode } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { Checkbox } from '@infrapro/ui/components/checkbox';
import { Input } from '@infrapro/ui/components/input';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { PermissionCodeToggle } from '../access/permission-help';
import { groupPermissions } from '../access/permission-groups';
import { projectPermissionGroup, projectPermissionSections } from './role-permission-model';

export function ProjectRolePermissionHelp() {
  return <details className="rounded-lg border bg-muted/20 px-4 text-sm"><summary className="min-h-11 cursor-pointer py-3 font-medium">权限与授权范围说明</summary><div className="space-y-2 pb-4 leading-6 text-muted-foreground"><p>项目角色将一组操作权限授予绑定成员，仅在当前项目生效。多个启用角色的权限合并计算。</p><p>设施、设备和监测数据还需要相应对象及数据范围授权；写入、导出等操作可能需要配套查看权限，系统不会自动补选。</p><p>管理员保留能力由固定项目管理员角色提供，不能加入自定义角色。创建或复制角色不会自动绑定成员。</p></div></details>;
}

export function ProjectRolePermissions({ catalog, codes, onChange, initialCodes = codes, archived = false, grantable }: {
  catalog: Permission[]; codes: PermissionCode[]; onChange?: (codes: PermissionCode[]) => void;
  initialCodes?: PermissionCode[]; archived?: boolean; grantable?: PermissionCode[];
}) {
  const [q, setQ] = useState(''); const [group, setGroup] = useState('全部');
  const [showCodes, setShowCodes] = useState(false); const [selectedOnly, setSelectedOnly] = useState(false);
  const id = useId(); const selected = new Set(codes); const editing = !!onChange;
  const pool = catalog.filter(item => editing || selected.has(item.code));
  const groups = groupPermissions(pool, projectPermissionGroup, projectPermissionSections.map(section => section.name));
  const matches = (item: Permission) => (!selectedOnly || selected.has(item.code)) && `${item.name} ${item.code} ${item.description}`.toLowerCase().includes(q.trim().toLowerCase());
  const eligible = (item: Permission) => item.project_custom_role_assignable && item.lifecycle === 'active' && (!archived || initialCodes.includes(item.code)) && (!grantable || grantable.includes(item.code));
  const visible = groups.filter(section => group === '全部' || section.name === group).map(section => ({ ...section, items: section.items.filter(matches) })).filter(section => section.items.length);
  const selectable = visible.flatMap(section => section.items).filter(eligible).map(item => item.code);
  const missing = codes.filter(code => !catalog.some(item => item.code === code));
  return <section className="space-y-4" aria-label={editing ? '选择项目角色权限' : '查看项目角色权限'}>
    <ProjectRolePermissionHelp />
    {editing && <p className="text-sm text-muted-foreground">已选 {codes.length} 项。{archived ? '项目已归档，仅可减少已有权限。' : '按实际职责勾选，保存后生效。'}</p>}
    <div className="flex flex-wrap items-center gap-2"><div className="relative min-w-40 flex-1"><Search aria-hidden="true" className="pointer-events-none absolute top-3 left-3 size-4 text-muted-foreground" /><Input className="pl-9" aria-label="搜索权限名称或编码" placeholder="搜索权限名称或编码…" value={q} onChange={event => setQ(event.target.value)} /></div>{editing && <Button type="button" variant={selectedOnly ? 'secondary' : 'outline'} aria-pressed={selectedOnly} onClick={() => setSelectedOnly(!selectedOnly)}>只看已选 · {codes.length}</Button>}</div>
    <div className="flex flex-wrap gap-1.5" aria-label="权限业务分组">{['全部', ...groups.map(section => section.name)].map(name => <Button type="button" key={name} variant={group === name ? 'secondary' : 'ghost'} size="sm" className="h-8 px-2.5 text-xs max-sm:min-h-11" aria-pressed={group === name} onClick={() => setGroup(name)}>{name}</Button>)}</div>
    <div className="flex flex-wrap items-center justify-between gap-2 border-b pb-2"><PermissionCodeToggle checked={showCodes} onChange={setShowCodes} />{editing && <div className="flex flex-wrap gap-1"><Button type="button" size="sm" variant="ghost" onClick={() => onChange?.([...new Set([...codes, ...selectable])])}>选中当前可配置项</Button><Button type="button" size="sm" variant="ghost" onClick={() => onChange?.(codes.filter(code => !visible.some(section => section.items.some(item => item.code === code))))}>清除当前选择</Button></div>}</div>
    {visible.map(section => <section key={section.name} className="overflow-hidden rounded-lg border" aria-label={`${section.name}权限`}><div className="flex items-center justify-between gap-3 bg-muted/30 px-4 py-2.5"><h4 className="text-sm font-medium">{section.name}</h4><span className="text-xs text-muted-foreground">{editing ? `${section.items.filter(item => selected.has(item.code)).length} / ` : ''}{section.items.length} 项</span></div><ul className="divide-y">{section.items.map(item => <li className="flex gap-3 px-4 py-3.5 text-sm" key={item.code}>{editing && <Checkbox id={`${id}-${item.code}`} className="mt-0.5" aria-label={item.name} checked={selected.has(item.code)} disabled={!eligible(item) && !selected.has(item.code)} onCheckedChange={checked => onChange?.(checked ? [...codes, item.code] : codes.filter(code => code !== item.code))} />}<div className="min-w-0 flex-1 space-y-1.5"><label htmlFor={editing ? `${id}-${item.code}` : undefined} className={`flex flex-wrap items-center gap-2 ${editing ? 'cursor-pointer' : ''}`}><span className="font-medium">{item.name}</span>{item.risk === 'high' && <Badge variant="secondary" className="font-normal">敏感操作</Badge>}{!item.project_custom_role_assignable && <Badge variant="outline" className="gap-1 font-normal"><LockKeyhole aria-hidden="true" className="size-3" />管理员保留</Badge>}{item.lifecycle !== 'active' && <Badge variant="outline">已退役</Badge>}</label><p className="text-xs leading-5 text-muted-foreground">{item.description}</p>{showCodes && <code className="block break-all text-xs text-muted-foreground">{item.code}</code>}</div></li>)}</ul></section>)}
    {missing.length > 0 && <div className="space-y-3 rounded-lg border p-4"><FormNotice>{missing.length} 项已配置权限暂没有可用说明。{editing ? '请核对并移除无效权限后再保存。' : '可开启编码显示核对。'}</FormNotice>{missing.map(code => <div key={code} className="flex min-w-0 items-center gap-3 text-sm">{editing && <Checkbox aria-label={`移除未识别权限 ${code}`} checked onCheckedChange={() => onChange?.(codes.filter(value => value !== code))} />}<span className="min-w-0 break-all">{showCodes ? code : '未识别的已配置权限'}</span></div>)}</div>}
    {!visible.length && !missing.length && <p className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">{q || group !== '全部' || selectedOnly ? '没有符合条件的权限。' : codes.length ? '权限说明暂不可用。' : '该角色暂未配置权限。'}</p>}
  </section>;
}
