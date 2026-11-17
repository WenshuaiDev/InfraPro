import type { ProjectSummary, ProjectGovernanceOverview } from '@infrapro/api-client';
import { ArrowRight, CalendarDays, FolderKanban, ShieldCheck, TriangleAlert, UserRound } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { plannedEndNotice, projectDate, type GovernanceSearch, type GovernanceView } from './governance-search';

type CardProps = {
  project: ProjectSummary;
  onOpen: (id: string, view: GovernanceView, trigger: HTMLElement) => void;
  onOwner: (id: string) => void;
  hasAccess: boolean;
};

export function ProjectStatus({ status }: { status: string }) {
  return <Badge variant="secondary" className={status === 'active' ? 'border-transparent bg-emerald-50 text-emerald-800 dark:bg-emerald-950/50 dark:text-emerald-300' : ''}>
    <span aria-hidden="true" className="size-1.5 rounded-full bg-current" />{status === 'active' ? '运行中' : '已归档'}
  </Badge>;
}

export function ProjectScale({ statistics }: { statistics?: ProjectSummary['statistics'] }) {
  const metrics = [
    { key: 'facilities', label: '授权设施', value: statistics?.facilities },
    { key: 'points', label: '授权测点', value: statistics?.points },
    { key: 'devices', label: '授权设备', value: statistics?.devices },
    { key: 'members', label: '当前成员', value: statistics?.members },
  ];
  return <dl className="grid grid-cols-4 divide-x" aria-label="项目规模">
    {metrics.map(metric => <div key={metric.key} className="flex min-w-0 flex-col px-2 text-center first:pl-0 first:text-left last:pr-0" title={metric.value === null ? '无查看权限' : metric.value === undefined ? '尚未取得统计' : metric.label + '：' + metric.value}>
      <dt className="order-2 mt-1.5 text-xs text-muted-foreground">{metric.label}</dt>
      <dd className="text-2xl leading-8 font-semibold tracking-tight tabular-nums">{metric.value == null ? <><span aria-hidden="true">—</span><span className="sr-only">{metric.value === null ? '无查看权限' : '尚未取得统计'}</span></> : metric.value.toLocaleString('zh-CN')}</dd>
    </div>)}
  </dl>;
}

function ProjectCover({ id }: { id: string }) {
  const pattern = id.charCodeAt(0) % 3;
  return <svg viewBox="0 0 360 86" fill="none" stroke="currentColor" strokeWidth="1.2" className="absolute right-0 bottom-0 h-24 w-4/5 text-emerald-800/30 dark:text-emerald-300/25" aria-hidden="true">
    {pattern === 0 ? <><path d="M0 64h360M60 76V16m210 60V16M0 57C45 57 48 16 60 16s50 38 105 38S260 16 270 16s46 41 90 41M95 36v28m35-15v15m35-10v10m35-15v15m35-28v28" /><path d="M0 78c25-6 35 6 60 0s35 6 60 0 35 6 60 0 35 6 60 0 35 6 60 0 35 6 60 0" opacity=".5" /></>
      : pattern === 1 ? <><path d="M35 85C95 68 68 24 125 14S216 22 243 1M135 85C190 60 150 38 190 29s74 9 96-28" /><path d="M85 85c53-20 18-49 62-62s87 10 118-22" strokeDasharray="8 6" /><path d="M0 69h39m117-2h88M12 50h43m125 2h105" opacity=".5" /></>
        : <path d="M50 78V50a28 28 0 0 1 56 0v28m-48 0V51a20 20 0 0 1 40 0v27M145 78V40a38 38 0 0 1 76 0v38m-64 0V41a26 26 0 0 1 52 0v37M263 78V50a28 28 0 0 1 56 0v28m-48 0V51a20 20 0 0 1 40 0v27M0 78h360" />}
  </svg>;
}

export function GovernanceCard({ project: p, onOpen, onOwner, hasAccess }: CardProps) {
  const attention = !p.administrator.available;
  const owner = p.owner.display_name || p.owner.username;
  const administrator = p.administrator.display_name || p.administrator.username;
  const plan = plannedEndNotice(p.planned_end, p.status);
  return <article data-project-card={p.id} aria-label={p.name} className="flex min-w-0 flex-col overflow-hidden rounded-xl border bg-card shadow-xs transition-shadow hover:shadow-md motion-reduce:transition-none">
    <div className="relative flex h-24 items-start justify-between overflow-hidden border-b bg-emerald-50/50 p-5 dark:bg-emerald-950/15">
      <ProjectCover id={p.id} />
      <span className="relative grid size-9 place-items-center rounded-lg border bg-background/90 text-muted-foreground"><FolderKanban className="size-5" aria-hidden="true" /></span>
      <span className="relative"><ProjectStatus status={p.status} /></span>
    </div>
    <div className="flex flex-1 flex-col p-5">
      <h2><button id={'project-title-' + p.id} className="rounded-sm text-left text-base leading-6 font-semibold break-words hover:text-emerald-700 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring dark:hover:text-emerald-300" onClick={event => onOpen(p.id, 'summary', event.currentTarget)}>{p.name}</button></h2>
      <p className="mt-1.5 text-xs break-all text-muted-foreground">{p.code}</p>
      <div className="my-5 border-y py-4"><ProjectScale statistics={p.statistics} /></div>
      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2 text-xs">
        <button className="flex min-h-8 min-w-0 items-center gap-2 rounded text-left text-muted-foreground hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring" aria-label={'按负责人 ' + owner + ' 筛选'} onClick={() => onOwner(p.owner.tenant_membership_id)}>
          <span aria-hidden="true" className="grid size-6 shrink-0 place-items-center rounded-full bg-muted"><UserRound className="size-3.5" /></span>
          <span className="break-words">负责人 <span className={!p.owner.available ? 'text-amber-700 dark:text-amber-300' : 'text-foreground'}>{owner}{!p.owner.available && '（不可用）'}</span></span>
        </button>
        <span className={'min-w-0 break-words ' + (attention ? 'text-amber-700 dark:text-amber-300' : 'text-muted-foreground')}>管理员 {p.management_gap ? '待指定' : administrator}</span>
      </div>
      <p className="mt-3 flex items-start gap-2 text-xs leading-5 text-muted-foreground"><CalendarDays className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" /><span className="tabular-nums">{projectDate(p.planned_start)} — {projectDate(p.planned_end)}</span></p>
      {(attention || plan) && <p className="mt-3 flex items-start gap-2 rounded-md bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800 dark:bg-amber-950/30 dark:text-amber-300"><TriangleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" /><span>{[attention ? p.management_gap ? '管理员空缺，需要恢复管理' : '管理员不可用，需要恢复管理' : '', plan].filter(Boolean).join(' · ')}</span></p>}
    </div>
    <footer className="flex flex-wrap items-center justify-between gap-1 border-t px-3 py-2">
      <Button id={'project-detail-' + p.id} variant="ghost" size="sm" className="min-h-10 text-emerald-800 dark:text-emerald-300" onClick={event => onOpen(p.id, 'summary', event.currentTarget)}>治理详情<ArrowRight className="size-4" aria-hidden="true" /></Button>
      {hasAccess && <Button id={'project-access-' + p.id} variant="ghost" size="sm" className="min-h-10 text-muted-foreground" onClick={event => onOpen(p.id, 'access', event.currentTarget)}><ShieldCheck className="size-4" aria-hidden="true" />授权管理</Button>}
    </footer>
  </article>;
}

export function GovernanceOverview({ value, search, onFilter }: { value?: ProjectGovernanceOverview; search: GovernanceSearch; onFilter: (filter: 'all' | 'active' | 'attention') => void }) {
  const items = [
    { key: 'all' as const, name: '全部项目', value: value?.total, note: value ? '含 ' + value.archived + ' 个已归档项目' : '正在读取统计', selected: (search.status ?? 'all') === 'all' && !search.unavailable },
    { key: 'active' as const, name: '运行中', value: value?.active, note: '当前处于运行状态的项目', selected: search.status === 'active' && !search.unavailable },
    { key: 'attention' as const, name: '需要关注', value: value?.attention, note: '管理员不可用或空缺', selected: !!search.unavailable },
  ];
  return <section aria-label="租户项目全景" className="grid overflow-hidden rounded-xl border bg-card shadow-xs lg:grid-cols-[minmax(16rem,0.9fr)_2fr]">
    <div className="flex flex-col justify-center gap-2 bg-emerald-50/60 p-5 sm:p-6 dark:bg-emerald-950/25"><h2 className="text-lg font-semibold">项目全景</h2><p className="max-w-sm text-xs leading-6 text-muted-foreground">掌握项目规模、管理团队与计划周期。<br className="hidden sm:block" />总览统计覆盖本租户全部项目。</p></div>
    <div className="grid grid-cols-3 divide-x py-5">
      {items.map(item => <button key={item.key} className="min-w-0 rounded-sm px-3 text-left focus-visible:outline-2 focus-visible:outline-ring sm:px-6" onClick={() => onFilter(item.key)} aria-label={'查看' + item.name} aria-pressed={item.selected}>
        <span className="text-xs text-muted-foreground">{item.name}</span>
        <strong className={'my-1.5 block text-3xl font-semibold tracking-tight tabular-nums ' + (item.key === 'attention' && item.value ? 'text-amber-700 dark:text-amber-300' : '')}>{item.value?.toLocaleString('zh-CN') ?? '—'}</strong>
        <span className="block text-xs leading-5 text-muted-foreground">{item.note}</span>
      </button>)}
    </div>
  </section>;
}
