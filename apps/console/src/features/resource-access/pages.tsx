import { ObjectHistory } from '../audit/object-history';
import { useRef, useState } from 'react';
import { Link, useBlocker } from '@tanstack/react-router';
import { useQueries, useQuery } from '@tanstack/react-query';
import { ArrowLeft, ArrowRight, Building2, CircleHelp, Eye, Pencil, Plus } from 'lucide-react';
import { PageHeader } from '@infrapro/layouts';
import type { ResourceAccessDetail } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { Tabs, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { DirectoryTable, DirectorySearch } from '@infrapro/ui/patterns/directory-table';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@infrapro/ui/components/dialog';
import { cn } from '@infrapro/ui/lib/utils';
import { useCursorList } from '../../hooks/use-cursor-list';
import { AccessCommit, useResourceAccess } from './shared';
import { AccessSteps, Hint, ResourceCounts } from './presentation';
import { AccessHistory } from './history';

export function ResourceAccessProjects({ facility }: { facility?: string }) {
  const { tenant } = useResourceAccess();
  return facility ? (
    <section className="space-y-5">
      <Button asChild variant="ghost" className="-ml-3">
        <Link
          to="/t/$tenantId/resource-access"
          params={{ tenantId: tenant.tenant.id }}
          search={{ facility: undefined }}
        >
          <ArrowLeft aria-hidden="true" />
          返回项目资源授权
        </Link>
      </Button>
      <FacilityUsage facility={facility} />
    </section>
  ) : (
    <ProjectPicker />
  );
}

export function FacilityUsage({ facility }: { facility: string }) {
  const { client, tenant, canRead, canWrite, prefix } = useResourceAccess();
  const [adding, setAdding] = useState(false);
  const [pageSize, setPageSize] = useState(20);
  const state = useCursorList({
    queryKey: [...prefix, 'usage', facility, pageSize],
    load: (cursor, signal) =>
      client.resourceUsage(tenant.tenant.id, facility, { cursor, limit: pageSize }, signal),
    enabled: canRead,
  });
  if (!canRead) return <ErrorState message="需要租户资源查看、项目治理查看及资源授权查看权限。" />;
  return (
    <section className="min-w-0 space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">设施项目使用</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            这些项目使用同一座设施，资源范围和操作权限分别配置。
          </p>
        </div>
        {canWrite && (
          <Button onClick={() => setAdding(true)}>
            <Plus aria-hidden="true" />
            授权给其他项目
          </Button>
        )}
      </div>
      <DirectoryTable
        label="使用项目"
        items={state.items}
        rowId={(p) => p.link_id}
        loading={state.isPending}
        error={state.error?.message}
        retry={() => void state.refetch()}
        empty="还没有项目获准使用这座设施。"
        columns={[
          {
            id: 'name',
            label: '项目',
            cell: (p) => (
              <div className="space-y-1">
                <Link
                  className="font-medium hover:underline"
                  to="/t/$tenantId/resource-access/$projectId/facilities/$facilityId"
                  params={{ tenantId: tenant.tenant.id, projectId: p.project_id, facilityId: facility }}
                >
                  {p.name}
                </Link>
                <p className="text-xs text-muted-foreground">
                  {p.code} · {p.status === 'archived' ? '已归档' : '运行中'}
                </p>
              </div>
            ),
          },
          {
            id: 'actions',
            label: '操作',
            className: 'w-32 sm:w-44',
            cell: (p) => (
              <Button asChild variant="outline" size="sm" className="h-auto min-h-9 whitespace-normal">
                <Link
                  to="/t/$tenantId/resource-access/$projectId/facilities/$facilityId"
                  params={{ tenantId: tenant.tenant.id, projectId: p.project_id, facilityId: facility }}
                >
                  管理资源与权限
                </Link>
              </Button>
            ),
          },
        ]}
        pagination={
          <ListPagination
            label="使用项目"
            count={state.items.length}
            page={state.page}
            pageCount={state.pageCount}
            hasNext={state.hasNext}
            fetching={state.isFetching}
            go={state.go}
            next={() => void state.next()}
            pageSize={pageSize}
            setPageSize={setPageSize}
          />
        }
      />
      <Hint>收回某个项目的授权不会删除设施，也不会影响其他项目的授权。</Hint>
      <ResponsivePanel
        open={adding}
        onOpenChange={setAdding}
        title="选择要授权的项目"
        description="选择项目后，继续配置这座设施的资源范围和操作权限。"
      >
        {adding && <ProjectPicker facility={facility} />}
      </ResponsivePanel>
    </section>
  );
}

function ProjectPicker({ facility }: { facility?: string }) {
  const { client, tenant, canRead, prefix } = useResourceAccess();
  const [q, setQ] = useState('');
  const [status, setStatus] = useState<'all' | 'active' | 'archived'>('all');
  const [pageSize, setPageSize] = useState(20);
  const state = useCursorList({
    queryKey: [...prefix, 'projects', q, status, pageSize],
    load: (cursor, signal) =>
      client.projectGovernance(tenant.tenant.id, { q, status, cursor, limit: pageSize }, signal),
    enabled: canRead,
  });
  const summaries = useQueries({
    queries: state.items.map(({ project }) => ({
      queryKey: [...prefix.slice(0, -1), project.id, 'overview'],
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        client.resourceAccess(tenant.tenant.id, project.id, undefined, { limit: 1 }, signal),
      enabled: canRead && !facility,
      staleTime: 30_000,
    })),
  });
  const summaryFor = (id: string) => summaries[state.items.findIndex((v) => v.project.id === id)];
  if (!canRead) return <ErrorState message="需要租户资源查看、项目治理查看及资源授权查看权限。" />;
  return (
    <section className="min-w-0 space-y-6">
      {!facility && (
        <>
          <PageHeader
            title="项目资源授权"
            description="选择项目，配置它可以使用哪些设施和资源、允许做哪些操作。"
          />
          <AccessSteps />
        </>
      )}
      <DirectoryTable
        label="项目资源授权"
        items={state.items}
        rowId={(v) => v.project.id}
        loading={state.isPending}
        error={state.error?.message}
        retry={() => void state.refetch()}
        empty="没有匹配的项目。"
        toolbar={
          <>
            <DirectorySearch label="搜索项目名称或编号" value={q} onChange={setQ} />
            <label className="flex items-center gap-2 text-sm text-muted-foreground sm:ml-auto">
              项目状态
              <select
                aria-label="项目状态"
                className="h-10 rounded-md border bg-background px-3 text-foreground"
                value={status}
                onChange={(e) => setStatus(e.target.value as typeof status)}
              >
                <option value="all">全部状态</option>
                <option value="active">运行中</option>
                <option value="archived">已归档</option>
              </select>
            </label>
          </>
        }
        columns={[
          {
            id: 'name',
            label: '项目',
            cell: ({ project: p }) => (
              <div className="space-y-1">
                <Link
                  className="font-medium hover:underline"
                  to={
                    facility
                      ? '/t/$tenantId/resource-access/$projectId/facilities/$facilityId'
                      : '/t/$tenantId/resource-access/$projectId'
                  }
                  params={{ tenantId: tenant.tenant.id, projectId: p.id, facilityId: facility! }}
                >
                  {p.name}
                </Link>
                <p className="text-xs text-muted-foreground">{p.code}</p>
                <p className="text-xs text-muted-foreground lg:hidden">
                  {p.status === 'archived' ? '已归档' : '运行中'}
                  {summaryFor(p.id)?.data &&
                    ` · ${summaryFor(p.id)!.data!.summary.facilities} 座设施 · ${summaryFor(p.id)!.data!.policy.allow_facility_create ? '允许新建' : '不允许新建'}`}
                </p>
              </div>
            ),
          },
          ...(!facility
            ? [
                {
                  id: 'facilities',
                  label: '已授权设施',
                  secondary: true,
                  cell: ({ project: p }: (typeof state.items)[number]) => {
                    const v = summaryFor(p.id);
                    return v?.data ? (
                      `${v.data.summary.facilities} 座设施`
                    ) : v?.error ? (
                      <Button variant="link" size="sm" onClick={() => void v.refetch()}>
                        读取失败，重试
                      </Button>
                    ) : (
                      '读取中…'
                    );
                  },
                },
                {
                  id: 'creation',
                  label: '新建设施权限',
                  secondary: true,
                  cell: ({ project: p }: (typeof state.items)[number]) =>
                    summaryFor(p.id)?.data?.policy.allow_facility_create
                      ? '允许新建'
                      : summaryFor(p.id)?.data
                        ? '不允许新建'
                        : '—',
                },
              ]
            : []),
          {
            id: 'status',
            label: '项目状态',
            secondary: true,
            cell: ({ project: p }) => (
              <Badge variant="outline" className="font-normal">
                {p.status === 'archived' ? '已归档' : '运行中'}
              </Badge>
            ),
          },
          {
            id: 'actions',
            label: '操作',
            className: 'w-28 sm:w-36',
            cell: ({ project: p }) => (
              <Button asChild variant="outline" size="sm">
                <Link
                  to={
                    facility
                      ? '/t/$tenantId/resource-access/$projectId/facilities/$facilityId'
                      : '/t/$tenantId/resource-access/$projectId'
                  }
                  params={{ tenantId: tenant.tenant.id, projectId: p.id, facilityId: facility! }}
                >
                  管理授权
                  <ArrowRight aria-hidden="true" />
                </Link>
              </Button>
            ),
          },
        ]}
        pagination={
          <ListPagination
            label="项目资源授权"
            count={state.items.length}
            page={state.page}
            pageCount={state.pageCount}
            hasNext={state.hasNext}
            fetching={state.isFetching}
            go={state.go}
            next={() => void state.next()}
            pageSize={pageSize}
            setPageSize={setPageSize}
          />
        }
      />
      {!facility && (
        <Hint>授权给项目后，成员仍需对应的项目角色权限。项目共同使用租户中的同一份资源档案。</Hint>
      )}
    </section>
  );
}

export function ProjectResourceAccessPage({ project }: { project: string }) {
  const { client, tenant, canRead, canWrite, prefix } = useResourceAccess(project);
  const [tab, setTab] = useState('facilities');
  const [policyDirty, setPolicyDirty] = useState(false);
  const [pendingTab, setPendingTab] = useState<string>();
  const changeTab = (next: string) => {
    if (next === tab) return;
    if (tab === 'policy' && policyDirty) setPendingTab(next);
    else setTab(next);
  };
  const [adding, setAdding] = useState(false);
  const [q, setQ] = useState('');
  const [pageSize, setPageSize] = useState(20);
  const base = useQuery({
    queryKey: [...prefix, 'overview'],
    queryFn: ({ signal }) =>
      client.resourceAccess(tenant.tenant.id, project, undefined, { limit: 1 }, signal),
    enabled: canRead,
  });
  const state = useCursorList({
    queryKey: [...prefix, 'links', q, pageSize],
    load: async (cursor, signal) => {
      const v = await client.resourceAccess(
        tenant.tenant.id,
        project,
        undefined,
        { q, cursor, limit: pageSize },
        signal,
      );
      return { items: v.links, next_cursor: v.next_cursor };
    },
    enabled: canRead && tab === 'facilities',
  });
  if (!canRead) return <ErrorState message="没有资源授权查看权限。" />;
  const data = base.data;
  const archived = data?.policy.project_status === 'archived';
  return (
    <section className="min-w-0 space-y-6">
      <Button asChild variant="ghost" className="-ml-3">
        <Link
          to="/t/$tenantId/resource-access"
          params={{ tenantId: tenant.tenant.id }}
          search={{ facility: undefined }}
        >
          <ArrowLeft aria-hidden="true" />
          返回项目资源授权
        </Link>
      </Button>
      <PageHeader
        title={data?.policy.project_name ?? '项目资源授权'}
        description={`${tenant.tenant.name} · ${archived ? '已归档' : '运行中'}`}
      />
      <ObjectHistory objectType="resource_access" objectId={project}/>
      {base.error && <ErrorState message={base.error.message} onRetry={() => void base.refetch()} />}
      {base.isPending && <LoadingState label="正在读取项目授权…" />}
      {data && (
        <>
          <div className="flex flex-wrap gap-x-8 gap-y-2 rounded-lg border bg-muted/20 px-5 py-4 text-sm">
            <span>
              已授权 <strong>{data.summary.facilities}</strong> 座设施
            </span>
            <span>新建设施：{data.policy.allow_facility_create ? '允许' : '不允许'}</span>
            <span className="text-muted-foreground">资源范围和操作权限按设施分别设置</span>
          </div>
          {archived && <Hint>项目已归档，现有资源仍可查看。你可以收回授权，不能新增资源或扩大权限。</Hint>}
        </>
      )}
      <Tabs value={tab} onValueChange={changeTab}>
        <TabsList className="flex h-auto flex-wrap justify-start">
          <TabsTrigger value="facilities">已授权设施</TabsTrigger>
          <TabsTrigger value="policy">新建设施权限</TabsTrigger>
          <TabsTrigger value="history">授权记录</TabsTrigger>
        </TabsList>
      </Tabs>
      {tab === 'facilities' && (
        <>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h2 className="font-semibold">项目可使用的设施</h2>
              <p className="mt-1 text-sm text-muted-foreground">
                选择一座设施，管理它向项目开放的资源和操作权限。
              </p>
            </div>
            {canWrite && !archived && data && (
              <Button onClick={() => setAdding(true)}>
                <Plus aria-hidden="true" />
                添加设施到项目
              </Button>
            )}
          </div>
          <DirectoryTable
            label="已授权设施"
            items={state.items}
            rowId={(v) => v.id}
            loading={state.isPending}
            error={state.error?.message}
            retry={() => void state.refetch()}
            empty={
              q
                ? '没有匹配的设施。'
                : '还没有向这个项目开放设施。点击“添加设施到项目”，从租户目录选择第一座设施。'
            }
            toolbar={<DirectorySearch label="搜索已授权设施" value={q} onChange={setQ} />}
            columns={[
              {
                id: 'name',
                label: '设施',
                cell: (f) => (
                  <div className="flex items-start gap-3">
                    <span className="hidden rounded-lg border bg-muted/30 p-2.5 xl:block">
                      <Building2 className="size-5 text-muted-foreground" aria-hidden="true" />
                    </span>
                    <div className="min-w-0 space-y-1">
                      <Link
                        className="font-medium hover:underline"
                        to="/t/$tenantId/resource-access/$projectId/facilities/$facilityId"
                        params={{ tenantId: tenant.tenant.id, projectId: project, facilityId: f.facility_id }}
                      >
                        {f.facility_name}
                      </Link>
                      <p className="break-all text-xs text-muted-foreground">{f.facility_code}</p>
                      {f.summary && (
                        <p className="text-xs text-muted-foreground lg:hidden">
                          <ResourceCounts value={f.summary} /> ·{' '}
                          {f.summary.maintainable ? '部分可维护' : '仅查看'}
                        </p>
                      )}
                    </div>
                  </div>
                ),
              },
              {
                id: 'resources',
                label: '已开放的资源',
                secondary: true,
                cell: (f) => (
                  <div className="space-y-1 text-sm">
                    {f.summary && <ResourceCounts value={f.summary} />}
                    <p className="text-xs text-muted-foreground">含设施基本档案 · 下级按清单开放</p>
                  </div>
                ),
              },
              {
                id: 'rights',
                label: '操作权限',
                secondary: true,
                className: 'w-32',
                cell: (f) => (
                  <Badge variant="secondary" className="font-normal">
                    {f.summary?.maintainable ? (
                      <>
                        <Pencil aria-hidden="true" />
                        部分可维护
                      </>
                    ) : (
                      <>
                        <Eye aria-hidden="true" />
                        仅查看
                      </>
                    )}
                  </Badge>
                ),
              },
              {
                id: 'actions',
                label: '操作',
                className: 'w-32 sm:w-44',
                cell: (f) => (
                  <Button asChild variant="outline" size="sm" className="h-auto min-h-9 whitespace-normal">
                    <Link
                      to="/t/$tenantId/resource-access/$projectId/facilities/$facilityId"
                      params={{ tenantId: tenant.tenant.id, projectId: project, facilityId: f.facility_id }}
                    >
                      管理资源与权限
                    </Link>
                  </Button>
                ),
              },
            ]}
            pagination={
              <ListPagination
                label="已授权设施"
                count={state.items.length}
                page={state.page}
                pageCount={state.pageCount}
                hasNext={state.hasNext}
                fetching={state.isFetching}
                go={state.go}
                next={() => void state.next()}
                pageSize={pageSize}
                setPageSize={setPageSize}
              />
            }
          />
          <Hint>
            勾选父级不会自动开放下级资源。项目人员修改获准维护的资料时，其他使用同一资源的项目也会看到变化。
          </Hint>
        </>
      )}
      {tab === 'policy' && data && (
        <PolicyEditor
          key={data.policy.revision}
          project={project}
          data={data}
          onDirty={setPolicyDirty}
          onFacilities={() => changeTab('facilities')}
        />
      )}
      {tab === 'history' && <AccessHistory project={project} />}
      <Dialog
        open={!!pendingTab}
        onOpenChange={(v) => {
          if (!v) setPendingTab(undefined);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>放弃未保存的权限修改？</DialogTitle>
            <DialogDescription>新建设施权限尚未保存。你可以继续编辑，或放弃后切换页面。</DialogDescription>
          </DialogHeader>
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => setPendingTab(undefined)}>
              继续编辑
            </Button>
            <Button
              onClick={() => {
                if (pendingTab) setTab(pendingTab);
                setPolicyDirty(false);
                setPendingTab(undefined);
              }}
            >
              放弃后离开
            </Button>
          </div>
        </DialogContent>
      </Dialog>
      <ResponsivePanel
        open={adding}
        onOpenChange={setAdding}
        title="添加设施到项目"
        description="先选设施，再勾选需要的资源并配置权限。"
      >
        {adding && <FacilityPicker project={project} />}
      </ResponsivePanel>
    </section>
  );
}

function FacilityPicker({ project }: { project: string }) {
  const { client, tenant, prefix } = useResourceAccess(project);
  const [q, setQ] = useState('');
  const [selected, setSelected] = useState('');
  const [pageSize, setPageSize] = useState(20);
  const list = useCursorList({
    queryKey: [...prefix, 'facilities', q, pageSize],
    load: (cursor, signal) =>
      client.resourceList(
        { kind: 'tenant', tenantId: tenant.tenant.id },
        { kind: 'facility' },
        { q, status: 'all', cursor, limit: pageSize },
        signal,
      ),
  });
  return (
    <div className="space-y-5">
      <DirectorySearch label="搜索设施名称或编号" value={q} onChange={setQ} />
      {list.error && <ErrorState message={list.error.message} onRetry={() => void list.refetch()} />}
      <div className="space-y-2">
        {list.items.map((f) => (
          <label
            key={f.id}
            className={cn(
              'flex cursor-pointer items-start gap-3 rounded-lg border p-4 hover:bg-muted/30',
              selected === f.id && 'border-primary bg-muted/30',
            )}
          >
            <input
              type="radio"
              name="facility-selection"
              checked={selected === f.id}
              onChange={() => setSelected(f.id)}
              className="mt-1 accent-primary"
            />
            <span className="min-w-0">
              <span className="block break-words font-medium">{f.name}</span>
              <span className="mt-1 block text-xs text-muted-foreground">
                {f.code} · {f.status === 'retired' ? '已退役' : '正常'}
              </span>
            </span>
          </label>
        ))}
      </div>
      {list.isPending ? (
        <LoadingState label="正在读取设施…" />
      ) : (
        !list.items.length && (
          <p className="py-6 text-center text-sm text-muted-foreground">没有匹配的设施。</p>
        )
      )}
      <ListPagination
        label="选择设施"
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
      <Hint>
        设施基本档案会一并开放。构件、测点和通道在下一步独立勾选；已加入的设施会打开现有授权供调整。
      </Hint>
      {selected ? (
        <Button asChild className="w-full">
          <Link
            to="/t/$tenantId/resource-access/$projectId/facilities/$facilityId"
            params={{ tenantId: tenant.tenant.id, projectId: project, facilityId: selected }}
          >
            下一步：选择资源
            <ArrowRight aria-hidden="true" />
          </Link>
        </Button>
      ) : (
        <Button disabled className="w-full">
          下一步：选择资源
        </Button>
      )}
    </div>
  );
}

function PolicyEditor({
  project,
  data,
  onFacilities,
  onDirty,
}: {
  project: string;
  data: ResourceAccessDetail;
  onFacilities: () => void;
  onDirty: (dirty: boolean) => void;
}) {
  const { canWrite } = useResourceAccess(project);
  const [allow, setAllow] = useState(data.policy.allow_facility_create);
  const archived = data.policy.project_status === 'archived';
  const completed = useRef(false);
  const dirty = allow !== data.policy.allow_facility_create;
  const blocker = useBlocker({
    shouldBlockFn: () => dirty && !completed.current,
    withResolver: true,
    enableBeforeUnload: dirty,
  });
  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1.7fr)_minmax(250px,1fr)]">
      <section className="space-y-5 rounded-xl border p-5 sm:p-6">
        <div className="flex items-start gap-3">
          <div className="rounded-lg bg-muted p-2.5">
            <Building2 className="size-5" aria-hidden="true" />
          </div>
          <div>
            <h2 className="text-lg font-semibold">允许这个项目新建设施吗？</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              决定项目团队能否自行建立新的桥梁、隧道等设施档案。
            </p>
          </div>
        </div>
        <p className="text-sm">
          当前设置：
          <Badge variant="secondary">
            {data.policy.allow_facility_create ? '允许新建设施' : '不允许新建设施'}
          </Badge>
        </p>
        {[false, true].map((value) => (
          <label
            key={String(value)}
            className={cn(
              'flex items-start gap-3 rounded-lg border p-4',
              allow === value && 'border-primary bg-muted/30',
            )}
          >
            <input
              type="radio"
              name="creation-permission"
              checked={allow === value}
              disabled={!canWrite || (value && archived && !data.policy.allow_facility_create)}
              onChange={() => {
                setAllow(value);
                onDirty(value !== data.policy.allow_facility_create);
              }}
              className="mt-1 accent-primary"
            />
            <span>
              <span className="block text-sm font-medium">
                {value ? '允许项目新建设施，并维护新建资源' : '不允许项目新建设施'}
              </span>
              <span className="mt-1 block text-sm leading-relaxed text-muted-foreground">
                {value
                  ? '新设施归租户统一管理，并自动开放给当前项目；其他项目需单独授权。'
                  : '项目使用租户已经授权的设施。已有资源及其维护权限保留。'}
              </span>
            </span>
          </label>
        ))}
        {archived && <Hint>归档项目只能关闭这项权限，不能开启新建设施。</Hint>}
        {canWrite && (
          <div className="flex justify-end border-t pt-4">
            <AccessCommit
              project={project}
              label="预览权限变更"
              draftKey={String(allow)}
              disabled={allow === data.policy.allow_facility_create}
              build={(reason, request_id) => ({
                action: 'policy',
                reason,
                request_id,
                expected_access_revision: data.policy.revision,
                allow_facility_create: allow,
              })}
              summary={
                <p className="text-sm">
                  新建设施：{data.policy.allow_facility_create ? '允许' : '不允许'} →{' '}
                  {allow ? '允许' : '不允许'}。已有资源授权保留。
                </p>
              }
              onSaved={() => {
                completed.current = true;
                onDirty(false);
              }}
            />
          </div>
        )}
      </section>
      <aside className="space-y-5">
        <div className="rounded-xl border bg-muted/20 p-5">
          <h3 className="flex items-center gap-2 font-medium">
            <CircleHelp className="size-4" aria-hidden="true" />
            已有设施里需要新增测点？
          </h3>
          <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
            到“已授权设施 → 管理资源与权限”，给对应设施或构件勾选“新增测点”。新增通道则在测点上配置。
          </p>
          <Button variant="outline" className="mt-4" onClick={onFacilities}>
            去配置已有设施
          </Button>
        </div>
        <Hint>成员还需要项目角色中的相应权限。这个设置决定项目范围内是否允许新建设施。</Hint>
      </aside>
      <Dialog
        open={blocker.status === 'blocked'}
        onOpenChange={(v) => {
          if (!v && blocker.status === 'blocked') blocker.reset();
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>放弃未保存的权限修改？</DialogTitle>
            <DialogDescription>新建设施权限尚未保存，离开后需要重新设置。</DialogDescription>
          </DialogHeader>
          <div className="flex justify-end gap-2">
            <Button
              variant="outline"
              onClick={() => {
                if (blocker.status === 'blocked') blocker.reset();
              }}
            >
              继续编辑
            </Button>
            <Button
              onClick={() => {
                if (blocker.status === 'blocked') blocker.proceed();
              }}
            >
              放弃后离开
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
