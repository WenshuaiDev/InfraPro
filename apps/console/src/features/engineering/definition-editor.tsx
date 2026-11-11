import { useRef, useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import {
  ApiError,
  Permissions as P,
  type EngineeringContent,
  type EngineeringDefinition,
  type EngineeringIssue,
  type EngineeringResult,
  type EngineeringScope,
  type EngineeringVersion,
} from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { ConfirmDialog } from '@infrapro/ui/patterns/confirm-dialog';
import { useOperation } from '../account/use-operation';
import { useOptionalTenant } from '../tenancy/context';
import { TenantAudit } from '../tenancy/pages';
import { useLivePage } from '../tenancy/membership-shared';
import { CatalogEditor } from './catalog-editors';
import { Issues, Section, SelectControl, TextControl } from './controls';
import { DirectoryTable } from '@infrapro/ui/patterns/directory-table';
import { DefinitionInfo, DefinitionSnapshot } from './definition-sections';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { ArrowLeft } from 'lucide-react';
import { Badge } from '@infrapro/ui/components/badge';
import { emptyContent, titles } from './model';
import { PreviewPanel } from './preview-panel';
import { TemplateEditor } from './template-editor';
import { TypeEditor } from './type-editor';
import { useRequestMeta } from './use-request-meta';
export function DefinitionEditor({
  initial,
  scope,
  editable,
  publishable,
  stateable,
  copyable,
  refresh,
  close,
  copied,
}: {
  initial: EngineeringDefinition;
  scope: EngineeringScope;
  editable: boolean;
  publishable: boolean;
  stateable: boolean;
  copyable: boolean;
  refresh: () => Promise<EngineeringDefinition>;
  close: () => void;
  copied: (id: string) => void;
}) {
  const [tab, setTab] = useState('definition');
  const { client, account, can } = useIdentity();
  const tenant = useOptionalTenant();
  const key = tenant
    ? ['tenant', account?.id, tenant.tenant.id, tenant.membership_id, 'engineering']
    : ['platform', account?.id, 'engineering'];
  const alive = useLivePage();
  const [definition, setDefinition] = useState(initial);
  const [versionId, setVersionId] = useState(initial.draft ? '' : (initial.versions[0]?.id ?? ''));
  const version = definition.versions.find((v) => v.id === versionId);
  const form = useForm<{ content: EngineeringContent }>({
    defaultValues: { content: initial.draft?.content ?? initial.versions[0]?.content ?? emptyContent(scope.kind) },
  });
  const content = useWatch({ control: form.control, name: 'content' });
  const displayed = version?.content ?? content;
  const dirty = !versionId && form.formState.isDirty;
  const [reason, setReason] = useState('');
  const [copyCode, setCopyCode] = useState('');
  const [notice, setNotice] = useState('');
  const [issues, setIssues] = useState<EngineeringIssue[]>([]);
  const [confirm, setConfirm] = useState('');
  const requestMeta = useRequestMeta();
  const [lastRequest, setLastRequest] = useState('');
  const meta = (action: string, payload: unknown, reason: string) => {
    const result = requestMeta(action, payload, reason);
    setLastRequest(result.request_id);
    return result;
  };
  const errorSummary = useRef<HTMLDivElement>(null);
  const [diffFrom, setDiffFrom] = useState(initial.versions[1]?.id ?? initial.versions[0]?.id ?? '');
  const [diffTo, setDiffTo] = useState(initial.draft ? '' : (initial.versions[0]?.id ?? ''));
  const diff = useQuery({
    queryKey: [...key, 'diff', scope.tenant, definition.id, definition.revision, diffFrom, diffTo],
    queryFn: ({ signal }) =>
      client.engineeringDiff(scope, definition.id, diffFrom, diffTo, definition.revision, signal),
    enabled: false,
  });
  const impact = useQuery({
    queryKey: [...key, 'impact', scope.tenant, definition.id, versionId],
    queryFn: ({ signal }) => client.engineeringImpact(scope, definition.id, versionId, signal),
    enabled: confirm === 'state' && !!versionId && !scope.catalog,
  });
  const operation = useOperation<EngineeringResult, 'save' | 'publish' | 'delete' | 'draft' | 'state' | 'copy'>({
    mutationFn: async (action) => {
      if (!reason.trim()) throw new Error('请填写本次变更原因。');
      const common = { revision: definition.revision };
      const id = definition.id;
      if (action === 'save') {
        const payload = { ...common, content: form.getValues('content') };
        return client.engineeringSave(scope, id, { ...payload, ...meta(action, [id, payload], reason) });
      }
      if (action === 'publish') {
        if (dirty) throw new Error('请先保存草稿。');
        return client.engineeringPublish(scope, id, { ...common, ...meta(action, [id, common], reason) });
      }
      if (action === 'delete')
        return client.engineeringDeleteDraft(scope, id, { ...common, ...meta(action, [id, common], reason) });
      if (action === 'draft') {
        const payload = { ...common, version_id: versionId };
        return client.engineeringNewDraft(scope, id, { ...payload, ...meta(action, [id, payload], reason) });
      }
      if (action === 'state' && version) {
        const payload = { revision: version.state_revision, enabled: !version.enabled };
        return client.engineeringState(scope, id, versionId, {
          ...payload,
          ...meta(action, [id, versionId, payload], reason),
        });
      }
      if (action === 'copy') {
        const payload = { code: copyCode, source_version: versionId };
        return client.engineeringCopy({ ...scope, catalog: false }, { ...payload, ...meta(action, payload, reason) });
      }
      throw new Error('请选择明确版本。');
    },
    onSuccess: async (result, action) => {
      setConfirm('');
      setIssues(result.issues);
      setNotice(
        action === 'publish'
          ? '发布成功，已生成不可变版本。'
          : action === 'copy'
            ? '已创建独立副本草稿。'
            : '操作成功。',
      );
      if (action === 'copy') {
        copied(result.definition_id);
        return;
      }
      if (action === 'delete' && !definition.versions.length) {
        close();
        return;
      }
      try {
        const next = await refresh();
        if (!alive.current) return;
        setDefinition(next);
        if (action === 'publish') setVersionId(result.version_id);
        if (action === 'draft') setVersionId('');
        if (action === 'delete') setVersionId(next.versions[0]?.id ?? '');
        form.reset({ content: next.draft?.content ?? next.versions[0]?.content ?? emptyContent(scope.kind) });
      } catch {
        if (alive.current) setNotice('操作已成功，但详情暂未刷新；请重新读取结果。');
      }
    },
    onError: (e) => {
      if (e instanceof ApiError) setIssues(e.issues);
      errorSummary.current?.focus();
    },
  });
  const receipt = useOperation({
    mutationFn: () => client.engineeringReceipt({ ...scope, catalog: false }, lastRequest),
    onSuccess: (v) =>
      setNotice(
        `请求已提交成功${v.version_id ? '，发布或状态版本：' + v.version_id : '，草稿修订：' + v.revision}。可重新读取结果。`,
      ),
  });
  const reload = useOperation({
    mutationFn: refresh,
    onSuccess: (next) => {
      setDefinition(next);
      form.reset({ content: next.draft?.content ?? next.versions[0]?.content ?? emptyContent(scope.kind) });
      setIssues([]);
      setNotice('已读取当前版本。');
      setConfirm('');
    },
  });
  const validation = useOperation({
    mutationFn: () =>
      client.engineeringPreview(scope, definition.id, {
        version_id: versionId,
        revision: definition.revision,
        type_version: '',
        unit_version: '',
      }),
    onSuccess: (v) => {
      setIssues(v.issues);
      setNotice(v.valid ? '定义校验通过；发布时将再次检查权限、修订和依赖。' : '定义尚有待完善项。');
      errorSummary.current?.focus();
    },
    onError: (e) => {
      if (e instanceof ApiError) setIssues(e.issues);
    },
  });
  const canAudit =
    !scope.catalog && (tenant ? tenant.permissions.includes(P.TenantAuditRead) : can(P.PlatformAuditRead));
  const busy = operation.isPending || reload.isPending;
  const update = (v: EngineeringContent) => form.setValue('content', v, { shouldDirty: true });
  const versionOptions = [
    ...(definition.draft ? [{ value: 'draft', label: `当前草稿 · 修订 ${definition.draft.revision}` }] : []),
    ...definition.versions.map((v) => ({
      value: v.id,
      label: `V${v.number} · ${v.content.name} · ${v.usable ? '可新使用' : '不可新使用'}`,
    })),
  ];
  const [picked, setPicked] = useState<EngineeringVersion[]>([]);
  const known = [...definition.dependencies, ...picked];
  const remember = (versions: EngineeringVersion[]) =>
    setPicked((previous) => [...previous.filter((v) => !versions.some((next) => next.id === v.id)), ...versions]);
  const modifying = !version && editable;
  return (
    <div className="min-w-0 space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0 space-y-3">
          <Button variant="ghost" className="-ml-3" onClick={close}>
            <ArrowLeft aria-hidden="true" />
            返回目录
          </Button>
          <p className="text-sm text-muted-foreground">{scope.tenant ? '本租户监测模板' : titles[scope.kind]}</p>
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="break-words text-2xl font-semibold tracking-tight">{displayed.name || '未命名草稿'}</h1>
            <Badge variant="outline">
              {version ? `V${version.number} · ${version.usable ? '可新使用' : '不可新使用'}` : '草稿'}
            </Badge>
          </div>
          <p className="text-sm text-muted-foreground">
            {definition.code}
            {definition.source_version && ' · 复制自公共版本'}
          </p>
        </div>
        <div className="w-full sm:w-72">
          <SelectControl
            label="查看草稿或版本"
            value={versionId || 'draft'}
            options={versionOptions}
            onChange={(id) => {
              setVersionId(id === 'draft' ? '' : id);
              setIssues([]);
              setNotice('');
            }}
          />
        </div>
      </div>
      {initial.revision !== definition.revision && (
        <p className="text-sm text-muted-foreground">目录有新修订。未保存输入已保留，可重新读取并比较。</p>
      )}
      <div ref={errorSummary} tabIndex={-1}>
        <FormNotice error>{operation.error?.message ?? reload.error?.message ?? validation.error?.message}</FormNotice>
        <Issues
          issues={issues}
          onNavigate={(path) => {
            setTab(
              path.startsWith('/fields')
                ? 'fields'
                : path.startsWith('/components') || path.startsWith('/relations')
                  ? 'components'
                  : path.startsWith('/metric')
                    ? 'rules'
                    : path.startsWith('/template/items') || path.startsWith('/template/groups')
                      ? 'items'
                      : path.startsWith('/template')
                        ? 'scope'
                        : 'definition',
            );
            requestAnimationFrame(() => {
              let current = path;
              while (current && !document.getElementById(current)) current = current.slice(0, current.lastIndexOf('/'));
              const target = document.getElementById(current);
              target?.scrollIntoView({ block: 'center' });
              target?.focus();
            });
          }}
        />
        {lastRequest && operation.error && (
          <div className="space-y-2 rounded-lg border p-3 text-sm">
            <p className="break-all">上次请求编号：{lastRequest}</p>
            <Button variant="outline" disabled={receipt.isPending} onClick={() => receipt.mutate()}>
              查询上次请求结果
            </Button>
            <FormNotice error>{receipt.error?.message}</FormNotice>
          </div>
        )}
        {notice && (
          <p role="status" className="rounded-lg border p-3 text-sm">
            {notice}
          </p>
        )}
      </div>
      <div className="flex flex-wrap items-end gap-3">
        {!versionId && (editable || publishable) && (
          <div className="w-full sm:max-w-sm">
            <TextControl label="本次变更原因" value={reason} onChange={setReason} />
          </div>
        )}
        <div className="flex flex-wrap gap-2">
          {!versionId && definition.draft && editable && (
            <Button disabled={busy} onClick={() => operation.mutate('save')}>
              保存草稿
            </Button>
          )}
          {!versionId && definition.draft && publishable && (
            <ConfirmDialog
              trigger={
                <Button variant="outline" disabled={busy || dirty}>
                  发布当前草稿
                </Button>
              }
              open={confirm === 'publish'}
              onOpenChange={(open) => setConfirm(open ? 'publish' : '')}
              title="直接发布当前草稿"
              description={`发布「${content.name}」，生成固定版本，供后续选用。`}
              pending={busy}
              error={operation.error?.message}
              onConfirm={() => operation.mutate('publish')}
            />
          )}
          {version && !definition.draft && editable && (
            <Button disabled={busy} onClick={() => setConfirm('draft')}>
              从此版本建立草稿
            </Button>
          )}
          {version && copyable && (
            <Button variant="outline" disabled={busy || !version.usable} onClick={() => setConfirm('copy')}>
              复制为独立定义
            </Button>
          )}
          <Button
            variant="outline"
            disabled={busy || dirty || validation.isPending}
            onClick={() => validation.mutate()}
          >
            校验定义
          </Button>
          {version && stateable && (
            <Button variant="outline" disabled={busy} onClick={() => setConfirm('state')}>
              {version.enabled ? '停用版本' : '恢复版本'}
            </Button>
          )}
          {!versionId && definition.draft && editable && (
            <ConfirmDialog
              trigger={
                <Button variant="ghost" disabled={busy}>
                  删除草稿
                </Button>
              }
              open={confirm === 'delete'}
              onOpenChange={(open) => setConfirm(open ? 'delete' : '')}
              title="删除当前草稿"
              description="草稿将被删除，已发布历史与副本保留。未发布对象的编号仍然保留。"
              pending={busy}
              error={operation.error?.message}
              destructive
              onConfirm={() => operation.mutate('delete')}
            />
          )}
          <ConfirmDialog
            trigger={
              <Button variant="ghost" disabled={busy}>
                重新读取结果
              </Button>
            }
            open={confirm === 'reload'}
            onOpenChange={(open) => setConfirm(open ? 'reload' : '')}
            title="重新读取当前结果"
            description="将替换本地未保存的定义编辑；请先核对是否需要保留。"
            pending={busy}
            error={reload.error?.message}
            onConfirm={() => reload.mutate()}
          />
        </div>
      </div>
      <ResponsivePanel
        open={['draft', 'copy', 'state'].includes(confirm)}
        onOpenChange={(open) => {
          if (!open && !busy) setConfirm('');
        }}
        title={
          confirm === 'copy'
            ? '复制为独立定义'
            : confirm === 'state'
              ? version?.enabled
                ? '停用当前版本'
                : '恢复当前版本'
              : '从此版本建立草稿'
        }
        description={
          confirm === 'copy'
            ? '复制当前版本，创建独立维护的新定义草稿。'
            : confirm === 'state'
              ? '调整此版本能否用于新的资源和定义。'
              : '已发布版本保持固定，在新草稿中编制修改。'
        }
      >
        <div className="space-y-4">
          {confirm === 'copy' && <TextControl label="副本新编号" value={copyCode} onChange={setCopyCode} />}
          <TextControl label="本次变更原因" value={reason} onChange={setReason} />
          {confirm === 'state' && (
            <>
              <p className="text-sm text-muted-foreground">
                {version?.enabled ? '停用将阻止后续新采用，历史内容保留。' : '依赖全部可用时才能恢复。'}
              </p>
              {impact.isFetching && <p role="status">正在读取引用影响…</p>}
              {impact.data && (
                <p className="text-sm">
                  当前可见引用 {impact.data.public.length} 个。
                  {impact.data.private_references ? '另有受限私有引用。' : ''}
                  {impact.data.resource_references ? '存在租户资源引用。' : ''}
                </p>
              )}
              <FormNotice error>{impact.error?.message}</FormNotice>
            </>
          )}
          <FormNotice error>{operation.error?.message}</FormNotice>
          <Button
            disabled={
              busy ||
              !reason.trim() ||
              (confirm === 'copy' && !copyCode.trim()) ||
              (confirm === 'state' && (impact.isFetching || !!impact.error))
            }
            onClick={() => {
              if (confirm === 'draft' || confirm === 'copy' || confirm === 'state') operation.mutate(confirm);
            }}
          >
            {confirm === 'copy' ? '创建副本草稿' : confirm === 'state' ? '确认变更' : '建立草稿'}
          </Button>
        </div>
      </ResponsivePanel>
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList className="flex h-auto flex-wrap justify-start">
          <TabsTrigger value="definition">基本资料</TabsTrigger>
          {scope.kind === 'type-definitions' && (
            <>
              <TabsTrigger value="fields">属性字段</TabsTrigger>
              <TabsTrigger value="components">构件规则</TabsTrigger>
            </>
          )}
          {scope.kind === 'metrics' && <TabsTrigger value="rules">取值规则</TabsTrigger>}
          {scope.kind === 'monitoring-templates' && (
            <>
              <TabsTrigger value="scope">适用范围</TabsTrigger>
              <TabsTrigger value="items">监测项</TabsTrigger>
            </>
          )}
          <TabsTrigger value="preview">样例预览</TabsTrigger>
          <TabsTrigger value="history">版本记录</TabsTrigger>
          {canAudit && <TabsTrigger value="audit">操作记录</TabsTrigger>}
        </TabsList>
        <TabsContent value="definition" className="space-y-5">
          {modifying ? (
            <fieldset disabled={busy} className="min-w-0 space-y-5">
              <Section title="基本资料">
                <div className="grid gap-4 sm:grid-cols-2">
                  <TextControl
                    id="/name"
                    label="定义名称"
                    value={displayed.name}
                    onChange={(name) => update({ ...content, name })}
                  />
                  <TextControl
                    label="版本发布说明"
                    value={displayed.release_notes}
                    onChange={(release_notes) => update({ ...content, release_notes })}
                  />
                </div>
                <TextControl
                  id="/description"
                  label="定义说明"
                  value={displayed.description}
                  onChange={(description) => update({ ...content, description })}
                  multiline
                />
              </Section>
              {scope.kind === 'units' && (
                <CatalogEditor onReferences={remember} value={content} onChange={update} known={known} />
              )}
            </fieldset>
          ) : (
            <Section title="基本资料">
              <DefinitionInfo
                values={[
                  { label: '定义名称', value: displayed.name },
                  { label: '稳定编号', value: definition.code },
                  { label: '定义说明', value: displayed.description },
                  { label: '版本发布说明', value: displayed.release_notes },
                  ...(displayed.unit
                    ? [
                        { label: '单位符号', value: displayed.unit.symbol },
                        { label: '单位类别', value: displayed.unit.category },
                      ]
                    : []),
                ]}
              />
            </Section>
          )}
        </TabsContent>
        {scope.kind === 'type-definitions' &&
          (['fields', 'components'] as const).map((section) => (
            <TabsContent key={section} value={section}>
              {modifying ? (
                <fieldset disabled={busy} className="min-w-0">
                  <TypeEditor
                    onReferences={remember}
                    section={section}
                    value={content}
                    onChange={update}
                    known={known}
                  />
                </fieldset>
              ) : (
                <DefinitionSnapshot content={displayed} section={section} known={known} />
              )}
            </TabsContent>
          ))}
        {scope.kind === 'metrics' && (
          <TabsContent value="rules">
            {modifying ? (
              <fieldset disabled={busy} className="min-w-0">
                <CatalogEditor onReferences={remember} value={content} onChange={update} known={known} />
              </fieldset>
            ) : (
              <DefinitionSnapshot content={displayed} section="rules" known={known} />
            )}
          </TabsContent>
        )}
        {content.template &&
          (['scope', 'items'] as const).map((section) => (
            <TabsContent key={section} value={section}>
              {modifying ? (
                <fieldset disabled={busy} className="min-w-0">
                  <TemplateEditor
                    onReferences={remember}
                    section={section}
                    value={content.template!}
                    onChange={(template) => update({ ...content, template })}
                    known={known}
                  />
                </fieldset>
              ) : (
                <DefinitionSnapshot content={displayed} section={section} known={known} />
              )}
            </TabsContent>
          ))}
        <TabsContent value="preview">
          <PreviewPanel
            key={versionId || 'draft'}
            scope={scope}
            definition={definition}
            content={displayed}
            version={version}
            dirty={dirty}
          />
        </TabsContent>
        <TabsContent value="history" className="space-y-4">
          <DirectoryTable
            label="版本历史"
            items={definition.versions}
            rowId={(v) => v.id}
            empty="尚未发布任何版本。"
            columns={[
              {
                id: 'version',
                label: '版本',
                className: 'w-24',
                cell: (v) => (
                  <Button
                    variant="link"
                    className="p-0 text-foreground"
                    onClick={() => {
                      setVersionId(v.id);
                      setTab('definition');
                    }}
                  >
                    V{v.number}
                  </Button>
                ),
              },
              {
                id: 'notes',
                label: '发布说明',
                cell: (v) => (
                  <div className="space-y-2">
                    <p>{v.content.release_notes || '未填写发布说明'}</p>
                    <p className="text-xs text-muted-foreground lg:hidden">
                      {new Date(v.published_at).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' })}
                    </p>
                    <Issues issues={v.issues} />
                  </div>
                ),
              },
              {
                id: 'time',
                label: '发布时间',
                secondary: true,
                cell: (v) => new Date(v.published_at).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' }),
              },
              {
                id: 'status',
                label: '可用状态',
                className: 'w-28',
                cell: (v) => (
                  <Badge variant="outline" className="whitespace-normal font-normal">
                    {v.usable ? '可新使用' : v.enabled ? '依赖不可用' : '已停用'}
                  </Badge>
                ),
              },
            ]}
          />
          {definition.source_version && (
            <Section title="复制来源">
              <p className="break-all text-sm">
                来源版本：
                {definition.source
                  ? `${definition.source.name} · ${definition.source.code} V${definition.source.number}`
                  : definition.source_version}
              </p>
              <p className="text-sm text-muted-foreground">来源只用于追溯，副本内容独立维护，不自动同步原件。</p>
            </Section>
          )}
          <Section title="固定依赖">
            {definition.dependencies.map((v) => (
              <p key={v.id} className="text-sm">
                {v.content.name} · {v.code} V{v.number} · {v.usable ? '可新使用' : '历史可解释，不能新采用'}
              </p>
            ))}
          </Section>
          {definition.versions.length > 0 && (
            <Section title="版本差异">
              <div className="grid gap-3 sm:grid-cols-2">
                <SelectControl
                  label="比较起点"
                  value={diffFrom}
                  onChange={setDiffFrom}
                  options={versionOptions.filter((v) => v.value !== 'draft')}
                />
                <SelectControl
                  label="比较终点"
                  value={diffTo || 'draft'}
                  onChange={(v) => setDiffTo(v === 'draft' ? '' : v)}
                  options={versionOptions}
                />
              </div>
              <Button type="button" variant="outline" onClick={() => void diff.refetch()}>
                比较明确版本
              </Button>
              {diff.error && <FormNotice error>{diff.error.message}</FormNotice>}
              {diff.data?.length === 0 && <p>两份内容相同。</p>}
              {diff.data?.map((v, i) => (
                <article key={i} className="space-y-2 rounded-lg border p-3 text-sm">
                  <p className="break-all font-medium">{v.path}</p>
                  <p className="break-all text-muted-foreground">之前：{JSON.stringify(v.before)}</p>
                  <p className="break-all">之后：{JSON.stringify(v.after)}</p>
                </article>
              ))}
            </Section>
          )}
        </TabsContent>
        {canAudit && (
          <TabsContent value="audit">
            <TenantAudit
              id={scope.tenant ?? ''}
              platform={!scope.tenant}
              membership={tenant?.membership_id}
              objectType="engineering_definition"
              objectId={definition.id}
            />
          </TabsContent>
        )}
      </Tabs>
    </div>
  );
}
