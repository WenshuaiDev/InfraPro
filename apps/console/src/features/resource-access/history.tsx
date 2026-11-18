import { useState } from 'react';
import { History } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { useCursorList } from '../../hooks/use-cursor-list';
import { useResourceAccess } from './shared';
import { capabilities, describe, kinds, type Capability } from './model';
import { ChangeList, type Difference } from './presentation';
import type { ResourceKind } from '@infrapro/api-client';

const record = (value: unknown): Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
const array = (value: unknown): unknown[] => (Array.isArray(value) ? value : []);
const string = (value: unknown) => (typeof value === 'string' ? value : '');
function grant(value: unknown) {
  const v = record(value);
  const kind = string(v.kind);
  if (!(kind in kinds)) return undefined;
  const k = kind as ResourceKind;
  return {
    id: string(v.id) || string(v.resource_id),
    name: string(v.name) || '资源',
    code: string(v.code),
    kind: k,
    capabilities: array(v.capabilities).filter(
      (c): c is Capability => typeof c === 'string' && capabilities[k].includes(c as Capability),
    ),
  };
}
function changes(before: unknown, after: unknown, operation: string): Difference[] {
  const b = record(before),
    a = record(after);
  if (operation === 'project_create') {
    const created = grant(after);
    return created
      ? [{ ...created, before: '未创建', after: describe(created.capabilities), action: 'add' }]
      : [];
  }
  if (operation === 'policy')
    return [
      {
        id: 'policy',
        name: '新建设施权限',
        before: record(b.policy).allow_facility_create ? '允许新建设施' : '不允许新建设施',
        after: a.allow_facility_create ? '允许新建设施，并维护新建资源' : '不允许新建设施；已有授权保留',
        action: 'update',
      },
    ];
  const previous = array(b.grants)
    .map(grant)
    .filter((v) => v !== undefined);
  return (['added', 'updated', 'ended'] as const).flatMap((key) =>
    array(a[key]).flatMap((raw) => {
      const g = grant(raw);
      return g
        ? [
            {
              id: `${key}:${g.id}`,
              name: g.name,
              code: g.code,
              kind: g.kind,
              before:
                key === 'added'
                  ? '未授权'
                  : describe(
                      key === 'ended' ? g.capabilities : previous.find((v) => v.id === g.id)?.capabilities,
                    ),
              after: key === 'ended' ? '收回当前项目的授权' : describe(g.capabilities),
              action: ({ added: 'add', updated: 'update', ended: 'end' } as const)[key],
            },
          ]
        : [];
    }),
  );
}
const operationNames: Record<string, string> = {
  policy: '调整新建设施权限',
  create_link: '添加设施并授权资源',
  change_grants: '调整资源范围与权限',
  revoke_link: '将设施移出项目',
  project_create: '项目创建资源并自动授权',
  project_resource_create: '项目创建资源并自动授权',
};
export function AccessHistory({ project }: { project: string }) {
  const { client, tenant, prefix, canRead } = useResourceAccess(project);
  const [pageSize, setPageSize] = useState(20);
  const list = useCursorList({ pageSize,
    queryKey: [...prefix, 'history', pageSize],
    load: (cursor, signal) =>
      client.resourceAccessChanges(tenant.tenant.id, project, { cursor, limit: pageSize }, signal),
    enabled: canRead,
  });
  return (
    <section className="space-y-4">
      <div>
        <h2 className="font-semibold">授权记录</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          查看授权何时调整、为什么调整，以及每项资源的权限变化。
        </p>
      </div>
      {list.error && <ErrorState message={list.error.message} onRetry={() => void list.refetch()} />}
      {list.isPending ? (
        <LoadingState label="正在读取授权记录…" />
      ) : !list.items.length ? (
        <div className="rounded-xl border p-10 text-center">
          <History className="mx-auto mb-3 size-6 text-muted-foreground" aria-hidden="true" />
          <p className="font-medium">还没有授权变更记录</p>
          <p className="mt-2 text-sm text-muted-foreground">
            保存资源授权或新建设施权限后，会在这里留下记录。
          </p>
        </div>
      ) : (
        list.items.map((item) => {
          const diff = changes(item.before, item.after, item.operation);
          return (
            <article key={item.id} className="space-y-3 rounded-xl border p-5">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h3 className="font-medium">{operationNames[item.operation] ?? '资源授权更新'}</h3>
                <time className="text-xs text-muted-foreground" dateTime={item.changed_at}>
                  {new Date(item.changed_at).toLocaleString('zh-CN', { hour12: false })}
                </time>
              </div>
              <p className="break-words text-sm">变更原因：{item.reason || '随资源创建自动授权'}</p>
              <p className="text-xs text-muted-foreground">
                {item.actor_scope === 'tenant' ? '租户管理端' : '项目工作空间'} · 授权版本{' '}
                {item.access_revision} · {diff.length} 项变化
              </p>
              <details>
                <summary className="cursor-pointer text-sm font-medium">查看权限变化</summary>
                <div className="mt-3 space-y-3">
                  {diff.length ? (
                    <ChangeList items={diff} />
                  ) : (
                    <p className="text-sm text-muted-foreground">此历史记录没有逐项权限差异。</p>
                  )}
                  <p className="break-all text-xs text-muted-foreground">操作账号 ID：{item.actor_id}</p>
                </div>
              </details>
            </article>
          );
        })
      )}
      <ListPagination label="授权记录" {...list.pagination} setPageSize={setPageSize} />
      {list.error && (
        <Button variant="outline" onClick={() => void list.refetch()}>
          重新读取记录
        </Button>
      )}
    </section>
  );
}
