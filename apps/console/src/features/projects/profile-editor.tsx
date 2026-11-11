import { useLivePage } from '../tenancy/membership-shared';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useQueryClient } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { type ProjectPerson, type ProjectProfile, Permissions as P } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Form } from '@infrapro/ui/components/form';
import { FormInput } from '@infrapro/ui/patterns/form-input';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@infrapro/ui/components/dialog';
import { z } from '../../config/validation';
import { useTenant } from '../tenancy/context';
import { useProject } from './context';
import { CandidatePicker } from './shared';
import { MutationForm } from './actions';
import { refreshProjects } from './scope';
const schema = z.object({ name: z.string().transform((s) => s.normalize('NFKC').trim()).refine((s) => Array.from(s).length >= 1 && Array.from(s).length <= 100, '名称需为 1–100 字符。'), code: z.string().trim().toLowerCase().regex(/^[a-z0-9][a-z0-9_-]{2,31}$/, '编号需为 3–32 位字母、数字、下划线或连字符。'), purpose: z.string().trim().min(1, '请填写业务目的。').max(1000), description: z.string().max(2000), planned_start: z.string(), planned_end: z.string() }).refine((v) => !v.planned_start || !v.planned_end || v.planned_start <= v.planned_end, { message: '结束日期不得早于开始日期。', path: ['planned_end'] });
export function ProjectCreatePage() { const ctx = useTenant(); if (!ctx.permissions.includes(P.TenantProjectCreate) || !ctx.permissions.includes(P.TenantProjectRead)) return <p>没有项目创建权限。</p>; return <section className="max-w-3xl space-y-5"><h1 className="text-2xl font-semibold">创建项目</h1><p>创建者、负责人和管理员分别记录。仅指定管理员自动成为项目成员。</p><Editor tenant={ctx.tenant.id} available={ctx.available} /></section>; }
export function ProjectProfileEditor({ profile }: { profile: ProjectProfile }) { const ctx = useProject(); const [open, setOpen] = useState(false); return <Dialog open={open} onOpenChange={setOpen}><DialogTrigger asChild><Button disabled={!ctx.available}>编辑项目资料</Button></DialogTrigger><DialogContent className="max-h-[88dvh] overflow-y-auto sm:max-w-2xl"><DialogHeader><DialogTitle>编辑项目资料</DialogTitle><DialogDescription>编号历史永久归属原项目；计划日期不自动触发归档。</DialogDescription></DialogHeader>{open && <Editor tenant={profile.tenant_id} initial={profile} available={ctx.available} done={() => setOpen(false)} />}</DialogContent></Dialog>; }
function Editor({ tenant, initial, available, done }: { tenant: string; initial?: ProjectProfile; available: boolean; done?: () => void }) {
  const { client, account } = useIdentity(); const cache = useQueryClient(); const live = useLivePage(); const [profile, setProfile] = useState(initial); const [owner, setOwner] = useState<ProjectPerson>(); const [admin, setAdmin] = useState<ProjectPerson>(); const [result, setResult] = useState<{ id: string; name: string; admin: string; canEnter: boolean }>();
  const form = useForm({ resolver: zodResolver(schema), defaultValues: { name: initial?.name ?? '', code: initial?.code ?? '', purpose: initial?.purpose ?? '', description: initial?.description ?? '', planned_start: initial?.planned_start ?? '', planned_end: initial?.planned_end ?? '' } });
  const refresh = async () => { if (profile) setProfile(await client.projectProfile(tenant, profile.id)); };
  if (result) return <section className="space-y-4 rounded-xl border p-5"><h2 className="text-xl font-semibold">项目已创建</h2><p>{result.name} · 指定管理员：{result.admin}</p>{result.canEnter && <Button asChild><Link to="/t/$tenantId/p/$projectId/profile" params={{ tenantId: tenant, projectId: result.id }}>进入项目</Link></Button>}<Button asChild variant="outline"><Link to="/t/$tenantId/projects" params={{ tenantId: tenant }}>返回项目治理</Link></Button></section>;
  return <Form {...form}><MutationForm label={profile ? '保存项目资料' : '确认创建项目'} disabled={!available} refresh={refresh} cancel={done} submit={async (reason, requestId) => {
    if (!await form.trigger()) throw new Error('请检查标记的字段。'); const fields = schema.parse(form.getValues());
    try { if (profile) { await client.updateProject(tenant, profile.id, { ...fields, reason, client_request_id: requestId, expected_version: profile.version, expected_project_version: profile.version }); if (!live.current) return; refreshProjects(cache, account?.id, tenant, profile.id); done?.(); } else { if (!owner || !admin) throw new Error('请选择业务负责人及初始管理员。'); const r = await client.createProject(tenant, { ...fields, reason, client_request_id: requestId, owner_tenant_membership_id: owner.tenant_membership_id, administrator_tenant_membership_id: admin.tenant_membership_id }); if (!live.current) return; refreshProjects(cache, account?.id, tenant); setResult({ id: r.project_id, name: fields.name, admin: admin.display_name, canEnter: admin.account_id === account?.id }); } } catch (e) { if (e && typeof e === 'object' && 'fields' in e) { const errors = e.fields as Record<string, string>; for (const k of ['name', 'code', 'purpose', 'description', 'planned_start', 'planned_end'] as const) if (errors[k]) form.setError(k, { message: errors[k] }); } throw e; }
  }}><FormInput control={form.control} name="name" label="项目名称" hint="1–100 字符；名称允许重复，以编号区分。" /><FormInput control={form.control} name="code" label="项目编号" hint="3–32 位；租户内唯一，历史编号保留。" /><FormInput control={form.control} name="purpose" label="业务目的" hint="必填，最多 1000 字符纯文本。" /><FormInput control={form.control} name="description" label="项目说明" hint="最多 2000 字符纯文本。" /><div className="grid gap-4 sm:grid-cols-2"><FormInput control={form.control} name="planned_start" label="计划开始日期" type="date" /><FormInput control={form.control} name="planned_end" label="计划结束日期" type="date" /></div>{!profile && <><CandidatePicker tenant={tenant} kind="create" label="选择业务负责人" value={owner} onChange={setOwner} /><CandidatePicker tenant={tenant} kind="create" label="选择初始管理员" value={admin} onChange={setAdmin} /></>}{profile && initial && profile.version !== initial.version && <FormNotice>服务器已更新至版本 {profile.version}：{profile.name} / {profile.code} / {profile.purpose}。本地输入保留，请比较后重新提交。</FormNotice>}</MutationForm></Form>;
}
