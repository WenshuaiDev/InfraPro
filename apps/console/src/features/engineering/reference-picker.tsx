import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import type { EngineeringKind, EngineeringVersion } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { useOptionalTenant } from '../tenancy/context';
import { SelectControl, TextControl } from './controls';
export function VersionPicker({ kind, value, onChange, label, known = [] }: { kind: EngineeringKind; value: string; onChange: (id: string, version?: EngineeringVersion, dependencies?: EngineeringVersion[]) => void; label: string; known?: EngineeringVersion[] }) {
 const { client, account } = useIdentity(); const tenant = useOptionalTenant(); const current = known.find((v) => v.id === value);
 const [choice, setChoice] = useState(current?.definition_id ?? ''); const [q, setQ] = useState(''); const [offset, setOffset] = useState(0);
 const scope = { kind, tenant: tenant?.tenant.id, catalog: !!tenant };
 const prefix = tenant ? ['tenant', account?.id, tenant.tenant.id, tenant.membership_id, 'engineering'] : ['platform', account?.id, 'engineering'];
 const list = useQuery({ queryKey: [...prefix, 'picker', kind, q, offset], queryFn: ({ signal }) => client.engineeringList(scope, { q, status: 'usable', offset, limit: 20 }, signal) });
 const detail = useQuery({ queryKey: [...prefix, 'picker-detail', kind, choice], queryFn: ({ signal }) => client.engineeringDetail(scope, choice, signal), enabled: !!choice });
 const options = (list.data?.items ?? []).map((v) => ({ value: v.id, label: `${v.name} · ${v.code}` })); if (current && !options.some((v) => v.value === current.definition_id)) options.unshift({ value: current.definition_id, label: `${current.content.name} · ${current.code}` });
 const versions = (detail.data?.versions ?? []).filter((v) => v.usable || v.id === value);
 if (current && !versions.some((v) => v.id === current.id)) versions.unshift(current);
 return <div className="space-y-3 rounded-lg bg-muted/30 p-3"><p className="text-sm font-medium">{label}</p>{current && <p className="text-xs text-muted-foreground">当前：{current.content.name} · {current.code} V{current.number}{!current.usable && ' · 已不可新使用'}</p>}<div className="grid gap-3 sm:grid-cols-2"><TextControl label={`${label}搜索`} value={q} onChange={(v) => { setQ(v); setOffset(0); }} /><SelectControl label={`${label}定义`} value={choice} onChange={setChoice} options={options} /></div><div className="flex flex-wrap items-end gap-3"><div className="min-w-0 flex-1"><SelectControl label={`${label}版本`} value={versions.some((v) => v.id === value) ? value : ''} onChange={(id) => onChange(id, versions.find((v) => v.id === id), detail.data?.dependencies)} options={versions.map((v) => ({ value: v.id, label: `V${v.number} · ${v.content.name}${v.usable ? '' : '（历史）'}` }))} /></div><Button variant="ghost" type="button" onClick={() => { onChange(''); setChoice(''); }}>清除引用</Button></div>{list.isFetching && <p role="status" className="text-xs">加载可用定义…</p>}{(list.error || detail.error) && <FormNotice error>{list.error?.message ?? detail.error?.message}</FormNotice>}{(offset > 0 || (list.data?.total ?? 0) > 20) && <div className="flex gap-2"><Button type="button" variant="outline" size="sm" disabled={offset === 0} onClick={() => setOffset(offset - 20)}>上一页定义</Button><Button type="button" variant="outline" size="sm" disabled={offset + 20 >= (list.data?.total ?? 0)} onClick={() => setOffset(offset + 20)}>下一页定义</Button></div>}</div>;
}
