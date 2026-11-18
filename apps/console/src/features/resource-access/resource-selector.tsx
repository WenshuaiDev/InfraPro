import { useState } from 'react';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import type { ResourceAccessFilter, ResourceKind } from '@infrapro/api-client';
import { Building2, ChevronDown, ChevronRight, Layers, MapPin, Waves } from 'lucide-react';
import { Checkbox } from '@infrapro/ui/components/checkbox';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
import { cn } from '@infrapro/ui/lib/utils';
import { useResourceAccess } from './shared';
import { asChoice, assertAccessRevision, kinds, type Choice, type Draft } from './model';

const icons = { facility: Building2, component: Layers, monitoring_section: Layers, borehole: MapPin, point: MapPin, channel: Waves };
export type SelectorProps = {
  project: string;
  facility: string;
  revision: string;
  generation: number;
  draft: Draft;
  active: string;
  canWrite: boolean;
  archived: boolean;
  sourceManage: boolean;
  onActive: (value: Choice) => void;
  onToggle: (value: Choice, checked: boolean) => void;
};
function ResourceRow({
  value,
  depth = 0,
  expanded,
  onExpand,
  ...props
}: SelectorProps & { value: Choice; depth?: number; expanded?: boolean; onExpand?: () => void }) {
  const current = props.draft[value.id] ?? value;
  const checked = !current.end && (!!current.grant || !!props.draft[value.id]);
  const Icon = icons[value.kind];
  const canToggle =
    props.canWrite &&
    value.kind !== 'facility' &&
    (!props.archived || !!value.grant) &&
    (!value.grant?.capabilities.includes('source_manage') || props.sourceManage);
  return (
    <div
      className={cn(
        'flex min-h-16 items-center gap-2 border-b pr-3',
        props.active === value.id && 'bg-muted/60',
      )}
      style={{ paddingLeft: 8 + Math.min(depth, 3) * 14 }}
    >
      {onExpand ? (
        <button
          type="button"
          className="flex size-8 shrink-0 items-center justify-center rounded hover:bg-muted focus-visible:outline-ring"
          aria-label={`${expanded ? '收起' : '展开'} ${value.name}`}
          aria-expanded={expanded}
          onClick={onExpand}
        >
          {expanded ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
        </button>
      ) : (
        <span className="w-8 shrink-0" />
      )}
      <Checkbox
        aria-label={`开放 ${kinds[value.kind]} ${value.name}`}
        checked={checked}
        disabled={!canToggle}
        onCheckedChange={(yes) => props.onToggle(value, !!yes)}
      />
      <button
        type="button"
        className="flex min-w-0 flex-1 items-center gap-2 py-3 text-left focus-visible:outline-ring"
        onClick={() => props.onActive(value)}
      >
        <Icon className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        <span className="min-w-0">
          <span className="block break-words text-sm font-medium">{value.name}</span>
          <span className="mt-0.5 block break-all text-xs text-muted-foreground">
            {kinds[value.kind]} · {value.code}
            {value.status === 'retired' ? ' · 已退役' : ''}
          </span>
        </span>
      </button>
      <span
        className={cn(
          'max-w-16 shrink-0 text-right text-[11px] leading-4',
          props.draft[value.id] ? 'text-foreground' : 'text-muted-foreground',
        )}
      >
        {props.draft[value.id]
          ? current.end
            ? '待收回'
            : value.grant
              ? '待调整'
              : '待新增'
          : checked
            ? current.capabilities.length > 1
              ? '可维护'
              : '仅查看'
            : '未开放'}
      </span>
    </div>
  );
}
function CandidateList({
  kind,
  parent,
  structureParent,
  point,
  ownerKind,
  ownerID,
  q = '',
  depth = 0,
  flat = false,
  ...props
}: SelectorProps & {
  kind: ResourceKind;
  parent?: string;
  structureParent?: { kind: Exclude<ResourceKind, "point" | "channel">; id: string };
  point?: string;
  ownerKind?: Exclude<ResourceKind, 'channel'>;
  ownerID?: string;
  q?: string;
  depth?: number;
  flat?: boolean;
}) {
  const { client, tenant, prefix } = useResourceAccess(props.project);
  const filter: ResourceAccessFilter = {
    candidate_kind: kind,
    candidate_structure_parent_kind: structureParent?.kind,
    candidate_structure_parent_id: structureParent?.id,
    candidate_q: q,
    candidate_status: 'all',
    candidate_limit: 100,
    candidate_parent_id: parent ?? 'all',
    candidate_owner: parent ?? 'all',
    candidate_point_id: point,
    candidate_owner_kind: ownerKind,
    candidate_owner_id: ownerID,
  };
  const query = usePagedQuery({
    queryKey: [...prefix, 'facility', props.facility, props.revision, props.generation, 'tree', filter],
    queryFn: async ({ cursor, limit, signal }) => {
      const page = assertAccessRevision(await client.resourceAccess(
        tenant.tenant.id, props.project, props.facility,
        { ...filter, candidate_limit: limit, candidate_cursor: cursor }, signal,
      ), props.revision);
      const items = page.candidates.items.map(r => {
        const grant = page.candidate_grants.find(g => g.resource_id === r.id);
        return {
          kind: r.kind, id: r.id, name: r.name, code: r.code, status: r.status,
          source_kind: 'source_kind' in r ? r.source_kind : undefined,
          grant, capabilities: grant?.capabilities ?? ['read'],
        } satisfies Choice;
      });
      return { items, next_cursor: page.candidates.next_cursor };
    },
    refetchOnWindowFocus: false, refetchOnReconnect: false, staleTime: Infinity,
  });
  const items = query.data?.items ?? [];
  return (
    <>
      {query.error && (
        <div className="p-3">
          <ErrorState message={query.error.message} onRetry={() => void query.refetch()} />
        </div>
      )}
      {query.isPending && (
        <p role="status" className="p-3 text-xs text-muted-foreground">
          正在读取{kinds[kind]}…
        </p>
      )}
      {items.map((value) =>
        flat ? (
          <ResourceRow key={value.id} {...props} value={value} />
        ) : (
          <ResourceBranch key={value.id} {...props} value={value} depth={depth} />
        ),
      )}
      {!query.isPending && !query.error && !items.length && (
        <p className="px-5 py-2 text-xs text-muted-foreground">
          {q ? '未找到匹配的' : '暂无'}
          {kinds[kind]}
        </p>
      )}
      {query.data && (flat || query.pagination.hasNext || query.pagination.page > 0) &&
        <ListPagination label={kinds[kind]} {...query.pagination} />}
    </>
  );
}
function ResourceBranch({ value, depth = 0, ...props }: SelectorProps & { value: Choice; depth?: number }) {
  const [expanded, setExpanded] = useState(value.kind === 'facility');
  return (
    <div>
      <ResourceRow
        {...props}
        value={value}
        depth={depth}
        expanded={expanded}
        onExpand={value.kind !== 'channel' ? () => setExpanded(!expanded) : undefined}
      />
      {expanded && value.kind !== 'point' && value.kind !== 'channel' && (
        <>
          {(['component','monitoring_section','borehole','point'] as const).map(kind => <CandidateList key={kind} {...props} kind={kind} structureParent={{kind:value.kind as Exclude<ResourceKind,'point'|'channel'>,id:value.id}} depth={depth+1} />)}
          <CandidateList {...props} kind="channel" ownerKind={value.kind} ownerID={value.id} depth={depth + 1} />
        </>
      )}
      {expanded && value.kind === 'point' && (
        <CandidateList {...props} kind="channel" point={value.id} depth={depth + 1} />
      )}
    </div>
  );
}
function SelectedList({ q, kind, ...props }: SelectorProps & { q: string; kind: ResourceKind | 'all' }) {
  const { client, tenant, prefix } = useResourceAccess(props.project);
  const query = usePagedQuery({
    queryKey: [...prefix, 'facility', props.facility, props.revision, props.generation, 'selected', kind, q, props.draft],
    queryFn: async ({ cursor, signal }) => {
      const page = assertAccessRevision(await client.resourceAccess(
        tenant.tenant.id, props.project, props.facility, { grant_cursor: cursor, limit: 100 }, signal,
      ), props.revision);
      const grants = page.grants.map(grant => props.draft[grant.resource_id] ?? asChoice(grant));
      const added = cursor ? [] : Object.values(props.draft).filter(choice => !choice.grant);
      const items = [...added, ...grants].filter(choice => !choice.end &&
        (kind === 'all' || kind === choice.kind) && `${choice.name} ${choice.code}`.toLowerCase().includes(q.toLowerCase()));
      return { items, next_cursor: page.grant_next_cursor };
    },
    refetchOnWindowFocus: false, refetchOnReconnect: false, staleTime: Infinity,
  });
  const values = query.data?.items ?? [];
  return (
    <>
      {query.error && (
        <div className="p-3">
          <ErrorState message={query.error.message} onRetry={() => void query.refetch()} />
        </div>
      )}
      {query.isPending && (
        <p role="status" className="p-3 text-sm">
          正在读取已选资源…
        </p>
      )}
      {values.map((value) => (
        <ResourceRow key={value.id} {...props} value={value} />
      ))}
      {!query.isPending && !values.length && (
        <p className="p-5 text-sm text-muted-foreground">
          没有匹配的已选资源。
        </p>
      )}
      {query.data && <ListPagination label="已选资源" {...query.pagination} />}
    </>
  );
}
export function ResourceSelector({
  root,
  q,
  kind,
  onlySelected,
  ...props
}: SelectorProps & { root: Choice; q: string; kind: ResourceKind | 'all'; onlySelected: boolean }) {
  return (
    <section aria-label="资源选择" className="min-w-0 overflow-hidden rounded-lg border">
      <div className="border-b bg-muted/30 px-4 py-3 text-xs text-muted-foreground">
        勾选开放查看 · 点名称设置权限
        {onlySelected ? ' · 已选资源' : q || kind !== 'all' ? ' · 搜索整个设施' : ''}
      </div>
      <div className="max-h-[560px] overflow-y-auto">
        {onlySelected ? (
          <SelectedList {...props} q={q} kind={kind} />
        ) : q || kind !== 'all' ? (
          <>
            {(kind === 'all' || kind === 'facility') &&
              `${root.name} ${root.code}`.toLowerCase().includes(q.toLowerCase()) && (
                <ResourceRow {...props} value={root} />
              )}{' '}
            {(['component', 'monitoring_section', 'borehole', 'point', 'channel'] as const)
              .filter((k) => kind === 'all' || kind === k)
              .map((k) => (
                <CandidateList key={k} {...props} kind={k} q={q} flat />
              ))}
          </>
        ) : (
          <ResourceBranch {...props} value={root} />
        )}
      </div>
    </section>
  );
}
