import { useState } from 'react';
import { Link } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { ArrowLeft, Plus } from 'lucide-react';
import { PageHeader } from '@infrapro/layouts';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { Tabs, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { DirectoryTable, DirectorySearch } from '@infrapro/ui/patterns/directory-table';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
import { CheckControl } from '../engineering-fields/controls';
import { useCursorList } from '../../hooks/use-cursor-list';
import { AccessCommit, useResourceAccess } from './shared';
import type { ResourceAccessDetail } from '@infrapro/api-client';

export function ResourceAccessProjects({ facility }: { facility?: string }) {
  return facility ? <FacilityUsage facility={facility} /> : <ProjectPicker />;
}

export function FacilityUsage({ facility }: { facility: string }) {
  const { client, tenant, canRead, canWrite, prefix } = useResourceAccess();
  const [adding, setAdding] = useState(false);
  const [pageSize, setPageSize] = useState(20);
  const state = useCursorList({
    queryKey: [...prefix, 'usage', facility, pageSize],
    load: (cursor, signal) => client.resourceUsage(tenant.tenant.id, facility, { cursor, limit: pageSize }, signal),
    enabled: canRead,
  });
  if (!canRead) return <ErrorState message="需要租户资源查看、项目治理查看及资源授权查看权限。" />;
  return (
    <section className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="space-y-1">
          <h2 className="text-lg font-semibold">设施项目使用</h2>
          <p className="text-sm text-muted-foreground">当前关联本设施的项目，包含已归档项目。</p>
        </div>
        {canWrite && (
          <Button onClick={() => setAdding(true)}>
            <Plus aria-hidden="true" />
            为其他项目配置授权
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
        empty="当前没有项目使用此设施。"
        columns={[
          {
            id: 'name',
            label: '项目',
            cell: (p) => (
              <div className="space-y-1">
                <Button
                  asChild
                  variant="link"
                  className="h-auto max-w-full whitespace-normal p-0 text-left text-foreground"
                >
                  <Link
                    to="/t/$tenantId/resource-access/$projectId/facilities/$facilityId"
                    params={{ tenantId: tenant.tenant.id, projectId: p.project_id, facilityId: facility }}
                  >
                    {p.name}
                  </Link>
                </Button>
                <p className="break-all text-xs text-muted-foreground">{p.code}</p>
              </div>
            ),
          },
          {
            id: 'status',
            label: '状态',
            className: 'w-24',
            cell: (p) => (
              <Badge variant="outline" className="font-normal">
                {p.status === 'archived' ? '已归档' : '运行中'}
              </Badge>
            ),
          },
          {
            id: 'open',
            label: '授权范围',
            secondary: true,
            className: 'w-36',
            cell: (p) => (
              <Button asChild variant="ghost" size="sm">
                <Link
                  to="/t/$tenantId/resource-access/$projectId/facilities/$facilityId"
                  params={{ tenantId: tenant.tenant.id, projectId: p.project_id, facilityId: facility }}
                >
                  查看资源授权
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
      <ResponsivePanel
        open={adding}
        onOpenChange={setAdding}
        title="选择项目"
        description="为选中的项目配置本设施的对象范围。"
      >
        {adding && <ProjectPicker facility={facility} />}
      </ResponsivePanel>
    </section>
  );
}

function ProjectPicker({ facility }: { facility?: string }) {
  const { client, tenant, canRead, prefix } = useResourceAccess();
  const [q, setQ] = useState('');
  const [pageSize, setPageSize] = useState(20);
  const state = useCursorList({
    queryKey: [...prefix, 'projects', q, pageSize],
    load: (cursor, signal) =>
      client.projectGovernance(tenant.tenant.id, { q, status: 'all', cursor, limit: pageSize }, signal),
    enabled: canRead,
  });
  if (!canRead) return <ErrorState message="需要租户资源查看、项目治理查看及资源授权查看权限。" />;
  return (
    <section className="space-y-6">
      {!facility && <PageHeader title="项目资源授权" description="按项目管理设施使用范围与资源维护权限。" />}
      <DirectoryTable
        label="项目资源授权"
        items={state.items}
        rowId={({ project }) => project.id}
        loading={state.isPending}
        error={state.error?.message}
        retry={() => void state.refetch()}
        empty="没有匹配的项目。"
        toolbar={<DirectorySearch label="搜索项目名称或编号" value={q} onChange={setQ} />}
        columns={[
          {
            id: 'name',
            label: '项目',
            cell: ({ project: p }) => (
              <div className="space-y-1">
                <Button
                  asChild
                  variant="link"
                  className="h-auto max-w-full whitespace-normal p-0 text-left text-foreground"
                >
                  <Link
                    to={
                      facility
                        ? '/t/$tenantId/resource-access/$projectId/facilities/$facilityId'
                        : '/t/$tenantId/resource-access/$projectId'
                    }
                    params={{ tenantId: tenant.tenant.id, projectId: p.id, facilityId: facility! }}
                  >
                    {p.name}
                  </Link>
                </Button>
                <p className="break-all text-xs text-muted-foreground">{p.code}</p>
              </div>
            ),
          },
          {
            id: 'period',
            label: '计划周期',
            secondary: true,
            cell: ({ project: p }) => [p.planned_start, p.planned_end].filter(Boolean).join(' ～ ') || '—',
          },
          {
            id: 'status',
            label: '状态',
            className: 'w-24 lg:w-32',
            cell: ({ project: p }) => (
              <Badge variant="outline" className="font-normal">
                {p.status === 'archived' ? '已归档' : '运行中'}
              </Badge>
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
    </section>
  );
}

export function ProjectResourceAccessPage({ project }: { project: string }) {
  const { client, tenant, canRead, canWrite, prefix } = useResourceAccess(project);
  const [tab, setTab] = useState('facilities');
  const [adding, setAdding] = useState(false);
  const [q, setQ] = useState('');
  const [pageSize, setPageSize] = useState(20);
  const base = useQuery({
    queryKey: [...prefix, 'policy'],
    queryFn: ({ signal }) => client.resourceAccess(tenant.tenant.id, project, undefined, {}, signal),
    enabled: canRead,
  });
  const state = useCursorList({
    queryKey: [...prefix, 'links', pageSize],
    load: async (cursor, signal) => {
      const result = await client.resourceAccess(
        tenant.tenant.id,
        project,
        undefined,
        { cursor, limit: pageSize },
        signal,
      );
      return { items: result.links, next_cursor: result.next_cursor };
    },
    enabled: canRead && tab === 'facilities',
  });
  const candidates = useCursorList({
    queryKey: [...prefix, 'facilities', q, pageSize],
    load: (cursor, signal) =>
      client.resourceList(
        { kind: 'tenant', tenantId: tenant.tenant.id },
        { kind: 'facility' },
        { q, status: 'all', cursor, limit: pageSize },
        signal,
      ),
    enabled: canRead && adding,
  });
  const history = useCursorList({
    queryKey: [...prefix, 'history', pageSize],
    load: (cursor, signal) =>
      client.resourceAccessChanges(tenant.tenant.id, project, { cursor, limit: pageSize }, signal),
    enabled: canRead && tab === 'history',
  });
  if (!canRead) return <ErrorState message="没有资源授权查看权限。" />;
  const data = base.data;
  return (
    <section className="min-w-0 space-y-5">
      <Button asChild variant="ghost" className="-ml-3">
        <Link
          to="/t/$tenantId/resource-access"
          search={{ facility: undefined }}
          params={{ tenantId: tenant.tenant.id }}
        >
          <ArrowLeft aria-hidden="true" />
          返回项目资源授权
        </Link>
      </Button>
      <PageHeader
        title={data?.policy.project_name ?? '项目资源授权'}
        description={`${tenant.tenant.name} · ${data?.policy.project_status === 'archived' ? '已归档，只允许缩减现有授权' : '运行中'}`}
        actions={
          canWrite &&
          data?.policy.project_status !== 'archived' &&
          tab === 'facilities' && (
            <Button onClick={() => setAdding(true)}>
              <Plus aria-hidden="true" />
              添加设施范围
            </Button>
          )
        }
      />
      {base.error && <ErrorState message={base.error.message} onRetry={() => void base.refetch()} />}
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList className="flex h-auto flex-wrap justify-start">
          <TabsTrigger value="facilities">设施范围</TabsTrigger>
          <TabsTrigger value="policy">新建政策</TabsTrigger>
          <TabsTrigger value="history">操作记录</TabsTrigger>
        </TabsList>
      </Tabs>
      {tab === 'policy' && data && <PolicyEditor key={data.policy.revision} project={project} data={data} />}
      {tab === 'facilities' && (
        <DirectoryTable
          label="已关联设施"
          items={state.items}
          rowId={(link) => link.id}
          loading={state.isPending}
          error={state.error?.message}
          retry={() => void state.refetch()}
          empty="尚无设施授权。"
          columns={[
            {
              id: 'name',
              label: '设施',
              cell: (link) => (
                <div className="space-y-1">
                  <Button
                    asChild
                    variant="link"
                    className="h-auto max-w-full whitespace-normal p-0 text-left text-foreground"
                  >
                    <Link
                      to="/t/$tenantId/resource-access/$projectId/facilities/$facilityId"
                      params={{ tenantId: tenant.tenant.id, projectId: project, facilityId: link.facility_id }}
                    >
                      {link.facility_name}
                    </Link>
                  </Button>
                  <p className="break-all text-xs text-muted-foreground">{link.facility_code}</p>
                </div>
              ),
            },
            {
              id: 'open',
              label: '对象范围',
              className: 'w-32',
              cell: (link) => (
                <Button asChild variant="outline" size="sm">
                  <Link
                    to="/t/$tenantId/resource-access/$projectId/facilities/$facilityId"
                    params={{ tenantId: tenant.tenant.id, projectId: project, facilityId: link.facility_id }}
                  >
                    查看对象范围
                  </Link>
                </Button>
              ),
            },
          ]}
          pagination={
            <ListPagination
              label="已关联设施"
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
      )}
      {tab === 'history' && (
        <DirectoryTable
          label="授权变更记录"
          items={history.items}
          rowId={(c) => c.id}
          loading={history.isPending}
          error={history.error?.message}
          retry={() => void history.refetch()}
          columns={[
            {
              id: 'reason',
              label: '变更说明',
              cell: (c) => (
                <div className="space-y-2">
                  <p>{c.reason}</p>
                  <p className="text-xs text-muted-foreground lg:hidden">{new Date(c.changed_at).toLocaleString()}</p>
                  <details>
                    <summary className="cursor-pointer text-sm text-muted-foreground">查看变更详情</summary>
                    <pre className="mt-3 max-h-64 overflow-auto whitespace-pre-wrap break-all text-xs">
                      {JSON.stringify({ before: c.before, after: c.after }, null, 2)}
                    </pre>
                  </details>
                </div>
              ),
            },
            { id: 'time', label: '变更时间', secondary: true, cell: (c) => new Date(c.changed_at).toLocaleString() },
            { id: 'revision', label: '修订', className: 'w-20', cell: (c) => c.access_revision },
          ]}
          pagination={
            <ListPagination
              label="授权变更记录"
              count={history.items.length}
              page={history.page}
              pageCount={history.pageCount}
              hasNext={history.hasNext}
              fetching={history.isFetching}
              go={history.go}
              next={() => void history.next()}
              pageSize={pageSize}
              setPageSize={setPageSize}
            />
          }
        />
      )}
      <ResponsivePanel
        open={adding}
        onOpenChange={setAdding}
        title="添加设施范围"
        description="选择设施后，指定项目可用的对象和维护能力。"
      >
        {adding && (
          <DirectoryTable
            label="选择设施"
            items={candidates.items}
            rowId={(f) => f.id}
            loading={candidates.isPending}
            error={candidates.error?.message}
            retry={() => void candidates.refetch()}
            toolbar={<DirectorySearch label="搜索统一设施" value={q} onChange={setQ} />}
            columns={[
              {
                id: 'name',
                label: '设施',
                cell: (f) => (
                  <div className="space-y-1">
                    <Button
                      asChild
                      variant="link"
                      className="h-auto max-w-full whitespace-normal p-0 text-left text-foreground"
                    >
                      <Link
                        to="/t/$tenantId/resource-access/$projectId/facilities/$facilityId"
                        params={{ tenantId: tenant.tenant.id, projectId: project, facilityId: f.id }}
                      >
                        {f.name}
                      </Link>
                    </Button>
                    <p className="break-all text-xs text-muted-foreground">{f.code}</p>
                  </div>
                ),
              },
              {
                id: 'status',
                label: '状态',
                className: 'w-24',
                cell: (f) => (f.status === 'normal' ? '正常' : '已退役'),
              },
            ]}
            pagination={
              <ListPagination
                label="选择设施"
                count={candidates.items.length}
                page={candidates.page}
                pageCount={candidates.pageCount}
                hasNext={candidates.hasNext}
                fetching={candidates.isFetching}
                go={candidates.go}
                next={() => void candidates.next()}
                pageSize={pageSize}
                setPageSize={setPageSize}
              />
            }
          />
        )}
      </ResponsivePanel>
    </section>
  );
}
function PolicyEditor({ project, data }: { project: string; data: ResourceAccessDetail }) {
  const { canWrite } = useResourceAccess(project);
  const [allow, setAllow] = useState(data.policy.allow_facility_create);
  const [editing, setEditing] = useState(false);
  return (
    <section className="space-y-3 rounded-xl border p-4">
      <h2 className="font-semibold">新建设施政策</h2>
      <p>{data.policy.allow_facility_create ? '已允许新建设施并维护新建资源' : '未允许新建设施'}</p>
      <p className="text-sm text-muted-foreground">
        关闭政策会保留已有授权。新建对象自动在本项目内授予标准维护能力；其他项目须单独授权。
      </p>
      {canWrite && (
        <Button type="button" variant="outline" onClick={() => setEditing(!editing)}>
          {editing ? '取消政策编辑' : '修改新建政策'}
        </Button>
      )}
      {editing && (
        <>
          <fieldset disabled={data.policy.project_status === 'archived' && !data.policy.allow_facility_create}>
            <CheckControl label="允许新建设施并维护新建资源" value={allow} onChange={setAllow} />
          </fieldset>
          <AccessCommit
            draftKey={String(allow)}
            project={project}
            disabled={allow === data.policy.allow_facility_create}
            build={(reason, request_id) => ({
              action: 'policy',
              request_id,
              reason,
              expected_access_revision: data.policy.revision,
              allow_facility_create: allow,
            })}
            onSaved={() => setEditing(false)}
          />
        </>
      )}
    </section>
  );
}
