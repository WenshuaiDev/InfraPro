import { useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import { ApiError, Permissions as P, type EngineeringKind, type EngineeringScope } from '@infrapro/api-client';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Plus, Copy, Settings2 } from 'lucide-react';
import { Badge } from '@infrapro/ui/components/badge';
import {
  DirectoryTable,
  DirectorySearch,
  DirectoryReset,
  DirectoryActions,
} from '@infrapro/ui/patterns/directory-table';
import { ListFilter, ListPagination } from '@infrapro/ui/patterns/list-controls';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { useOptionalTenant } from '../tenancy/context';
import { useOperation } from '../account/use-operation';
import { CategoriesPanel } from './catalog-editors';
import { Issues, Section, TextControl } from './controls';
import { CopyPublicTemplate } from './copy-template';
import { DefinitionEditor } from './definition-editor';
import { emptyContent, kinds, permissions, titles } from './model';
import { useRequestMeta } from './use-request-meta';
export function EngineeringPage({
  kind,
  catalog = false,
  selected,
  onSelect,
}: {
  kind: string;
  catalog?: boolean;
  selected?: string;
  onSelect: (id?: string) => void;
}) {
  const { account, can } = useIdentity();
  const tenant = useOptionalTenant();
  if (!kinds.includes(kind as EngineeringKind))
    return <ErrorState title="目录不存在" message="请选择设施类型、指标、单位或监测模板目录。" />;
  const scope: EngineeringScope = { kind: kind as EngineeringKind, tenant: tenant?.tenant.id, catalog };
  const codes = permissions(scope);
  const allows = (index: number) =>
    !!codes[index] && (tenant ? tenant.permissions.includes(codes[index]!) : can(codes[index]!));
  if (!allows(0)) return <ErrorState title="没有工程目录读取权限" message="请联系当前范围的管理员配置相应角色。" />;
  return (
    <EngineeringBody
      key={`${account?.id}:${tenant?.membership_id ?? 'public'}:${kind}:${catalog}`}
      scope={scope}
      selected={selected}
      onSelect={onSelect}
      editable={allows(1) && tenant?.available !== false}
      publishable={allows(2) && tenant?.available !== false}
      stateable={allows(3) && tenant?.available !== false}
      copyable={
        scope.catalog
          ? scope.kind === 'monitoring-templates' &&
            !!tenant?.permissions.includes(P.TenantEngineeringTemplateRead) &&
            tenant.permissions.includes(P.TenantEngineeringTemplateEdit) &&
            tenant.available
          : allows(1)
      }
    />
  );
}
function EngineeringBody({
  scope,
  selected,
  onSelect,
  editable,
  publishable,
  stateable,
  copyable,
}: {
  scope: EngineeringScope;
  selected?: string;
  onSelect: (id?: string) => void;
  editable: boolean;
  publishable: boolean;
  stateable: boolean;
  copyable: boolean;
}) {
  const { client, account } = useIdentity();
  const tenant = useOptionalTenant();
  const cache = useQueryClient();
  const [filter, setFilter] = useState({ q: '', status: '', offset: 0 });
  const [pageSize, setPageSize] = useState(20);
  const [panel, setPanel] = useState<'create' | 'copy' | 'categories' | ''>('');
  const prefix = tenant
    ? ['tenant', account?.id, tenant.tenant.id, tenant.membership_id, 'engineering']
    : ['platform', account?.id, 'engineering'];
  const list = useQuery({
    queryKey: [...prefix, scope.catalog, scope.kind, 'list', filter, pageSize],
    queryFn: ({ signal }) => client.engineeringList(scope, { ...filter, limit: pageSize }, signal),
    enabled: !selected,
  });
  const detail = useQuery({
    queryKey: [...prefix, scope.catalog, scope.kind, 'detail', selected],
    queryFn: ({ signal }) => client.engineeringDetail(scope, selected!, signal),
    enabled: !!selected,
    retry: false,
  });
  const refresh = async () => {
    const result = await detail.refetch({ throwOnError: true });
    if (!result.data) throw new Error('未能读取当前结果。');
    void cache.invalidateQueries({ queryKey: prefix });
    return result.data;
  };
  const title = `${scope.tenant ? '本租户' : scope.kind === 'monitoring-templates' ? '公共' : ''}${titles[scope.kind]}`;
  const descriptions = {
    'type-definitions': '维护设施的属性字段、构件类别与组成规则。',
    metrics: '维护监测指标的取值规则和允许使用的单位。',
    units: '维护计量单位、符号与类别。',
    'monitoring-templates': scope.tenant
      ? '维护本租户的监测模板，或从公共模板复制后调整。'
      : '维护可供租户选用的监测项与适用范围。',
  };
  const error = detail.error instanceof ApiError ? detail.error : undefined;
  const restricted = error?.status === 401 || error?.status === 403 || error?.status === 404;
  const copied = (id: string) => {
    setPanel('');
    onSelect(id);
    void cache.invalidateQueries({ queryKey: prefix });
  };
  const canCopyPublic = editable && !!tenant?.permissions.includes(P.TenantEngineeringCatalogRead);
  const page = Math.floor(filter.offset / pageSize);
  const total = list.data?.total ?? 0;
  return (
    <section className="min-w-0 space-y-6">
      {!selected && (
        <PageHeader
          title={title}
          description={descriptions[scope.kind]}
          actions={
            <div className="flex flex-wrap gap-2">
              {!scope.tenant && ['units', 'metrics'].includes(scope.kind) && (
                <Button variant="outline" onClick={() => setPanel('categories')}>
                  <Settings2 aria-hidden="true" />
                  类别管理
                </Button>
              )}
              {canCopyPublic && (
                <Button id="copy-public-template" variant="outline" onClick={() => setPanel('copy')}>
                  <Copy aria-hidden="true" />
                  从公共模板复制
                </Button>
              )}
              {editable && (
                <Button id="create-definition" onClick={() => setPanel('create')}>
                  <Plus aria-hidden="true" />
                  新建定义
                </Button>
              )}
            </div>
          }
        />
      )}
      {selected ? (
        <>
          {detail.error && (
            <ErrorState
              title={restricted ? '当前定义不可访问' : '暂时无法读取，编辑已保留'}
              message={detail.error.message}
              onRetry={() => void detail.refetch()}
            />
          )}
          {!detail.data && !detail.error && <p role="status">正在读取工程定义…</p>}
          {detail.data && !restricted && (
            <DefinitionEditor
              key={selected}
              initial={detail.data}
              scope={scope}
              editable={editable}
              publishable={publishable}
              stateable={stateable}
              copyable={copyable}
              refresh={refresh}
              close={() => {
                onSelect();
                void cache.invalidateQueries({ queryKey: prefix });
              }}
              copied={copied}
            />
          )}
          {restricted && (
            <Button variant="outline" onClick={() => onSelect()}>
              返回目录
            </Button>
          )}
        </>
      ) : (
        <>
          <DirectoryTable
            label={title}
            items={list.data?.items ?? []}
            rowId={(v) => v.id}
            loading={list.isPending}
            error={list.error?.message}
            retry={() => void list.refetch()}
            empty="暂无工程定义"
            toolbar={
              <>
                <DirectorySearch
                  label="搜索名称或编号"
                  value={filter.q}
                  onChange={(q) => setFilter({ ...filter, q, offset: 0 })}
                />
                <ListFilter
                  label="可用状态"
                  value={filter.status}
                  onChange={(status) => setFilter({ ...filter, status, offset: 0 })}
                  options={[
                    { value: '', label: '全部状态' },
                    { value: 'usable', label: '可新使用' },
                    { value: 'unavailable', label: '不可新使用' },
                    { value: 'draft', label: '存在草稿' },
                  ]}
                />
                {(filter.q || filter.status) && (
                  <DirectoryReset onClick={() => setFilter({ q: '', status: '', offset: 0 })} />
                )}
              </>
            }
            columns={[
              {
                id: 'name',
                label: '名称',
                cell: (v) => (
                  <div className="space-y-1">
                    <Button
                      variant="link"
                      className="h-auto max-w-full justify-start whitespace-normal p-0 text-left text-foreground"
                      onClick={() => onSelect(v.id)}
                    >
                      {v.name || '未命名草稿'}
                    </Button>
                    <p className="break-all text-xs text-muted-foreground">{v.code}</p>
                    <p className="text-xs text-muted-foreground lg:hidden">
                      {v.latest ? `V${v.latest}` : '未发布'}
                      {v.has_draft && ' · 有草稿'}
                    </p>
                  </div>
                ),
              },
              {
                id: 'version',
                label: '发布版本',
                secondary: true,
                className: 'w-40',
                cell: (v) => (
                  <span>
                    {v.latest ? `V${v.latest}` : '—'}
                    {v.has_draft && (
                      <Badge variant="secondary" className="ml-2 font-normal">
                        草稿
                      </Badge>
                    )}
                  </span>
                ),
              },
              {
                id: 'status',
                label: '状态',
                className: 'w-28 lg:w-40',
                cell: (v) => (
                  <Badge variant={v.usable ? 'outline' : 'secondary'} className="whitespace-normal font-normal">
                    {v.usable ? '可新使用' : v.latest ? '不可新使用' : '待发布'}
                  </Badge>
                ),
              },
              {
                id: 'actions',
                label: '操作',
                className: 'w-14 text-right',
                cell: (v) => (
                  <DirectoryActions
                    name={v.name || v.code}
                    actions={[{ label: '查看详情', run: () => onSelect(v.id) }]}
                  />
                ),
              },
            ]}
            pagination={
              <ListPagination
                label={title}
                count={list.data?.items.length ?? 0}
                page={page}
                pageCount={Math.ceil(total / pageSize)}
                hasNext={filter.offset + pageSize < total}
                fetching={list.isFetching}
                go={(p) => setFilter({ ...filter, offset: p * pageSize })}
                next={() => setFilter({ ...filter, offset: filter.offset + pageSize })}
                pageSize={pageSize}
                setPageSize={(size) => {
                  setPageSize(size);
                  setFilter({ ...filter, offset: 0 });
                }}
              />
            }
          />
          <ResponsivePanel
            open={!!panel}
            onOpenChange={(open) => {
              if (!open) setPanel('');
            }}
            title={
              panel === 'copy'
                ? '从公共模板复制'
                : panel === 'categories'
                  ? '单位类别管理'
                  : `新建${titles[scope.kind]}`
            }
            description={
              panel === 'copy'
                ? '选择可用的公共版本，创建本租户独立草稿。'
                : panel === 'categories'
                  ? '维护计量单位及数值指标共用的类别。'
                  : '填写名称与编号后，进入草稿编辑页面。'
            }
            fallbackFocusSelector={panel === 'copy' ? '#copy-public-template' : '#create-definition'}
          >
            {panel === 'create' && editable && <CreateForm scope={scope} done={copied} />}
            {panel === 'copy' && canCopyPublic && <CopyPublicTemplate done={copied} />}
            {panel === 'categories' && <CategoriesPanel editable={editable} />}
          </ResponsivePanel>
        </>
      )}
    </section>
  );
}

function CreateForm({ scope, done }: { scope: EngineeringScope; done: (id: string) => void }) {
  const { client } = useIdentity();
  const form = useForm({ defaultValues: { code: '', name: '', reason: '' } });
  const values = useWatch({ control: form.control });
  const meta = useRequestMeta();
  const mutation = useOperation({
    mutationFn: () => {
      const v = form.getValues();
      const input = { code: v.code, content: emptyContent(scope.kind, v.name) };
      return client.engineeringCreate(scope, { ...input, ...meta('create', input, v.reason) });
    },
    onSuccess: (v) => done(v.definition_id),
  });
  return (
    <Section title={`新建${titles[scope.kind]}`}>
      <form className="space-y-4" onSubmit={form.handleSubmit(() => mutation.mutate())}>
        <div className="grid gap-4 sm:grid-cols-2">
          <TextControl
            label="稳定编号"
            value={values.code ?? ''}
            onChange={(v) => form.setValue('code', v)}
            hint="小写字母开头，可包含数字和下划线；创建后不修改、不复用。"
          />
          <TextControl label="定义名称" value={values.name ?? ''} onChange={(v) => form.setValue('name', v)} />
        </div>
        <TextControl label="创建原因" value={values.reason ?? ''} onChange={(v) => form.setValue('reason', v)} />
        <FormNotice error>{mutation.error?.message}</FormNotice>
        {mutation.error instanceof ApiError && <Issues issues={mutation.error.issues} />}
        <Button disabled={mutation.isPending} type="submit">
          创建草稿
        </Button>
      </form>
    </Section>
  );
}
