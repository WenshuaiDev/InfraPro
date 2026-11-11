import { useState } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import type { ResourceFilter } from '@infrapro/api-client';
import { Plus } from 'lucide-react';
import { PageHeader } from '@infrapro/layouts';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import {
  DirectoryTable,
  DirectorySearch,
  DirectoryReset,
  DirectoryActions,
} from '@infrapro/ui/patterns/directory-table';
import { ListFilter, ListPagination } from '@infrapro/ui/patterns/list-controls';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { useCursorList } from '../../hooks/use-cursor-list';
import { useResources } from './context';
import { ResourceEditor } from './editor';
export function FacilitiesPage() {
  const scope = useResources();
  if (!scope.canRead) return <ErrorState title="没有工程资源查看权限" message="请联系租户管理员配置相应角色。" />;
  return <FacilitiesBody />;
}
function FacilitiesBody() {
  const { client, scope, paths, prefix, canCreateFacility, available } = useResources();
  const navigate = useNavigate();
  const [filter, setFilter] = useState<ResourceFilter>({ status: 'normal' });
  const [pageSize, setPageSize] = useState(20);
  const [creating, setCreating] = useState(false);
  const [selectedType, setSelectedType] = useState<{ value: string; label: string }>();
  const q = useCursorList({
    queryKey: [...prefix, 'facilities', filter, pageSize],
    load: (cursor, signal) =>
      client.resourceList(scope, { kind: 'facility' }, { ...filter, limit: pageSize, cursor }, signal),
  });
  const items = q.items.filter((r) => r.visibility !== 'context');
  const types = Array.from(
    new Map(
      (q.data?.pages.flatMap((p) => p.items) ?? [])
        .filter((r) => r.visibility !== 'context')
        .filter((r) => r.type_definition_id)
        .map((r) => [r.type_definition_id!, { value: r.type_definition_id!, label: r.type_name! }]),
    ).values(),
  );
  if (selectedType && !types.some((t) => t.value === selectedType.value)) types.unshift(selectedType);
  const title = scope.kind === 'project' ? '项目设施' : '设施管理';
  return (
    <section className="min-w-0 space-y-6">
      <PageHeader
        title={title}
        description={
          scope.kind === 'project'
            ? '查看本项目获准使用的设施，按授权维护构件、测点与通道。'
            : '管理设施档案、构件、测点与业务监测通道。'
        }
        actions={
          canCreateFacility && (
            <Button disabled={!available} id="create-facility" onClick={() => setCreating(true)}>
              <Plus aria-hidden="true" />
              新增设施
            </Button>
          )
        }
      />
      <DirectoryTable
        label="设施列表"
        items={items}
        rowId={(v) => v.id}
        loading={q.isPending}
        error={q.error?.message}
        retry={() => void q.refetch()}
        empty={scope.kind === 'project' ? '当前项目尚未获准使用设施，请联系租户授权人员。' : '暂无设施'}
        toolbar={
          <>
            <DirectorySearch
              label="搜索设施名称或编号"
              value={filter.q ?? ''}
              onChange={(text) => setFilter({ ...filter, q: text })}
            />
            <ListFilter
              label="设施状态"
              caption="状态"
              value={filter.deleted ? 'deleted' : (filter.status ?? 'normal')}
              onChange={(value) =>
                setFilter({
                  ...filter,
                  deleted: value === 'deleted' ? 'only' : undefined,
                  status: value === 'deleted' ? 'all' : (value as ResourceFilter['status']),
                })
              }
              options={[
                { value: 'normal', label: '正常' },
                { value: 'retired', label: '已退役' },
                { value: 'all', label: '全部未删除' },
                ...(scope.kind === 'tenant' ? [{ value: 'deleted', label: '已删除记录' }] : []),
              ]}
            />
            <ListFilter
              label="设施类型（已读取列表）"
              caption="类型"
              value={filter.type_definition_id ?? ''}
              onChange={(value) => {
                setSelectedType(types.find((t) => t.value === value));
                setFilter({ ...filter, type_definition_id: value || undefined });
              }}
              options={[{ value: '', label: '全部类型' }, ...types]}
            />
            {(filter.q || filter.type_definition_id || filter.status !== 'normal' || filter.deleted) && (
              <DirectoryReset
                onClick={() => {
                  setFilter({ status: 'normal' });
                  setSelectedType(undefined);
                }}
              />
            )}
          </>
        }
        columns={[
          {
            id: 'name',
            label: '设施',
            cell: (v) => (
              <div className="space-y-1">
                <Button
                  asChild
                  variant="link"
                  className="h-auto max-w-full justify-start whitespace-normal p-0 text-left text-foreground"
                >
                  <Link to={paths.facility(v.id)} search={v.deleted_at ? { tab: 'changes' } : {}}>
                    {v.name}
                  </Link>
                </Button>
                <p className="break-all text-xs text-muted-foreground">{v.code}</p>
                <p className="text-xs text-muted-foreground lg:hidden">
                  {v.type_name} · V{v.type_number}
                </p>
              </div>
            ),
          },
          {
            id: 'type',
            label: '设施类型',
            secondary: true,
            cell: (v) => (v.type_name ? `${v.type_name} · V${v.type_number}` : '—'),
          },
          {
            id: 'status',
            label: '状态',
            className: 'w-24 lg:w-32',
            cell: (v) => (
              <Badge variant="outline" className="font-normal">
                {v.deleted_at ? '已删除' : v.status === 'normal' ? '正常' : '已退役'}
              </Badge>
            ),
          },
          {
            id: 'actions',
            label: '操作',
            className: 'w-14 text-right',
            cell: (v) => (
              <DirectoryActions
                name={v.name}
                actions={[
                  {
                    label: v.deleted_at ? '查看历史' : '打开设施',
                    run: () =>
                      void navigate({ to: paths.facility(v.id), search: v.deleted_at ? { tab: 'changes' } : {} }),
                  },
                ]}
              />
            ),
          },
        ]}
        pagination={
          <ListPagination
            label={title}
            count={items.length}
            page={q.page}
            pageCount={q.pageCount}
            hasNext={q.hasNext}
            fetching={q.isFetching}
            go={q.go}
            next={() => void q.next()}
            pageSize={pageSize}
            setPageSize={setPageSize}
          />
        }
      />
      <ResponsivePanel
        open={creating}
        onOpenChange={setCreating}
        title="新增设施"
        description="选择设施类型及版本，填写设施资料。"
        fallbackFocusSelector="#create-facility"
      >
        {creating && (
          <ResourceEditor
            target={{ kind: 'facility' }}
            done={(v) => {
              setCreating(false);
              void navigate({ to: paths.facility(v.id), search: {} });
            }}
          />
        )}
      </ResponsivePanel>
    </section>
  );
}
