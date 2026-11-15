import { useRef, useState, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { AlertTriangle, Archive, ArrowRightLeft, ArrowUpRight, CalendarDays, ChevronDown, History, RotateCcw, ShieldCheck, Target, UserRound } from 'lucide-react';
import { ApiError, Permissions as P, type ProjectProfile } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { WorkspacePickerButton } from '../../app/workspace-picker-context';
import { ObjectHistory } from '../audit/object-history';
import { AttachmentPanel } from '../files/attachment-panel';
import { ProjectFiles } from '../files/context';
import { fileDate } from '../files/file-format';
import type { FileSearch } from '../files/state';
import { ProjectActionForm } from './actions';
import { useProject } from './context';
import { ProjectProfileEditor } from './profile-editor';

type ProfileAction = 'owner' | 'administrator-transfer' | 'archive' | 'restore';
const actionLabels: Record<ProfileAction, string> = { owner: '变更业务负责人', 'administrator-transfer': '交接项目管理员', archive: '归档项目', restore: '恢复项目' };
const actionDescriptions: Record<ProfileAction, string> = {
  owner: '只变更业务责任，不自动添加项目成员或授予角色。先核对真实影响，再填写原因并确认。',
  'administrator-transfer': '交接后原管理员退出本项目，租户成员关系保留；业务负责人不变。先核对真实影响，再填写原因并确认。',
  archive: '归档保留资料与历史查阅，停止资料编辑、负责人变更和新增附件。先核对真实影响，再填写原因并确认。',
  restore: '恢复沿用原项目和仍有效的成员关系，不补回已结束的关系或已撤销的角色。先核对真实影响，再确认恢复。',
};

export function ProjectProfilePage({ attachmentSearch, onAttachmentSearch }: {
  attachmentSearch: FileSearch;
  onAttachmentSearch: (search: FileSearch) => void;
}) {
  const ctx = useProject(), { client } = useIdentity();
  const { tenant_id: tenant, id: project } = ctx.project;
  const readable = ctx.can(P.ProjectProfileRead);
  const query = useQuery({
    queryKey: [...ctx.key, 'profile'], queryFn: ({ signal }) => client.projectProfile(tenant, project, signal),
    enabled: ctx.available && readable, retry: false, refetchInterval: 30_000,
  });
  const accessLost = query.error instanceof ApiError && [401, 403, 404].includes(query.error.status ?? 0);
  return <div className="mx-auto w-full max-w-6xl space-y-5">
    <h1 className="text-sm font-medium text-muted-foreground">项目资料</h1>
    {!readable ? <div className="space-y-4"><FormNotice>当前未获项目资料查看权限，请联系项目管理员。</FormNotice><WorkspacePickerButton filter="projects">重新选择项目</WorkspacePickerButton></div> : <>
      {query.error && <ErrorState message={query.error.message} onRetry={() => void query.refetch()} />}
      {!accessLost && (query.data ? <ProjectBrief profile={query.data} blocked={!ctx.available || !!query.error} attachmentSearch={attachmentSearch} onAttachmentSearch={onAttachmentSearch} /> : !query.error && <LoadingState label="正在加载项目资料…" />)}
    </>}
  </div>;
}

function ProjectBrief({ profile: p, blocked, attachmentSearch, onAttachmentSearch }: {
  profile: ProjectProfile;
  blocked: boolean;
  attachmentSearch: FileSearch;
  onAttachmentSearch: (search: FileSearch) => void;
}) {
  const ctx = useProject();
  const [action, setAction] = useState<ProfileAction | null>(null);
  const returnFocus = useRef<HTMLElement | null>(null), openingPanel = useRef(false);
  const archived = p.status === 'archived';
  const allowed: Record<ProfileAction, boolean> = {
    owner: !archived && ctx.allows('ProjectOwnerChange'),
    'administrator-transfer': ctx.allows('ProjectTransfer'),
    archive: !archived && ctx.allows('ProjectArchive'),
    restore: archived && ctx.allows('ProjectRestore'),
  };
  const edit = !archived && ctx.allows('ProjectUpdate');
  const history = ctx.can(P.ProjectAuditRead);
  const showMenu = history || Object.values(allowed).some(Boolean);
  const openAction = (value: ProfileAction, trigger?: HTMLElement) => {
    if (blocked || !allowed[value]) return;
    returnFocus.current = trigger ?? document.getElementById('project-actions');
    if (!trigger) openingPanel.current = true;
    setAction(value);
  };
  return <>
    <header className="flex flex-wrap items-start justify-between gap-5 pb-1">
      <div className="min-w-0 flex-1 basis-80">
        <h2 className="break-words text-2xl leading-snug font-semibold tracking-tight sm:text-3xl">{p.name}</h2>
        <div className="mt-3 flex flex-wrap items-center gap-3">
          <Badge variant="outline" className={archived ? 'gap-1.5 bg-muted/50 font-normal text-muted-foreground' : 'gap-1.5 border-transparent bg-emerald-50 font-normal text-emerald-800 dark:bg-emerald-950/50 dark:text-emerald-300'}>{archived ? <Archive aria-hidden="true" /> : <span aria-hidden="true" className="size-1.5 rounded-full bg-current" />}{archived ? '已归档' : '运行中'}</Badge>
          <span className="break-all font-mono text-xs text-muted-foreground" aria-label={`项目编号 ${p.code}`}>{p.code}</span>
          {!edit && !archived && <Badge variant="outline" className="font-normal text-muted-foreground">仅查看</Badge>}
        </div>
      </div>
      <div className="ml-auto flex items-center gap-2">
        {edit && <ProjectProfileEditor profile={p} blocked={blocked} />}
        {showMenu && <DropdownMenu><DropdownMenuTrigger asChild><Button id="project-actions" variant="outline" aria-label="更多项目操作">更多<ChevronDown aria-hidden="true" /></Button></DropdownMenuTrigger><DropdownMenuContent align="end" onCloseAutoFocus={event => { if (openingPanel.current) { event.preventDefault(); openingPanel.current = false; } }}>
          {allowed.owner && <DropdownMenuItem disabled={blocked} onSelect={() => openAction('owner')}><UserRound aria-hidden="true" />变更业务负责人</DropdownMenuItem>}
          {allowed['administrator-transfer'] && <DropdownMenuItem disabled={blocked} onSelect={() => openAction('administrator-transfer')}><ArrowRightLeft aria-hidden="true" />交接项目管理员</DropdownMenuItem>}
          {history && <DropdownMenuItem asChild><Link to="/t/$tenantId/p/$projectId/audit" params={{ tenantId: p.tenant_id, projectId: p.id }} search={{ object_type: 'project', object_id: p.id }}><History aria-hidden="true" />查看变更记录</Link></DropdownMenuItem>}
          {(allowed.archive || allowed.restore) && <DropdownMenuSeparator />}
          {allowed.archive && <DropdownMenuItem variant="destructive" disabled={blocked} onSelect={() => openAction('archive')}><Archive aria-hidden="true" />归档项目</DropdownMenuItem>}
          {allowed.restore && <DropdownMenuItem disabled={blocked} onSelect={() => openAction('restore')}><RotateCcw aria-hidden="true" />恢复项目</DropdownMenuItem>}
        </DropdownMenuContent></DropdownMenu>}
      </div>
    </header>

    {archived && <section aria-label="项目归档信息" className="flex flex-wrap items-start gap-3 rounded-lg border bg-muted/40 p-4 text-sm">
      <Archive aria-hidden="true" className="mt-0.5 size-5 shrink-0 text-muted-foreground" />
      <div className="min-w-0 flex-1 basis-64"><p className="font-medium">项目已归档 · 保留资料与历史查阅</p><p className="mt-1 whitespace-pre-wrap break-words leading-6 text-muted-foreground">{p.archive_reason || '未记录归档原因。'}</p>{p.archived_at && <p className="mt-1 text-xs text-muted-foreground">归档时间：<time dateTime={p.archived_at}>{fileDate(p.archived_at, true)}</time>（上海时间）</p>}<p className="mt-2 text-xs leading-6 text-muted-foreground">资料编辑、负责人变更与新增附件已关闭；管理员交接仍按当前资格办理。</p></div>
      {allowed.restore && <Button id="project-restore" variant="outline" size="sm" aria-disabled={blocked} onClick={event => openAction('restore', event.currentTarget)}><RotateCcw aria-hidden="true" />恢复项目</Button>}
    </section>}
    {(p.management_gap || !p.administrator.available) && <ManagementNotice title={p.management_gap ? '项目管理员空缺' : '项目管理员暂不可履职'}>请联系租户管理员，从项目治理办理管理恢复。业务负责人不会自动接任。</ManagementNotice>}
    {!p.owner.available && <ManagementNotice title="业务负责人暂不可用">原责任记录保留。{archived ? '项目恢复运行后，可重新指定负责人。' : '请核对业务协调安排，并由有权人员变更负责人。'}</ManagementNotice>}

    <section aria-labelledby="project-purpose-title" className="flex gap-3 rounded-lg bg-emerald-50/70 p-5 sm:gap-4 sm:px-7 sm:py-6 dark:bg-emerald-950/30">
      <Target aria-hidden="true" className="mt-0.5 size-5 shrink-0 text-emerald-800 sm:size-6 dark:text-emerald-300" />
      <div className="min-w-0"><h2 id="project-purpose-title" className="text-xs font-medium text-emerald-800 dark:text-emerald-300">业务目的</h2><p className="mt-2 whitespace-pre-wrap break-words text-base leading-8 font-medium sm:text-lg">{p.purpose}</p><p className="mt-3 whitespace-pre-wrap break-words text-sm leading-7 text-muted-foreground">{p.description || '尚未填写项目说明。'}</p></div>
    </section>
    <section aria-labelledby="project-plan-title" className="grid gap-4 border-y py-5 sm:grid-cols-[8rem_minmax(0,1fr)] lg:grid-cols-[9rem_minmax(0,1fr)_10rem] lg:items-center lg:gap-6">
      <h2 id="project-plan-title" className="flex items-center gap-2 text-sm font-medium"><CalendarDays aria-hidden="true" className="size-4" />计划周期</h2>
      <dl className="flex min-w-0 items-center gap-3 sm:gap-5"><PlanDate label="计划开始" value={p.planned_start} /><div aria-hidden="true" className="relative h-px min-w-4 flex-1 bg-border after:absolute after:top-[-2px] after:right-0 after:size-1 after:rounded-full after:bg-muted-foreground before:absolute before:top-[-2px] before:left-0 before:size-1 before:rounded-full before:bg-muted-foreground" /><PlanDate label="计划结束" value={p.planned_end} /></dl>
      <p className="text-xs leading-6 text-muted-foreground sm:col-start-2 lg:col-start-auto">日期用于计划安排<br />到期后不会自动归档</p>
    </section>
    <section aria-labelledby="project-people-title" className="border-b pt-1 pb-6">
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3"><h2 id="project-people-title" className="text-base font-semibold"><span aria-hidden="true" className="mr-3 font-mono text-xs font-normal text-muted-foreground">01</span>责任分工</h2>{ctx.can(P.ProjectMemberRead) && <Button variant="ghost" size="sm" className="text-emerald-800 dark:text-emerald-300" asChild><Link to="/t/$tenantId/p/$projectId/members" params={{ tenantId: p.tenant_id, projectId: p.id }}>项目成员<ArrowUpRight aria-hidden="true" /></Link></Button>}</div>
      <div className="grid gap-5 md:grid-cols-2 md:gap-7">
        <div className="flex min-w-0 items-start gap-3"><span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-blue-50 text-blue-700 dark:bg-blue-950/50 dark:text-blue-300"><UserRound aria-hidden="true" className="size-4" /></span><div className="min-w-0 flex-1"><h3 className="text-xs font-normal text-muted-foreground">业务负责人</h3><p className="mt-1 break-words text-sm font-medium">{p.owner.display_name || p.owner.username}</p><p className="mt-1 text-xs leading-6 text-muted-foreground">负责业务目标与协调，不自动取得项目权限。</p></div>{allowed.owner && <Button id="project-owner-change" aria-label="变更业务负责人" variant="ghost" size="sm" className="shrink-0 text-emerald-800 dark:text-emerald-300" aria-disabled={blocked} onClick={event => openAction('owner', event.currentTarget)}>变更</Button>}</div>
        <div className="flex min-w-0 items-start gap-3 border-t pt-5 md:border-t-0 md:border-l md:pt-0 md:pl-7"><span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-emerald-50 text-emerald-800 dark:bg-emerald-950/50 dark:text-emerald-300"><ShieldCheck aria-hidden="true" className="size-4" /></span><div className="min-w-0 flex-1"><h3 className="text-xs font-normal text-muted-foreground">项目管理员</h3><p className="mt-1 break-words text-sm font-medium">{p.management_gap ? '待任命管理员' : p.administrator.display_name || p.administrator.username}</p><p className="mt-1 text-xs leading-6 text-muted-foreground">负责成员、权限与项目日常管理。</p></div>{allowed['administrator-transfer'] && <Button id="project-administrator-transfer" aria-label="交接项目管理员" variant="ghost" size="sm" className="shrink-0 text-emerald-800 dark:text-emerald-300" aria-disabled={blocked} onClick={event => openAction('administrator-transfer', event.currentTarget)}>交接</Button>}</div>
      </div>
    </section>
    <div className="pt-1"><ProjectFiles><AttachmentPanel layout="list" title={<><span aria-hidden="true" className="mr-3 font-mono text-xs font-normal text-muted-foreground">02</span>项目附件</>} search={attachmentSearch} onSearch={onAttachmentSearch} /></ProjectFiles></div>
    <footer className="flex flex-wrap items-center justify-between gap-3 pt-2 text-xs text-muted-foreground"><p>资料更新于 <time dateTime={p.updated_at}>{fileDate(p.updated_at, true)}</time> · 上海时间</p><ObjectHistory objectType="project" objectId={p.id} variant="ghost" className="text-xs text-muted-foreground" label={<><History aria-hidden="true" />变更记录</>} /></footer>
    <ResponsivePanel open={!!action && allowed[action]} onOpenChange={open => { if (!open) setAction(null); }} title={action ? actionLabels[action] : '项目管理'} description={action ? actionDescriptions[action] : ''} returnFocus={returnFocus} fallbackFocusSelector="#project-actions">
      {action && allowed[action] && <ProjectActionForm key={action} tenant={p.tenant_id} project={p.id} action={action} label={actionLabels[action]} disabled={blocked} close={() => setAction(null)} />}
    </ResponsivePanel>
  </>;
}

function PlanDate({ label, value }: { label: string; value: string }) {
  return <div className="flex shrink-0 flex-col"><dt className="order-2 mt-1 text-xs text-muted-foreground">{label}</dt><dd className="font-mono text-base font-medium tracking-tight sm:text-lg">{value ? <time dateTime={value}>{value.replaceAll('-', '.')}</time> : '未设置'}</dd></div>;
}

function ManagementNotice({ title, children }: { title: string; children: ReactNode }) {
  return <div role="status" className="flex gap-3 rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-950 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-100"><AlertTriangle aria-hidden="true" className="mt-0.5 size-5 shrink-0" /><div className="min-w-0"><p className="font-medium">{title}</p><p className="mt-1 leading-6">{children}</p></div></div>;
}
