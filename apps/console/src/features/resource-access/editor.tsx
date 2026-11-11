import { useState } from 'react';
import { Link } from '@tanstack/react-router';
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import type {
  ResourceAccessDetail,
  ResourceAccessInput,
  ResourceGrant,
  ResourceGrantInput,
  ResourceKind,
} from '@infrapro/api-client';
import { DirectoryTable, DirectorySearch } from '@infrapro/ui/patterns/directory-table';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { Tabs, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { Button } from '@infrapro/ui/components/button';
import { ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { SelectControl, CheckControl } from '../engineering-fields/controls';
import { TenantResources } from '../resources/context';
import { ComponentTree } from '../resources/component-tree';
import { AccessCommit, CapabilityPicker, useResourceAccess } from './shared';
import { capNames, kinds, type Capability } from './model';
type Choice = {
  kind: ResourceKind;
  id: string;
  name: string;
  code: string;
  capabilities: Capability[];
  grant?: ResourceGrant;
  end?: boolean;
};
function input(v: Choice): ResourceGrantInput {
  return {
    kind: v.kind,
    capabilities: v.capabilities,
    ...(v.kind === 'component'
      ? { component_id: v.id }
      : v.kind === 'point'
        ? { point_id: v.id }
        : v.kind === 'channel'
          ? { channel_id: v.id }
          : {}),
  };
}
export function FacilityResourceAccessPage({ project, facility }: { project: string; facility: string }) {
  const { client, tenant, prefix, canRead } = useResourceAccess(project);
  const cache = useQueryClient();
  const [generation, setGeneration] = useState(0);
  const state = useQuery({
    queryKey: [...prefix, 'facility', facility, 'base', generation],
    queryFn: ({ signal }) => client.resourceAccess(tenant.tenant.id, project, facility, {}, signal),
    enabled: canRead,
    refetchOnWindowFocus: false,
  });
  const reload = () => {
    void cache.removeQueries({ queryKey: [...prefix, 'facility', facility] });
    setGeneration((g) => g + 1);
  };
  if (!canRead) return <p>没有资源授权查看权限。</p>;
  return (
    <section className="space-y-5">
      <Link
        to="/t/$tenantId/resource-access/$projectId"
        params={{ tenantId: tenant.tenant.id, projectId: project }}
        className="underline"
      >
        返回项目资源授权
      </Link>
      {state.error && <ErrorState message={state.error.message} onRetry={() => void state.refetch()} />}{' '}
      {state.data ? (
        <TenantResources>
          <GrantEditor
            key={`${project}:${facility}:${generation}`}
            project={project}
            facility={facility}
            initial={state.data}
            reload={reload}
          />
        </TenantResources>
      ) : (
        <p role="status">正在加载设施授权…</p>
      )}
    </section>
  );
}
function GrantEditor({
  project,
  facility,
  initial,
  reload,
}: {
  project: string;
  facility: string;
  initial: ResourceAccessDetail;
  reload: () => void;
}) {
  const { client, tenant, prefix, canWrite } = useResourceAccess(project);
  const [draft, setDraft] = useState<Record<string, Choice>>({});
  const [kind, setKind] = useState<ResourceKind>('component');
  const [q, setQ] = useState('');
  const [parent, setParent] = useState<string>();
  const [current, setCurrent] = useState(false);
  const [allKinds, setAllKinds] = useState<ResourceKind[]>(['component', 'point', 'channel']);
  const [revoke, setRevoke] = useState(false);
  const [reloadConfirm, setReloadConfirm] = useState(false);
  const [tab, setTab] = useState(initial.link ? 'current' : 'candidates');
  const [showTree, setShowTree] = useState(false);
  const base = [...prefix, 'facility', facility, initial.policy.revision];
  const archived = initial.policy.project_status === 'archived';
  const detail = useQuery({
    queryKey: [...base, 'name'],
    queryFn: ({ signal }) =>
      client.resourceDetail({ kind: 'tenant', tenantId: tenant.tenant.id }, { kind: 'facility', id: facility }, signal),
  });
  const grants = useInfiniteQuery({
    queryKey: [...base, 'grants'],
    initialPageParam: '',
    queryFn: ({ pageParam, signal }) =>
      client.resourceAccess(tenant.tenant.id, project, facility, { grant_cursor: pageParam }, signal),
    getNextPageParam: (p) => p.grant_next_cursor || undefined,
    refetchOnWindowFocus: false,
  });
  const candidates = useInfiniteQuery({
    queryKey: [...base, 'candidates', kind, q, parent],
    initialPageParam: '',
    queryFn: ({ pageParam, signal }) =>
      client.resourceAccess(
        tenant.tenant.id,
        project,
        facility,
        {
          candidate_kind: kind,
          candidate_cursor: pageParam,
          candidate_q: q,
          candidate_status: 'all',
          candidate_parent_id: parent || 'root',
          candidate_owner: parent || 'root',
        },
        signal,
      ),
    getNextPageParam: (p) => p.candidates.next_cursor || undefined,
    refetchOnWindowFocus: false,
  });
  const put = (value: Choice) => setDraft((d) => ({ ...d, [value.id]: value }));
  const remove = (id: string) =>
    setDraft((d) => {
      const copy = { ...d };
      delete copy[id];
      return copy;
    });
  const choices = Object.values(draft);
  const loadedGrants = [
    ...initial.grants,
    ...(grants.data?.pages.flatMap((p) => p.grants) ?? []),
    ...(candidates.data?.pages.flatMap((p) => p.candidate_grants) ?? []),
  ];
  const grantFor = (id: string) => loadedGrants.find((g) => g.resource_id === id);
  const asChoice = (g: ResourceGrant): Choice => ({
    kind: g.kind,
    id: g.resource_id,
    name: g.name,
    code: g.code,
    capabilities: g.capabilities,
    grant: g,
  });
  const build = (reason: string, request_id: string): ResourceAccessInput => {
    const add = choices.filter((v) => !v.grant && !v.end).map(input);
    if (!initial.link && !add.some((v) => v.kind === 'facility'))
      add.unshift({ kind: 'facility', capabilities: ['read'] });
    return {
      action: revoke ? 'revoke_link' : initial.link ? 'change_grants' : 'create_link',
      reason,
      request_id,
      expected_access_revision: initial.policy.revision,
      ...(initial.link ? { expected_link_revision: initial.link.revision } : {}),
      ...(!revoke
        ? {
            add,
            update: choices
              .filter((v) => v.grant && !v.end)
              .map((v) => ({
                grant_id: v.grant!.id,
                expected_revision: v.grant!.revision,
                capabilities: v.capabilities,
              })),
            end: choices
              .filter((v) => v.grant && v.end)
              .map((v) => ({ grant_id: v.grant!.id, expected_revision: v.grant!.revision })),
            ...(current
              ? {
                  select_current: {
                    root_kind: parent ? 'component' : 'facility',
                    root_id: parent || facility,
                    kinds: Object.fromEntries(allKinds.map((k) => [k, ['read']])),
                  },
                }
              : {}),
          }
        : {}),
    };
  };
  const draftKey = JSON.stringify([draft, current, allKinds, parent, revoke]);
  const rows = (values: Choice[], mode: 'current' | 'candidate') => (
    <DirectoryTable
      label={mode === 'current' ? '已授权对象' : '候选对象'}
      items={values}
      rowId={(v) => v.id}
      columns={[
        {
          id: 'name',
          label: '对象',
          cell: (v) => {
            const chosen = draft[v.id];
            const editing = chosen ?? v;
            return (
              <div className="space-y-2">
                <p className="font-medium">
                  {kinds[v.kind]} · {v.name}
                </p>
                <p className="break-all text-xs text-muted-foreground">
                  {v.code} ·{' '}
                  {chosen?.end
                    ? '拟撤销'
                    : chosen
                      ? chosen.grant
                        ? '拟调整'
                        : '拟新增'
                      : v.grant
                        ? '当前已授权'
                        : '未授权'}
                </p>
                {chosen && !chosen.end ? (
                  <CapabilityPicker
                    kind={v.kind}
                    value={editing.capabilities}
                    onChange={(capabilities) => put({ ...editing, capabilities })}
                    disabled={!canWrite || revoke}
                    archived={archived}
                    original={v.grant?.capabilities}
                  />
                ) : (
                  <p className="text-sm text-muted-foreground">
                    {v.grant
                      ? v.grant.capabilities.map((c) => capNames[c]).join('、')
                      : mode === 'candidate'
                        ? '默认仅授予读取'
                        : '—'}
                  </p>
                )}
              </div>
            );
          },
        },

        {
          id: 'actions',
          label: '操作',
          className: 'w-28 sm:w-40',
          cell: (v) => {
            const chosen = draft[v.id];
            return (
              canWrite &&
              !revoke && (
                <div className="flex flex-wrap gap-2">
                  {!chosen ? (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      className="h-auto min-h-9 whitespace-normal"
                      disabled={(archived && !v.grant) || current}
                      onClick={() => put(v)}
                    >
                      {v.grant ? '调整能力' : '选择并授予读取'}
                    </Button>
                  ) : (
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      className="h-auto min-h-9 whitespace-normal"
                      onClick={() => remove(v.id)}
                    >
                      取消本项变更
                    </Button>
                  )}
                  {v.grant && v.kind !== 'facility' && !chosen?.end && (
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      disabled={current}
                      onClick={() => put({ ...v, end: true })}
                    >
                      撤销本项
                    </Button>
                  )}
                </div>
              )
            );
          },
        },
      ]}
    />
  );

  return (
    <>
      <header className="space-y-2">
        <h1 className="break-words text-2xl font-semibold">
          {detail.data?.name ?? initial.link?.facility_name ?? '设施'} · 项目资源授权
        </h1>
        <p>
          {tenant.tenant.name} / {initial.policy.project_name} / {archived ? '已归档，只能缩减' : '运行中'}
        </p>
        <p className="text-sm text-muted-foreground">
          {initial.link ? '调整当前关联的明确对象范围。' : '保存时新建关联并授予根设施读取。'}
          维护同一份统一资源；父对象授权不会自动扩展到下级。
        </p>
      </header>
      <div className="flex flex-wrap gap-3">
        <Button type="button" variant="outline" onClick={() => setReloadConfirm(true)}>
          重新载入当前授权
        </Button>
        {initial.link && canWrite && (
          <Button type="button" variant="outline" onClick={() => setRevoke(!revoke)}>
            {revoke ? '取消结束关联' : '结束整条设施关联'}
          </Button>
        )}
      </div>
      {reloadConfirm && (
        <FormNotice>
          <p>重新载入将清除本页未保存选择，以最新授权重新编辑。</p>
          <Button type="button" variant="outline" onClick={reload}>
            确认重新载入
          </Button>
          <Button type="button" variant="ghost" onClick={() => setReloadConfirm(false)}>
            保留当前输入
          </Button>
        </FormNotice>
      )}
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="current">已授权对象</TabsTrigger>
          <TabsTrigger value="candidates">添加对象</TabsTrigger>
        </TabsList>
      </Tabs>
      {revoke ? (
        <FormNotice error>将撤销整座设施在该项目的全部授权；公共资源和其他项目仍保留。必须填写原因并预览。</FormNotice>
      ) : (
        <>
          {tab === 'current' && (
            <section className="space-y-3">
              <h2 className="text-lg font-semibold">当前已授权对象</h2>
              {grants.error && <ErrorState message={grants.error.message} onRetry={() => void grants.refetch()} />}
              {rows((grants.data?.pages.flatMap((p) => p.grants) ?? initial.grants).map(asChoice), 'current')}
              {!initial.link && <p>尚无关联，根设施将以读取能力加入。</p>}
              {grants.hasNextPage && (
                <Button type="button" variant="outline" onClick={() => void grants.fetchNextPage()}>
                  加载更多已授权对象
                </Button>
              )}
            </section>
          )}
          {tab === 'candidates' && (
            <section className="min-w-0 space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <h2 className="text-lg font-semibold">选择对象</h2>
                <Button variant="outline" onClick={() => setShowTree(true)}>
                  {parent ? '更换所属构件' : '按所属构件筛选'}
                </Button>
              </div>
              <div className="flex flex-wrap items-end gap-3">
                <div className="w-full sm:w-40">
                  <SelectControl
                    label="对象种类"
                    value={kind}
                    onChange={(v) => setKind(v as ResourceKind)}
                    options={Object.entries(kinds).map(([value, label]) => ({ value, label }))}
                  />
                </div>
                <DirectorySearch label="搜索候选名称或编号" value={q} onChange={setQ} />
                <p className="text-sm text-muted-foreground">
                  {parent ? '当前所选构件的直属对象' : '设施本体的直属对象'}
                </p>
              </div>
              {candidates.error && (
                <ErrorState message={candidates.error.message} onRetry={() => void candidates.refetch()} />
              )}{' '}
              {rows(
                (candidates.data?.pages.flatMap((p) => p.candidates.items) ?? []).map((r) => ({
                  kind: r.kind,
                  id: r.id,
                  name: r.name,
                  code: r.code,
                  capabilities: grantFor(r.id)?.capabilities ?? ['read'],
                  grant: grantFor(r.id),
                })),
                'candidate',
              )}
              {candidates.isPending && <p role="status">正在加载候选对象…</p>}
              {candidates.hasNextPage && (
                <Button type="button" variant="outline" onClick={() => void candidates.fetchNextPage()}>
                  加载更多候选对象
                </Button>
              )}
              <ResponsivePanel
                open={showTree}
                onOpenChange={setShowTree}
                title="选择所属对象"
                description="选择设施本体或构件，筛选直属候选对象。"
              >
                <ComponentTree
                  facility={facility}
                  selected={parent}
                  onSelect={(value) => {
                    setParent(value);
                    setShowTree(false);
                  }}
                />
              </ResponsivePanel>
            </section>
          )}
          {canWrite && !archived && (
            <section className="space-y-3 rounded-lg border p-4">
              <CheckControl
                label="选择当前全部下级（仅本次，不包含未来新增）"
                value={current}
                onChange={(v) => {
                  setCurrent(v);
                  if (v) setDraft({});
                }}
              />
              {current && (
                <>
                  <p className="text-sm">
                    以左侧当前选择为根，授予以下种类当前所有下级的读取能力；现有维护能力将按预览缩减为只读。单次最多
                    1,000 个对象，超限请按构件分批。
                  </p>
                  <div className="flex flex-wrap gap-4">
                    {(['component', 'point', 'channel'] as ResourceKind[]).map((k) => (
                      <CheckControl
                        key={k}
                        label={kinds[k]}
                        value={allKinds.includes(k)}
                        onChange={(yes) => setAllKinds((v) => (yes ? [...v, k] : v.filter((x) => x !== k)))}
                      />
                    ))}
                  </div>
                </>
              )}
            </section>
          )}
          {choices.length > 0 && (
            <section className="space-y-3">
              <h2 className="text-lg font-semibold">本次选择（跨页保留，共 {choices.length} 项）</h2>
              {rows(choices, 'candidate')}
            </section>
          )}
        </>
      )}
      {canWrite && (
        <AccessCommit
          project={project}
          facility={facility}
          draftKey={draftKey}
          build={build}
          disabled={
            choices.length > 1000 ||
            (!revoke && !current && !!initial.link && !choices.length) ||
            (archived && !initial.link)
          }
          onSaved={reload}
        />
      )}
    </>
  );
}
