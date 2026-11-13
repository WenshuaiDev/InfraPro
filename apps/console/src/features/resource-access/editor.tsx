import { useRef, useState } from 'react';
import { Link, useBlocker, useNavigate } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import type {
  ResourceAccessDetail,
  ResourceAccessInput,
  ResourceDetail,
  ResourceKind,
} from '@infrapro/api-client';
import { Permissions as P } from '@infrapro/api-client';
import { ArrowLeft, Eye, Settings2, ShieldCheck, Users } from 'lucide-react';
import { toast } from 'sonner';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Checkbox } from '@infrapro/ui/components/checkbox';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { DirectorySearch } from '@infrapro/ui/patterns/directory-table';
import { ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@infrapro/ui/components/dialog';
import { useOperation } from '../account/use-operation';
import { AccessCommit, CapabilityPicker, useResourceAccess } from './shared';
import {
  asChoice,
  assertAccessRevision,
  countResources,
  draftSummary,
  emptySummary,
  grantInput,
  kinds,
  putChoice,
  type Choice,
  type Draft,
} from './model';
import { ChangeList, Hint, ResourceCounts, choiceDifferences } from './presentation';
import { ResourceSelector } from './resource-selector';

export function FacilityResourceAccessPage({ project, facility }: { project: string; facility: string }) {
  const { client, tenant, prefix, canRead } = useResourceAccess(project);
  const [generation, setGeneration] = useState(0);
  const access = useQuery({
    queryKey: [...prefix, 'facility', facility, 'base', generation],
    queryFn: ({ signal }) =>
      client.resourceAccess(
        tenant.tenant.id,
        project,
        facility,
        { candidate_kind: 'facility', limit: 100 },
        signal,
      ),
    enabled: canRead,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const resource = useQuery({
    queryKey: [...prefix, 'facility', facility, 'name', generation],
    queryFn: ({ signal }) =>
      client.resourceDetail(
        { kind: 'tenant', tenantId: tenant.tenant.id },
        { kind: 'facility', id: facility },
        signal,
      ),
    enabled: canRead,
  });
  if (!canRead) return <ErrorState message="没有资源授权查看权限。" />;
  if (access.error || resource.error)
    return (
      <ErrorState
        message={(access.error || resource.error)!.message}
        onRetry={() => {
          void access.refetch();
          void resource.refetch();
        }}
      />
    );
  if (!access.data || !resource.data) return <LoadingState label="正在读取设施与授权…" />;
  return (
    <GrantEditor
      key={`${project}:${facility}:${generation}`}
      project={project}
      facility={facility}
      generation={generation}
      initial={access.data}
      resource={resource.data}
      reload={() => setGeneration((v) => v + 1)}
    />
  );
}
function GrantEditor({
  project,
  facility,
  initial: loaded,
  resource,
  reload,
  generation,
}: {
  project: string;
  facility: string;
  generation: number;
  initial: ResourceAccessDetail;
  resource: ResourceDetail;
  reload: () => void;
}) {
  // Freeze the baseline until explicit reload. Background reads must not rebase an unsaved draft.
  const [initial] = useState(loaded);
  const { client, tenant, canWrite } = useResourceAccess(project);
  const originalRoot =
    initial.candidate_grants.find((g) => g.kind === 'facility') ??
    initial.grants.find((g) => g.kind === 'facility');
  const root: Choice = originalRoot
    ? { ...asChoice(originalRoot), status: resource.status }
    : {
        id: facility,
        kind: 'facility',
        name: resource.name,
        code: resource.code,
        status: resource.status,
        capabilities: ['read'],
      };
  const seed = (): Draft =>
    initial.link || !canWrite || initial.policy.project_status === 'archived' ? {} : { [facility]: root };
  const [draft, setDraft] = useState<Draft>(seed);
  const [active, setActive] = useState<Choice>(root);
  const [q, setQ] = useState('');
  const [kind, setKind] = useState<ResourceKind | 'all'>('all');
  const [onlySelected, setOnlySelected] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [reloadConfirm, setReloadConfirm] = useState(false);
  const [resetConfirm, setResetConfirm] = useState(false);
  const navigate = useNavigate();
  const archived = initial.policy.project_status === 'archived';
  const sourceManage = tenant.permissions.includes(P.TenantMonitoringSourceManage);
  const choices = Object.values(draft);
  const summary = draftSummary(initial.summary ?? emptySummary, choices);
  const dirty = choices.length > 0;
  const completed = useRef(false);
  const blocker = useBlocker({
    shouldBlockFn: () => dirty && !completed.current,
    withResolver: true,
    enableBeforeUnload: dirty,
  });
  const selected = draft[active.id] ?? active;
  const checked = !selected.end && (!!selected.grant || !!draft[selected.id]);
  const put = (value: Choice) => setDraft((d) => putChoice(d, value));
  const open = (value: Choice) => {
    setActive(value);
    if (window.matchMedia('(max-width: 1023px)').matches) setSettingsOpen(true);
  };
  const toggle = (value: Choice, yes: boolean) => {
    const current = draft[value.id] ?? value;
    put({
      ...value,
      capabilities: yes
        ? current.end
          ? (value.grant?.capabilities ?? ['read'])
          : current.capabilities
        : current.capabilities,
      end: !yes,
    });
    setActive(value);
  };
  const build = (reason: string, request_id: string): ResourceAccessInput => ({
    action: initial.link ? 'change_grants' : 'create_link',
    reason,
    request_id,
    expected_access_revision: initial.policy.revision,
    ...(initial.link ? { expected_link_revision: initial.link.revision } : {}),
    add: choices.filter((v) => !v.grant && !v.end).map(grantInput),
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
  });
  const saved = () => {
    completed.current = true;
    toast.success('授权已保存。');
    reload();
  };
  const bulk = useOperation({
    mutationFn: async () => {
      const found: Choice[] = [];
      for (const k of ['component', 'point', 'channel'] as const) {
        let cursor = '';
        do {
          const page = assertAccessRevision(
            await client.resourceAccess(tenant.tenant.id, project, facility, {
              candidate_kind: k,
              candidate_cursor: cursor,
              candidate_limit: 100,
              candidate_status: 'all',
              candidate_parent_id: 'all',
              candidate_owner: 'all',
            }),
            initial.policy.revision,
          );
          found.push(
            ...page.candidates.items.map((r) => {
              const grant = page.candidate_grants.find((g) => g.resource_id === r.id);
              return {
                id: r.id,
                kind: r.kind,
                name: r.name,
                code: r.code,
                status: r.status,
                grant,
                capabilities: grant?.capabilities ?? ['read'],
              } satisfies Choice;
            }),
          );
          if (found.length > 999)
            throw new Error(
              '这座设施的资源超过单次 1,000 项的处理范围，请按构件展开并分批勾选。当前选择已保留。',
            );
          cursor = page.candidates.next_cursor;
        } while (cursor);
      }
      return found;
    },
    onSuccess: (found) => {
      setDraft((old) => {
        let next = { ...old };
        for (const v of found) {
          const pending = next[v.id];
          next = putChoice(next, pending && !pending.end ? pending : { ...v, end: false });
        }
        return next;
      });
      toast.success('已勾选当前资源，保留已有维护权限；保存前请核对。');
    },
  });
  const permissionPanel = (
    <div className="space-y-4">
      <div>
        <p className="break-words font-medium">{active.name}</p>
        <p className="mt-1 break-all text-xs text-muted-foreground">
          {kinds[active.kind]} · {active.code}
        </p>
      </div>
      {checked ? (
        <>
          <p className="flex items-center gap-2 text-sm">
            <Eye className="size-4" aria-hidden="true" />
            允许查看完整档案
          </p>
          <CapabilityPicker
            kind={active.kind}
            sourceKind={active.source_kind}
            value={selected.capabilities}
            onChange={(capabilities) => put({ ...selected, capabilities, end: false })}
            disabled={!canWrite || bulk.isPending}
            original={active.grant?.capabilities}
            archived={archived}
          />
          {active.status === 'retired' && <Hint>该资源已退役。维护操作仍受资源状态及上级状态限制。</Hint>}
        </>
      ) : (
        <>
          <Hint>这个资源尚未开放完整档案。勾选后可设置操作权限；未勾选的父级可能仅用于定位已授权下级。</Hint>
          <Button
            variant="outline"
            disabled={!canWrite || bulk.isPending || (archived && !active.grant)}
            onClick={() => toggle(active, true)}
          >
            开放查看权限
          </Button>
        </>
      )}
    </div>
  );
  return (
    <section className="min-w-0 space-y-5 pb-4">
      <Button asChild variant="ghost" className="-ml-3">
        <Link
          to="/t/$tenantId/resource-access/$projectId"
          params={{ tenantId: tenant.tenant.id, projectId: project }}
        >
          <ArrowLeft aria-hidden="true" />
          返回已授权设施
        </Link>
      </Button>
      <PageHeader title={resource.name} description={`${initial.policy.project_name} · 管理资源与权限`} />
      <Hint>
        {initial.link
          ? '勾选项目可以查看的资源，再点资源名称设置允许的操作。'
          : '设施基本档案会一并加入项目，请勾选还需要开放的下级资源。'}
        父级与下级分别授权；未勾选的下级不会自动开放。
      </Hint>
      {archived && <Hint>项目已归档，只能收回资源或缩减权限。</Hint>}
      {!canWrite && <Hint>你可以查看当前授权，修改需要资源管理和资源授权管理权限。</Hint>}
      <div className="flex flex-wrap items-center gap-3">
        <DirectorySearch label="搜索资源名称或编号" value={q} onChange={setQ} />
        <select
          aria-label="资源类型"
          className="h-10 max-w-full rounded-md border bg-background px-3 text-sm"
          value={kind}
          onChange={(e) => setKind(e.target.value as typeof kind)}
        >
          <option value="all">全部资源类型</option>
          {Object.entries(kinds).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </select>
        <label className="flex min-h-10 items-center gap-2 text-sm">
          <Checkbox checked={onlySelected} onCheckedChange={(v) => setOnlySelected(!!v)} />
          只看已选
        </label>
        <Button variant="ghost" size="sm" className="sm:ml-auto" onClick={() => setReloadConfirm(true)}>
          重新载入当前授权
        </Button>
      </div>
      <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,1.5fr)_minmax(250px,1fr)] 2xl:grid-cols-[minmax(0,1.65fr)_minmax(250px,1fr)_240px]">
        <div className="min-w-0 space-y-3">
          <ResourceSelector
            project={project}
            facility={facility}
            revision={initial.policy.revision}
            generation={generation}
            root={root}
            draft={draft}
            active={active.id}
            canWrite={canWrite && !bulk.isPending}
            archived={archived}
            sourceManage={sourceManage}
            onActive={open}
            onToggle={toggle}
            q={q}
            kind={kind}
            onlySelected={onlySelected}
          />
          {canWrite && !archived && (
            <div className="space-y-2">
              <Button
                variant="outline"
                className="h-auto min-h-10 max-w-full whitespace-normal"
                disabled={bulk.isPending}
                onClick={() => bulk.mutate()}
              >
                {bulk.isPending ? '正在读取当前资源…' : '勾选设施内全部当前资源'}
              </Button>
              <p className="text-xs leading-relaxed text-muted-foreground">
                新勾选的资源仅允许查看，保留已有维护权限；不包含未来新增的资源。单次最多 1,000 项。
              </p>
            </div>
          )}
          {bulk.error && <ErrorState message={bulk.error.message} />}
        </div>
        <section
          aria-label="资源操作权限"
          className="hidden min-w-0 space-y-5 rounded-xl border p-5 lg:block"
        >
          <h2 className="flex items-center gap-2 font-semibold">
            <Settings2 className="size-4" aria-hidden="true" />
            操作权限
          </h2>
          {permissionPanel}
        </section>
        <aside
          aria-label="授权摘要"
          className="min-w-0 space-y-4 rounded-xl border bg-muted/20 p-5 lg:col-span-2 2xl:col-span-1"
        >
          <h2 className="flex items-center gap-2 font-semibold">
            <ShieldCheck className="size-4" aria-hidden="true" />
            授权摘要
          </h2>
          <p className="text-2xl font-semibold">
            {countResources(summary)}
            <span className="ml-2 text-sm font-normal text-muted-foreground">项资源</span>
          </p>
          <p className="text-sm leading-relaxed">
            <ResourceCounts value={summary} />
          </p>
          <div className="space-y-2 border-t pt-3 text-sm">
            <p>
              仅查看 <strong>{countResources(summary) - summary.maintainable}</strong> 项
            </p>
            <p>
              可维护 <strong>{summary.maintainable}</strong> 项
            </p>
          </div>
          <p className="text-xs leading-relaxed text-muted-foreground">
            维护权限按资源设置。成员仍需相应的项目角色权限。
          </p>
          {dirty && (
            <details>
              <summary className="cursor-pointer text-sm font-medium">{choices.length} 项变更待保存</summary>
              <div className="mt-3 max-h-72 overflow-y-auto">
                <ChangeList items={choiceDifferences(choices)} />
              </div>
            </details>
          )}
        </aside>
      </div>
      {canWrite && (
        <div className="sticky bottom-3 z-10 flex flex-wrap items-center justify-between gap-3 rounded-xl border bg-background/95 p-4 shadow-sm backdrop-blur-sm">
          <div>
            <p className="text-sm font-medium">
              已选 {countResources(summary)} 项资源 · {choices.length} 项变更待保存
            </p>
            <p className="mt-1 text-xs text-muted-foreground">提交前会逐项展示权限变化。</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              disabled={!dirty || bulk.isPending}
              onClick={() => setResetConfirm(true)}
            >
              还原修改
            </Button>
            <AccessCommit
              project={project}
              facility={facility}
              build={build}
              draftKey={JSON.stringify(draft)}
              disabled={!dirty || bulk.isPending || choices.length > 1000 || (archived && !initial.link)}
              summary={<ChangeList items={choiceDifferences(choices)} />}
              onSaved={saved}
            />
          </div>
        </div>
      )}
      <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
        <Button asChild variant="ghost" className="h-auto min-h-10 whitespace-normal">
          <Link
            to="/t/$tenantId/resource-access"
            params={{ tenantId: tenant.tenant.id }}
            search={{ facility }}
          >
            <Users aria-hidden="true" />
            查看使用这座设施的项目
          </Link>
        </Button>
        {initial.link && canWrite && (
          <AccessCommit
            project={project}
            facility={facility}
            destructive
            label="将设施移出项目"
            disabled={bulk.isPending}
            draftKey={initial.link.revision}
            build={(reason, request_id) => ({
              action: 'revoke_link',
              reason,
              request_id,
              expected_access_revision: initial.policy.revision,
              expected_link_revision: initial.link!.revision,
            })}
            summary={
              <p className="text-sm">
                将收回已保存的 {countResources(initial.summary)} 项资源授权。本页尚未保存的调整不会提交。
              </p>
            }
            onSaved={() => {
              completed.current = true;
              toast.success('设施已移出项目。');
              void navigate({
                to: '/t/$tenantId/resource-access/$projectId',
                params: { tenantId: tenant.tenant.id, projectId: project },
              });
            }}
          />
        )}
      </div>
      <ResponsivePanel
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
        title="设置资源操作权限"
        description="为当前资源配置权限，再返回清单统一保存。"
      >
        {permissionPanel}
        <Button className="mt-5" onClick={() => setSettingsOpen(false)}>
          完成设置，返回清单
        </Button>
      </ResponsivePanel>
      <Dialog
        open={reloadConfirm || resetConfirm}
        onOpenChange={(v) => {
          if (!v) {
            setReloadConfirm(false);
            setResetConfirm(false);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{reloadConfirm ? '重新载入当前授权？' : '还原本次修改？'}</DialogTitle>
            <DialogDescription>本页未保存的选择和权限调整将被清除，已保存的授权不变。</DialogDescription>
          </DialogHeader>
          <div className="flex justify-end gap-2">
            <Button
              variant="outline"
              onClick={() => {
                setReloadConfirm(false);
                setResetConfirm(false);
              }}
            >
              继续编辑
            </Button>
            <Button
              onClick={() => {
                if (reloadConfirm) {
                  completed.current = true;
                  reload();
                } else {
                  setDraft(seed());
                  setResetConfirm(false);
                }
              }}
            >
              {reloadConfirm ? '确认重新载入' : '确认还原'}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
      <Dialog
        open={blocker.status === 'blocked'}
        onOpenChange={(v) => {
          if (!v && blocker.status === 'blocked') blocker.reset();
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>放弃未保存的授权修改？</DialogTitle>
            <DialogDescription>当前选择尚未保存，离开后需要重新配置。</DialogDescription>
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
    </section>
  );
}
