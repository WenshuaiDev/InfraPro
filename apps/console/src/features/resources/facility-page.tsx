import { useRef, useState } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { ApiError, type Resource, type ResourceDetail, type ResourceTarget } from '@infrapro/api-client';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@infrapro/ui/components/dropdown-menu';
import {
  DirectoryTable,
  DirectorySearch,
  DirectoryReset,
  DirectoryActions,
} from '@infrapro/ui/patterns/directory-table';
import { ListFilter, ListPagination } from '@infrapro/ui/patterns/list-controls';
import { Tabs, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { Badge } from '@infrapro/ui/components/badge';
import { Plus, ArrowLeft, MoreHorizontal } from 'lucide-react';
import { useCursorList } from '../../hooks/use-cursor-list';
import { SelectControl, TextControl } from '../engineering-fields/controls';
import { FacilityUsage } from '../resource-access/pages';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { useResources } from './context';
import { ResourceEditor } from './editor';
import { ResourceHistory } from './history';
import { LifecycleForm } from './lifecycle';
import { PositionView } from './position-editor';
export type Selection = {
  component?: string;
  point?: string;
  channel?: string;
  tab?: 'details' | 'components' | 'points' | 'channels' | 'usage' | 'changes';
};
type Panel = {
  mode: 'create' | 'edit' | 'retire' | 'restore' | 'delete';
  target: ResourceTarget;
  resource?: ResourceDetail;
  owner?: ResourceDetail;
};
const labels = { facility: '设施', component: '构件', point: '测点', channel: '通道' };
const modes = { create: '新增', edit: '编辑', retire: '退役', restore: '恢复', delete: '删除' };
export function FacilityPage(props: { facilityId: string; selection: Selection; select: (s: Selection) => void }) {
  const { canRead } = useResources();
  if (!canRead) return <ErrorState title="没有工程资源查看权限" message="请联系租户管理员配置相应角色。" />;
  return <FacilityBody key={props.facilityId} {...props} />;
}
function FacilityBody({
  facilityId,
  selection,
  select,
}: {
  facilityId: string;
  selection: Selection;
  select: (s: Selection) => void;
}) {
  const { client, scope, paths, prefix, canManage, available, canHistory, canGovern } = useResources();
  const navigate = useNavigate();
  const [panel, setPanel] = useState<Panel>();
  const returnFocus = useRef<HTMLElement | null>(null);
  const openPanel = (p: Panel, trigger: HTMLElement | null) => {
    returnFocus.current = trigger;
    setPanel(p);
  };
  const channel = selection.channel;
  const tab =
    selection.tab ??
    (channel ? 'channels' : selection.point ? 'points' : selection.component ? 'components' : 'details');
  const history = tab === 'changes';
  const [filter, setFilter] = useState({ q: '', status: 'all' });
  const [pageSize, setPageSize] = useState(20);
  const [pointSearch, setPointSearch] = useState('');
  const choose = (s: Selection) => {
    const nextTab = s.tab ?? (s.channel ? 'channels' : s.point ? 'points' : s.component ? 'components' : 'details');
    if (
      nextTab !== tab ||
      (nextTab === 'points' && s.component !== selection.component) ||
      (nextTab === 'channels' && s.point !== selection.point)
    ) {
      setFilter({ q: '', status: 'all' });
    }
    select(s);
  };
  const facility = useQuery({
    queryKey: [...prefix, 'detail', 'facility', facilityId],
    queryFn: ({ signal }) => client.resourceDetail(scope, { kind: 'facility', id: facilityId }, signal),
    retry: false,
  });
  const component = useQuery({
    queryKey: [...prefix, 'detail', 'component', facilityId, selection.component],
    queryFn: ({ signal }) =>
      client.resourceDetail(scope, { kind: 'component', facility: facilityId, id: selection.component! }, signal),
    enabled: !!selection.component,
    retry: false,
  });
  const point = useQuery({
    queryKey: [...prefix, 'detail', 'point', facilityId, selection.point],
    queryFn: ({ signal }) =>
      client.resourceDetail(scope, { kind: 'point', facility: facilityId, id: selection.point! }, signal),
    enabled: !!selection.point,
    retry: false,
  });
  const channelDetail = useQuery({
    queryKey: [...prefix, 'detail', 'channel', facilityId, selection.point, channel],
    queryFn: ({ signal }) =>
      client.resourceDetail(
        scope,
        { kind: 'channel', facility: facilityId, point: selection.point, id: channel! },
        signal,
      ),
    enabled: !!channel && !!selection.point,
    retry: false,
  });
  const components = useCursorList({
    queryKey: [...prefix, 'children', facilityId, 'all', filter, pageSize],
    load: (cursor, signal) =>
      client.resourceList(
        scope,
        { kind: 'component', facility: facilityId },
        {
          parent_id: 'all',
          q: filter.q,
          status: filter.status as 'all' | 'normal' | 'retired',
          limit: pageSize,
          cursor,
        },
        signal,
      ),
    enabled: !!facility.data && tab === 'components' && !selection.component,
  });
  const points = useCursorList({
    queryKey: [...prefix, 'points', facilityId, selection.component ?? 'all', filter, pageSize],
    load: (cursor, signal) =>
      client.resourceList(
        scope,
        { kind: 'point', facility: facilityId },
        {
          owner: selection.component ?? 'all',
          q: filter.q,
          status: filter.status as 'all' | 'normal' | 'retired',
          limit: pageSize,
          cursor,
        },
        signal,
      ),
    enabled: !!facility.data && tab === 'points' && !selection.point,
  });
  const channels = useCursorList({
    queryKey: [...prefix, 'channels', facilityId, selection.point, filter, pageSize],
    load: (cursor, signal) =>
      client.resourceList(
        scope,
        { kind: 'channel', facility: facilityId, point: selection.point },
        { q: filter.q, status: filter.status as 'all' | 'normal' | 'retired', limit: pageSize, cursor },
        signal,
      ),
    enabled: !!selection.point && !history,
  });
  const pointOptions = useCursorList({
    queryKey: [...prefix, 'points', facilityId, 'channel-picker', pointSearch],
    load: (cursor, signal) =>
      client.resourceList(
        scope,
        { kind: 'point', facility: facilityId },
        { owner: 'all', q: pointSearch, status: 'all', limit: 20, cursor },
        signal,
      ),
    enabled: tab === 'channels' && !channel,
  });
  const contextFailure = (query: { error: Error | null }) =>
    scope.kind === 'project' && query.error instanceof ApiError && query.error.status === 404;
  const selected = selection.component ? component : facility;
  const owner = selected.data;
  const pointContext = [...points.items, ...pointOptions.items].find(
    (p) => p.id === selection.point && p.visibility === 'context',
  );
  const hasPointContext = !!pointContext || (contextFailure(point) && !channels.error);
  const restricted = [facility, component, point, channelDetail].find(
    (query) => query.error instanceof ApiError && [401, 403].includes(query.error.status ?? 0),
  );
  if (restricted) return <ErrorState title="当前资源不可访问" message={restricted.error!.message} />;
  if (facility.error && !facility.data && !history)
    return <ErrorState title="设施不可访问" message={facility.error.message} onRetry={() => void facility.refetch()} />;
  const blocked =
    !!facility.error ||
    (!!selected.error && !contextFailure(selected)) ||
    (!!point.error && !hasPointContext) ||
    !!channelDetail.error;
  const targetFor = (r: Resource): ResourceTarget => ({
    kind: r.kind,
    facility: facilityId,
    point: r.point_id,
    id: r.id,
  });
  const actions = (r: ResourceDetail) => (
    <ObjectActions
      resource={r}
      disabled={blocked || !canManage || !available}
      choose={(mode, trigger) => openPanel({ mode, target: targetFor(r), resource: r }, trigger)}
    />
  );
  const create = (kind: 'component' | 'point' | 'channel', parent: ResourceDetail | undefined) =>
    parent &&
    canManage &&
    parent.actions[kind === 'component' ? 'create_component' : kind === 'point' ? 'create_point' : 'create_channel'] ? (
      <Button
        disabled={blocked || !available}
        onClick={(event) =>
          openPanel(
            {
              mode: 'create',
              target: { kind, facility: facilityId, ...(kind === 'channel' ? { point: selection.point } : {}) },
              owner: parent,
            },
            event.currentTarget,
          )
        }
      >
        <Plus aria-hidden="true" />
        {kind === 'component' ? '新增构件' : kind === 'point' ? '添加测点' : '添加通道'}
      </Button>
    ) : null;
  const list = tab === 'components' ? components : tab === 'points' ? points : channels;
  const listKind = tab === 'components' ? 'component' : tab === 'points' ? 'point' : 'channel';
  const listLabel = tab === 'components' ? '构件列表' : tab === 'points' ? '测点列表' : '业务通道列表';
  const showList =
    (tab === 'components' && !selection.component) ||
    (tab === 'points' && !selection.point) ||
    (tab === 'channels' && !!selection.point && !channel);
  const object = tab === 'components' ? component : tab === 'points' ? point : channelDetail;
  const detailKind = tab === 'components' ? 'component' : tab === 'points' ? 'point' : 'channel';
  const showObject =
    (tab === 'components' && !!selection.component) ||
    (tab === 'points' && !!selection.point) ||
    (tab === 'channels' && !!channel);
  const pickerOptions = pointOptions.items.map((p) => ({ value: p.id, label: `${p.name} · ${p.code}` }));
  if (selection.point && !pickerOptions.some((p) => p.value === selection.point))
    pickerOptions.unshift({ value: selection.point, label: point.data?.name ?? pointContext?.name ?? '当前测点' });
  const type = facility.data?.definitions.find((v) => v.id === facility.data?.type_version_id);
  return (
    <section id="resource-workspace-anchor" tabIndex={-1} className="min-w-0 space-y-5">
      <Button asChild variant="ghost" className="-ml-3">
        <Link to={paths.list}>
          <ArrowLeft aria-hidden="true" />
          返回设施列表
        </Link>
      </Button>
      <PageHeader
        title={facility.data?.name ?? (history ? '设施变更记录' : '正在读取设施…')}
        description={
          facility.data
            ? `${facility.data.code} · ${facility.data.status === 'normal' ? '正常' : '已退役'}${type ? ` · ${type.content.name} V${type.number}` : ''}`
            : undefined
        }
        actions={facility.data && actions(facility.data)}
      />
      <Tabs value={tab} onValueChange={(value) => choose({ tab: value as Selection['tab'] })}>
        <TabsList className="flex h-auto flex-wrap justify-start">
          <TabsTrigger value="details">基本资料</TabsTrigger>
          <TabsTrigger value="components">构件</TabsTrigger>
          <TabsTrigger value="points">测点</TabsTrigger>
          <TabsTrigger value="channels">业务通道</TabsTrigger>
          {canGovern && <TabsTrigger value="usage">项目使用</TabsTrigger>}
          {canHistory && <TabsTrigger value="changes">设施变更记录</TabsTrigger>}
        </TabsList>
      </Tabs>
      {history ? (
        canHistory ? (
          <ResourceHistory facility={facilityId} />
        ) : (
          <ErrorState message="没有资源历史查看权限。" />
        )
      ) : tab === 'usage' ? (
        canGovern ? (
          <FacilityUsage facility={facilityId} />
        ) : (
          <ErrorState message="没有项目资源授权查看权限。" />
        )
      ) : (
        <>
          {blocked && (
            <ErrorState
              title="暂时无法读取，输入已保留"
              message={
                facility.error?.message ??
                selected.error?.message ??
                point.error?.message ??
                channelDetail.error?.message ??
                ''
              }
              onRetry={() => {
                void facility.refetch();
                if (selection.component) void component.refetch();
                if (selection.point) void point.refetch();
                if (channel) void channelDetail.refetch();
              }}
            />
          )}
          {tab === 'details' && facility.data && (
            <section className="space-y-4 rounded-lg border p-5">
              <h2 className="font-semibold">设施资料</h2>
              <ResourceInfo resource={facility.data} />
            </section>
          )}
          {tab === 'components' && !selection.component && (
            <div className="flex items-center justify-between gap-3">
              <h2 className="text-lg font-semibold">构件</h2>
              {create('component', facility.data)}
            </div>
          )}
          {tab === 'points' && !selection.point && (
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h2 className="text-lg font-semibold">
                  {selection.component ? `${owner?.name ?? '当前构件'}的测点` : '测点'}
                </h2>
                {selection.component && (
                  <Button variant="link" className="p-0" onClick={() => choose({ tab: 'points' })}>
                    查看全部测点
                  </Button>
                )}
              </div>
              {create('point', owner)}
            </div>
          )}
          {tab === 'channels' && !channel && (
            <div className="space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <h2 className="text-lg font-semibold">业务通道</h2>
                {point.data && !point.error && create('channel', point.data)}
              </div>
              <div className="grid items-end gap-3 rounded-lg border bg-muted/20 p-4 sm:grid-cols-2">
                <TextControl label="搜索所属测点" value={pointSearch} onChange={setPointSearch} />
                <SelectControl
                  label="所属测点"
                  value={selection.point ?? ''}
                  onChange={(id) => choose({ tab: 'channels', point: id || undefined })}
                  options={pickerOptions}
                />
                {pointOptions.error && (
                  <ErrorState message={pointOptions.error.message} onRetry={() => void pointOptions.refetch()} />
                )}
                <div className="flex flex-wrap gap-2 sm:col-span-2">
                  {pointOptions.page > 0 && (
                    <Button variant="outline" size="sm" onClick={() => pointOptions.go(pointOptions.page - 1)}>
                      上一页测点
                    </Button>
                  )}
                  {pointOptions.hasNext && (
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={pointOptions.isFetching}
                      onClick={() => void pointOptions.next()}
                    >
                      下一页测点
                    </Button>
                  )}
                  {point.data && !point.error && (
                    <Button variant="ghost" size="sm" onClick={() => choose({ ...selection, tab: 'points' })}>
                      查看所属测点资料
                    </Button>
                  )}
                </div>
              </div>
              {!selection.point && (
                <p className="py-8 text-center text-sm text-muted-foreground">选择测点后查看业务通道。</p>
              )}
            </div>
          )}
          {showList && (
            <DirectoryTable
              label={listLabel}
              items={list.items}
              rowId={(r) => r.id}
              loading={list.isPending}
              error={list.error?.message}
              retry={() => void list.refetch()}
              empty={listKind === 'channel' ? '暂无业务通道' : `暂无${labels[listKind]}`}
              toolbar={
                <>
                  <DirectorySearch
                    label={`搜索${labels[listKind]}名称或编号`}
                    value={filter.q}
                    onChange={(q) => setFilter({ ...filter, q })}
                  />
                  <ListFilter
                    label="资源状态"
                    caption="状态"
                    value={filter.status === 'all' ? '' : filter.status}
                    onChange={(status) => setFilter({ ...filter, status: status || 'all' })}
                    options={[
                      { value: '', label: '全部状态' },
                      { value: 'normal', label: '正常' },
                      { value: 'retired', label: '已退役' },
                    ]}
                  />
                  {(filter.q || filter.status !== 'all') && (
                    <DirectoryReset onClick={() => setFilter({ q: '', status: 'all' })} />
                  )}
                </>
              }
              columns={[
                {
                  id: 'name',
                  label: labels[listKind],
                  cell: (r) => (
                    <div className="space-y-1">
                      <Button
                        variant="link"
                        className="h-auto max-w-full justify-start whitespace-normal p-0 text-left text-foreground"
                        onClick={() =>
                          choose({
                            ...selection,
                            tab,
                            component: listKind === 'component' ? r.id : selection.component,
                            point: listKind === 'point' ? r.id : selection.point,
                            channel: listKind === 'channel' ? r.id : undefined,
                          })
                        }
                      >
                        {r.name}
                      </Button>
                      <p className="break-all text-xs text-muted-foreground">
                        {r.code}
                        {r.visibility === 'context' ? ' · 仅用于定位已授权资源' : ''}
                      </p>
                    </div>
                  ),
                },
                {
                  id: 'status',
                  label: '状态',
                  className: 'w-24 lg:w-32',
                  cell: (r) => (
                    <Badge variant="outline" className="font-normal">
                      {r.status === 'normal' ? '正常' : '已退役'}
                    </Badge>
                  ),
                },
                {
                  id: 'actions',
                  label: '操作',
                  className: 'w-14 text-right',
                  cell: (r) => (
                    <DirectoryActions
                      name={r.name}
                      actions={[
                        {
                          label: `查看${labels[listKind]}`,
                          run: () =>
                            choose({
                              ...selection,
                              tab,
                              component: listKind === 'component' ? r.id : selection.component,
                              point: listKind === 'point' ? r.id : selection.point,
                              channel: listKind === 'channel' ? r.id : undefined,
                            }),
                        },
                      ]}
                    />
                  ),
                },
              ]}
              pagination={
                <ListPagination
                  label={listLabel}
                  count={list.items.length}
                  page={list.page}
                  pageCount={list.pageCount}
                  hasNext={list.hasNext}
                  fetching={list.isFetching}
                  go={list.go}
                  next={() => void list.next()}
                  pageSize={pageSize}
                  setPageSize={setPageSize}
                />
              }
            />
          )}
          {showObject && (
            <div className="space-y-4">
              <Button
                variant="ghost"
                className="-ml-3"
                onClick={() =>
                  choose(
                    tab === 'components'
                      ? { tab }
                      : tab === 'points'
                        ? { tab, component: selection.component }
                        : { ...selection, channel: undefined },
                  )
                }
              >
                <ArrowLeft aria-hidden="true" />
                返回{tab === 'components' ? '构件' : tab === 'points' ? '测点' : '通道'}列表
              </Button>
              {object.isPending && <p role="status">正在读取资料…</p>}
              {object.data && !object.error ? (
                <section className="space-y-5 rounded-lg border p-5">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <h2 className="break-words text-lg font-semibold">
                      {labels[detailKind]}：{object.data.name}
                    </h2>
                    {actions(object.data)}
                  </div>
                  <ResourceInfo resource={object.data} />
                  <div className="flex flex-wrap gap-2">
                    {detailKind === 'component' && (
                      <>
                        {create('component', object.data)}
                        {create('point', object.data)}
                        <Button
                          variant="outline"
                          onClick={() => choose({ tab: 'points', component: selection.component })}
                        >
                          查看构件测点
                        </Button>
                      </>
                    )}
                    {detailKind === 'point' && (
                      <>
                        {create('channel', object.data)}
                        <Button
                          variant="outline"
                          onClick={() => choose({ ...selection, tab: 'channels', channel: undefined })}
                        >
                          查看业务通道
                        </Button>
                      </>
                    )}
                  </div>
                </section>
              ) : contextFailure(object) ? (
                <section className="space-y-4 rounded-lg border p-5">
                  <h2 className="font-semibold">{detailKind === 'component' ? '构件定位' : '测点定位'}</h2>
                  <p className="text-sm text-muted-foreground">
                    仅用于定位已授权资源；未授予完整{labels[detailKind]}资料。
                  </p>
                  <Button
                    variant="outline"
                    onClick={() =>
                      choose({
                        ...selection,
                        tab: detailKind === 'component' ? 'points' : 'channels',
                        channel: undefined,
                      })
                    }
                  >
                    查看已授权{detailKind === 'component' ? '测点' : '通道'}
                  </Button>
                </section>
              ) : null}
            </div>
          )}
        </>
      )}
      <ResponsivePanel
        open={!!panel}
        onOpenChange={(open) => {
          if (!open) setPanel(undefined);
        }}
        title={panel ? `${modes[panel.mode]}${labels[panel.target.kind]}` : ''}
        description="填写当前对象资料，保存后返回详情。"
        returnFocus={returnFocus}
        fallbackFocusSelector="#resource-workspace-anchor"
      >
        {panel &&
          (panel.mode === 'create' || panel.mode === 'edit' ? (
            <ResourceEditor
              key={`${panel.mode}:${panel.target.kind}:${panel.target.id ?? 'new'}`}
              blocked={blocked}
              target={panel.target}
              initial={panel.resource}
              facility={facility.data}
              owner={panel.owner}
              done={(result) => {
                const saved = panel;
                setPanel(undefined);
                void facility.refetch();
                if (saved.target.kind === 'component') choose({ component: result.id, tab: 'components' });
                if (saved.target.kind === 'point')
                  choose({
                    component: saved.owner?.kind === 'component' ? saved.owner.id : selection.component,
                    point: result.id,
                    tab: 'points',
                  });
                if (saved.target.kind === 'channel')
                  choose({ ...selection, point: saved.target.point, channel: result.id, tab: 'channels' });
              }}
            />
          ) : (
            <LifecycleForm
              target={panel.target}
              resource={panel.resource!}
              structure={facility.data?.structure_revision}
              action={panel.mode}
              done={() => {
                const saved = panel;
                setPanel(undefined);
                if (saved.mode === 'delete') {
                  if (saved.target.kind === 'facility') void navigate({ to: paths.list });
                  else if (saved.target.kind === 'channel')
                    choose({ ...selection, channel: undefined, tab: 'channels' });
                  else choose({ tab: saved.target.kind === 'component' ? 'components' : 'points' });
                }
              }}
            />
          ))}
      </ResponsivePanel>
    </section>
  );
}
function ObjectActions({
  resource,
  disabled,
  choose,
}: {
  resource: ResourceDetail;
  disabled: boolean;
  choose: (mode: 'edit' | 'retire' | 'restore' | 'delete', trigger: HTMLElement | null) => void;
}) {
  const trigger = useRef<HTMLButtonElement>(null);
  const pendingMode = useRef<'edit' | 'retire' | 'restore' | 'delete' | null>(null);
  if (!['edit', 'retire', 'restore', 'delete'].some((k) => resource.actions[k as 'edit'])) return null;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          ref={trigger}
          id={`resource-actions-${resource.id}`}
          variant="outline"
          size="icon"
          aria-label={`${resource.name} 操作`}
          disabled={disabled}
        >
          <MoreHorizontal aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        onCloseAutoFocus={(event) => {
          const mode = pendingMode.current;
          pendingMode.current = null;
          if (!mode) return;
          // Finish menu teardown before the panel takes focus; exit pointer events can still refocus the menu.
          event.preventDefault();
          choose(mode, trigger.current);
        }}
      >
        {(['edit', 'retire', 'restore', 'delete'] as const)
          .filter((k) => resource.actions[k])
          .map((mode) => (
            <DropdownMenuItem
              key={mode}
              onSelect={() => {
                pendingMode.current = mode;
              }}
            >
              {modes[mode]}
              {labels[resource.kind]}
            </DropdownMenuItem>
          ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
function ResourceInfo({ resource: r }: { resource: ResourceDetail }) {
  const type = r.definitions.find((v) => v.kind === 'type-definitions');
  const fields =
    r.kind === 'facility'
      ? type?.content.fields
      : type?.content.components?.find((c) => c.key === r.category_key)?.fields;
  const metric = r.definitions.find((v) => v.id === r.metric_version_id);
  const unit = r.definitions.find((v) => v.id === r.unit_version_id);
  return (
    <div className="min-w-0 space-y-4 text-sm">
      <p className="break-words text-muted-foreground">
        归属路径：{[...r.ancestors.map((a) => a.name), r.name].join(' / ')}
      </p>
      <p className="break-all">
        编号：{r.code} · {r.status === 'normal' ? '正常' : '已退役'}
      </p>
      {r.actions.reason && <p className="text-muted-foreground">{r.actions.reason}</p>}
      {!!fields?.length && (
        <dl className="grid gap-3 sm:grid-cols-2">
          {fields.map((f) => (
            <div key={f.key} className="min-w-0">
              <dt className="text-muted-foreground">{f.name}</dt>
              <dd className="break-words">
                {r.values?.[f.key] === undefined
                  ? '未填写'
                  : typeof r.values[f.key] === 'boolean'
                    ? r.values[f.key]
                      ? '是'
                      : '否'
                    : Array.isArray(r.values[f.key])
                      ? (r.values[f.key] as unknown[]).join('、')
                      : String(r.values[f.key])}
                {f.unit_version && ` ${r.definitions.find((v) => v.id === f.unit_version)?.content.unit?.symbol ?? ''}`}
              </dd>
            </div>
          ))}
        </dl>
      )}
      {(
        [
          ['业务说明', r.description],
          ['监测目的', r.purpose],
          ['方向说明', r.direction_description],
          ['参考基准', r.reference_description],
          ['资料来源', r.source_reference],
        ] as const
      )
        .filter(([, v]) => v)
        .map(([label, value]) => (
          <div key={label}>
            <p className="text-muted-foreground">{label}</p>
            <p className="whitespace-pre-wrap break-words">{value}</p>
          </div>
        ))}
      {r.kind !== 'channel' ? (
        <details>
          <summary className="cursor-pointer font-medium">本对象位置资料</summary>
          <div className="mt-3">
            <PositionView position={r.position} />
          </div>
        </details>
      ) : (
        <div className="space-y-2">
          <p>
            指标：{metric?.content.name} · V{metric?.number} ·{' '}
            {metric?.content.metric?.shape === 'enum'
              ? '枚举'
              : metric?.content.metric?.shape === 'integer'
                ? '整数'
                : '数值'}
          </p>
          <p>固定单位：{unit ? `${unit.content.name}（${unit.content.unit?.symbol}）V${unit.number}` : '无单位'}</p>
          {metric?.content.metric?.shape === 'enum' && (
            <p>选项：{metric.content.metric.options.map((o) => `${o.name}（${o.key}）`).join('、')}</p>
          )}
          <p>时间含义：{r.timestamp_meaning === 'interval_end' ? '统计区间结束时刻' : '观测时刻'}</p>
          {r.aggregation_description && <p className="break-words">统计含义：{r.aggregation_description}</p>}
          <p className="break-words">
            观测方向：{r.observation_direction || '未填写'}；观测基准：{r.reference_basis || '未填写'}
          </p>
          <p>
            计划采样：
            {r.sampling
              ? `${{ periodic: '周期', event: '事件', manual: '人工' }[r.sampling.mode]}${r.sampling.mode === 'periodic' ? ` · ${r.sampling.interval_seconds} 秒` : ''}`
              : '未填写'}
          </p>
          {r.sampling?.description && <p className="break-words">{r.sampling.description}</p>}
          <p className="text-muted-foreground">尚无数据来源</p>
        </div>
      )}
      {r.kind !== 'facility' && r.ancestors[0] && (
        <details>
          <summary className="cursor-pointer text-muted-foreground">所属设施的位置参照（不代表本对象坐标）</summary>
          <div className="mt-3">
            <PositionView position={'position' in r.ancestors[0] ? r.ancestors[0].position : undefined} />
          </div>
        </details>
      )}
      <p className="text-xs text-muted-foreground">更新于 {new Date(r.updated_at).toLocaleString()}</p>
    </div>
  );
}
