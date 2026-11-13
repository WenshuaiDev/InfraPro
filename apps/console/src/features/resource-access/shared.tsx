import { useState, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import {
  Permissions as P,
  type ResourceAccessInput,
  type ResourceAccessPreview,
  type ResourceKind,
} from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Checkbox } from '@infrapro/ui/components/checkbox';
import { Textarea } from '@infrapro/ui/components/textarea';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { useTenant } from '../tenancy/context';
import { useOperation } from '../account/use-operation';
import { requestUUID } from '../tenancy/membership-utils';
import { capabilities, capNames, describe, type Capability } from './model';
import { ChangeList, Hint, type Difference } from './presentation';

// eslint-disable-next-line react-refresh/only-export-components
export function useResourceAccess(project?: string) {
  const tenant = useTenant();
  const identity = useIdentity();
  return {
    ...identity,
    tenant,
    canRead: [
      P.TenantProjectRead,
      P.TenantEngineeringResourceRead,
      P.TenantEngineeringResourceAuthorizationRead,
    ].every((p) => tenant.permissions.includes(p)),
    canWrite:
      tenant.available &&
      [P.TenantEngineeringResourceManage, P.TenantEngineeringResourceAuthorizationManage].every((p) =>
        tenant.permissions.includes(p),
      ),
    prefix: [
      'tenant',
      identity.account?.id,
      tenant.tenant.id,
      tenant.membership_id,
      'resource-access',
      project,
    ] as const,
  };
}
export function CapabilityPicker({
  kind,
  value,
  onChange,
  disabled,
  original,
  archived = false,
}: {
  kind: ResourceKind;
  value: Capability[];
  onChange: (v: Capability[]) => void;
  disabled?: boolean;
  original?: Capability[];
  archived?: boolean;
}) {
  const sourceManage = useTenant().permissions.includes(P.TenantMonitoringSourceManage);
  const field = (c: Capability) => (
    <label key={c} className="flex min-h-11 items-center gap-3 rounded-md border px-3 py-2 text-sm">
      <Checkbox
        checked={value.includes(c)}
        disabled={
          disabled ||
          c === 'read' ||
          (c === 'source_manage' && !sourceManage) ||
          (archived && !original?.includes(c))
        }
        onCheckedChange={(yes) => onChange(yes ? [...value, c] : value.filter((v) => v !== c))}
      />
      {capNames[c]}
      {c === 'read' && <span className="ml-auto text-xs text-muted-foreground">已包含</span>}
    </label>
  );
  return (
    <div className="space-y-3">
      {capabilities[kind].filter((c) => c !== 'lifecycle' && c !== 'source_manage').map(field)}
      {kind !== 'facility' && (
        <details className="rounded-lg border px-3 py-2">
          <summary className="cursor-pointer py-1 text-sm">更多操作权限</summary>
          <div className="mt-3 space-y-2">
            {capabilities[kind].filter((c) => c === 'lifecycle' || c === 'source_manage').map(field)}
            <p className="text-xs leading-relaxed text-muted-foreground">
              退役与恢复受资源状态和共享使用限制。维护通道来源还需独立的设备输出授权。
              {!sourceManage && kind === 'channel' && '你没有调整来源权限的资格。'}
            </p>
          </div>
        </details>
      )}
      {value.some((c) => c.startsWith('create_')) && (
        <p className="text-xs leading-relaxed text-muted-foreground">
          项目可新增并维护自己创建的下级资源；已有的其他下级仍需单独授权。
        </p>
      )}
    </div>
  );
}
function previewDifferences(preview: ResourceAccessPreview): Difference[] {
  return [
    ...preview.add.map((g) => ({
      id: g.resource_id,
      name: g.name,
      code: g.code,
      kind: g.kind,
      before: '未授权',
      after: describe(g.capabilities),
      action: 'add' as const,
    })),
    ...preview.update.map((g) => ({
      id: g.resource_id,
      name: g.name,
      code: g.code,
      kind: g.kind,
      before: describe(preview.before.find((b) => b.id === g.id)?.capabilities),
      after: describe(g.capabilities),
      action: 'update' as const,
    })),
    ...preview.end.map((g) => ({
      id: g.resource_id,
      name: g.name,
      code: g.code,
      kind: g.kind,
      before: describe(g.capabilities),
      after: '收回当前项目的授权',
      action: 'end' as const,
    })),
  ];
}
export function AccessCommit({
  project,
  facility,
  build,
  disabled,
  onSaved,
  draftKey = '',
  summary,
  destructive = false,
  label = '预览并保存授权',
}: {
  project: string;
  facility?: string;
  build: (reason: string, request: string) => ResourceAccessInput;
  disabled?: boolean;
  onSaved: () => void;
  draftKey?: string;
  summary?: ReactNode;
  destructive?: boolean;
  label?: string;
}) {
  const { client, tenant, prefix, canWrite } = useResourceAccess(project);
  const cache = useQueryClient();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState('');
  const [staged, setStaged] = useState<{
    input: ResourceAccessInput;
    preview: ResourceAccessPreview;
    key: string;
  }>();
  const saved = () => {
    setOpen(false);
    setStaged(undefined);
    setReason('');
    void cache.invalidateQueries({ queryKey: prefix.slice(0, -1) });
    void cache.invalidateQueries({ queryKey: ['project-context'] });
    onSaved();
  };
  const preview = useOperation({
    mutationFn: async () => {
      const input = build(reason.trim(), requestUUID());
      return {
        input,
        key: draftKey,
        preview: await client.previewResourceAccess(tenant.tenant.id, project, facility, input),
      };
    },
    onSuccess: setStaged,
  });
  const commit = useOperation({
    mutationFn: () => {
      if (!staged) throw new Error('请重新预览。');
      return client.changeResourceAccess(tenant.tenant.id, project, facility, {
        ...staged.input,
        preview_hash: staged.preview.preview_hash,
      });
    },
    onSuccess: saved,
  });
  const receipt = useOperation({
    mutationFn: () => {
      if (!staged) throw new Error('尚无提交请求。');
      return client.resourceAccessReceipt(tenant.tenant.id, project, staged.input.request_id);
    },
    onSuccess: saved,
  });
  const current = staged?.key === draftKey ? staged : undefined;
  const busy = preview.isPending || commit.isPending || receipt.isPending;
  const error = preview.error || commit.error || receipt.error;
  return (
    <>
      <Button
        type="button"
        variant={destructive ? 'outline' : 'default'}
        className={destructive ? 'text-destructive' : undefined}
        disabled={disabled || !canWrite}
        onClick={() => setOpen(true)}
      >
        {label}
      </Button>
      <ResponsivePanel
        open={open}
        onOpenChange={(v) => {
          if (!busy) setOpen(v);
        }}
        title={destructive ? '确认将设施移出项目' : '核对授权变更'}
        description="填写原因并预览实际影响，核对后再保存。"
      >
        <div className="space-y-5">
          <Hint>
            {destructive
              ? '将收回当前项目对这座设施及其下级的全部授权。租户资源档案及其他项目的授权保留。'
              : '授权给项目后，成员还需对应的项目角色权限才能执行操作。修改共享资料会影响使用同一资源的其他项目。'}
          </Hint>
          {!current && summary}
          <label className="block space-y-2 text-sm font-medium">
            授权变更原因 <span className="text-destructive">*</span>
            <Textarea
              value={reason}
              maxLength={2000}
              disabled={busy || !!current}
              onChange={(e) => setReason(e.target.value)}
              placeholder="例如：开放本期监测所需的测点，由项目团队负责维护。"
              rows={3}
            />
          </label>
          {error && (
            <FormNotice error>
              {error.message} 当前输入已保留。若授权已更新，请关闭面板并重新载入后复核。
            </FormNotice>
          )}
          {current && (
            <section aria-label="授权影响确认" className="space-y-4">
              <h3 className="font-semibold">{current.preview.policy.project_name}</h3>
              {current.input.action === 'policy' ? (
                <ChangeList
                  items={[
                    {
                      id: 'policy',
                      name: '新建设施权限',
                      before: current.preview.policy.allow_facility_create
                        ? '允许新建设施'
                        : '不允许新建设施',
                      after: current.input.allow_facility_create
                        ? '允许新建设施，并维护新建资源'
                        : '不允许新建设施；已有授权保留',
                      action: 'update',
                    },
                  ]}
                />
              ) : (
                <>
                  <p className="text-sm">
                    新增 {current.preview.add.length} 项 · 调整 {current.preview.update.length} 项 · 收回{' '}
                    {current.preview.end.length} 项
                  </p>
                  <ChangeList items={previewDifferences(current.preview)} />
                  {current.preview.retained_context.length > 0 && (
                    <Hint>
                      以下资源仅保留用于定位下级的名称和路径，不再开放完整档案：
                      {current.preview.retained_context.map((v) => v.name).join('、')}。
                    </Hint>
                  )}
                </>
              )}
              {!current.preview.changed && <Hint>没有需要保存的变化。</Hint>}
            </section>
          )}
          <div className="flex flex-wrap justify-end gap-2 border-t pt-4">
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => {
                if (current) {
                  setStaged(undefined);
                  commit.reset();
                  receipt.reset();
                } else setOpen(false);
              }}
            >
              返回修改
            </Button>
            {current ? (
              <Button
                variant={destructive ? 'destructive' : 'default'}
                disabled={busy || !canWrite || !current.preview.changed}
                onClick={() => commit.mutate()}
              >
                {commit.isPending ? '保存中…' : destructive ? '确认移出项目' : '确认保存'}
              </Button>
            ) : (
              <Button
                disabled={busy || disabled || !canWrite || !reason.trim()}
                onClick={() => preview.mutate()}
              >
                {preview.isPending ? '正在计算影响…' : '预览实际影响'}
              </Button>
            )}
            {commit.error && (
              <Button variant="outline" disabled={busy} onClick={() => receipt.mutate()}>
                核对原请求结果
              </Button>
            )}
          </div>
        </div>
      </ResponsivePanel>
    </>
  );
}
