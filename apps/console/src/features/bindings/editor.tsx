import { useEffect, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import type { DeviceOutput, DeviceSourceCurrent } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@infrapro/ui/components/dialog';
import { ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { useDevices } from '../devices/context';
import { CommandFeedback, useDeviceCommand } from '../devices/command';
import { DeviceField } from '../devices/fields';
import { CandidatePicker } from './candidates';
import { BindingHistory } from './history';
import { actionLabels, bindingCommand, targetOf, visibleOutput, type BindingAction, type BindingCommand, type BindingRow } from './model';
import { CurrentEquipment, PairSummary, DesignMeaning } from './shared';

export type BindingEdit = { row: BindingRow; action?: BindingAction; output?: DeviceOutput };
export function BindingDialog({ edit, close, saved }: { edit: BindingEdit; close: () => void; saved: (message: string) => void }) {
  const { client, scope, prefix } = useDevices();
  const [action, setAction] = useState(edit.action);
  const [locked, setLocked] = useState(false);
  const trigger = useRef<HTMLElement | null>(document.activeElement instanceof HTMLElement ? document.activeElement : null);
  const target = targetOf(edit.row);
  const current = useQuery({ queryKey: [...prefix, 'source', target], queryFn: ({ signal }) => client.deviceSource(scope, target, signal), refetchOnMount: 'always' });
  return <Dialog open onOpenChange={open => { if (!open && !locked) close(); }}><DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-2xl" showCloseButton={!locked} onCloseAutoFocus={event => { if (trigger.current?.isConnected) { event.preventDefault(); trigger.current.focus(); } }} onPointerDownOutside={event => event.preventDefault()}><DialogHeader><DialogTitle>{action ? actionLabels[action] : '查看绑定关系'}</DialogTitle><DialogDescription>{edit.row.facility_name} / {edit.row.point_name} / {edit.row.name}</DialogDescription></DialogHeader>
    {current.isPending ? <LoadingState /> : current.error || !current.data ? <ErrorState message={current.error?.message ?? '无法读取当前绑定'} onRetry={() => void current.refetch()} /> : action ? <BindingForm key={action} row={edit.row} action={action} current={current.data} preferred={edit.output} lock={setLocked} cancel={close} saved={saved} refreshCurrent={async () => { await current.refetch({ throwOnError: true }); }} /> : <div className="space-y-5"><PairSummary row={edit.row} output={visibleOutput(current.data.source)} /><DesignMeaning row={edit.row} current={current.data} /><BindingControls current={current.data} choose={setAction} /><BindingHistory target={target} /></div>}
  </DialogContent></Dialog>;
}

export function BindingControls({ current, choose }: { current: DeviceSourceCurrent; choose: (action: BindingAction) => void }) {
  const { canRead, canSource, available } = useDevices();
  return <div className="flex flex-wrap gap-2">{canRead && canSource ? (['bind', 'switch', 'unbind'] as const).filter(action => current.actions[action]).map(action => <Button key={action} className="min-h-11" variant={action === 'bind' ? 'default' : 'outline'} disabled={!available} onClick={() => choose(action)}>{actionLabels[action]}</Button>) : <p className="text-sm text-muted-foreground">当前为只读查看；绑定维护需要设备查看和来源维护权限。</p>}{current.source.visibility === 'restricted' && <p className="text-sm text-muted-foreground">当前设备详情受限，请取得原来源的访问权限后操作。</p>}</div>;
}
function BindingForm({ row, action, current, preferred, lock, cancel, saved, refreshCurrent }: { row: BindingRow; action: BindingAction; current: DeviceSourceCurrent; preferred?: DeviceOutput; lock: (value: boolean) => void; cancel: () => void; saved: (message: string) => void; refreshCurrent: () => Promise<void> }) {
  const { client, scope, prefix, available, canRead, canSource } = useDevices();
  const cache = useQueryClient();
  const [selection, setSelection] = useState(preferred);
  const [reason, setReason] = useState('');
  const [selecting, setSelecting] = useState(!preferred);
  const target = targetOf(row);
  const selected = useQuery({ queryKey: [...prefix, 'binding-selection', selection?.id], enabled: !!selection && action !== 'unbind',
    queryFn: ({ signal }) => client.deviceOutput(scope, selection!.id, selection!.device.id, signal), refetchOnMount: 'always' });
  const output = selected.data;
  const writable = available && canRead && canSource && current.actions[action];
  const outputUsable = action === 'unbind' || !!output && output.device_status === 'enabled' && !output.ended_at && !output.occupied && !selected.error && !selected.isFetching;
  const command = useDeviceCommand<BindingCommand>({ available: writable,
    preview: input => client.previewDeviceSource(scope, target, input), write: input => client.changeDeviceSource(scope, target, input), receipt: id => client.deviceReceipt(scope, id),
    refresh: async () => { await refreshCurrent(); if (selection) await selected.refetch({ throwOnError: true }); await cache.invalidateQueries({ queryKey: prefix }); },
    done: () => {
      // Every view (including device-side reverse relations and project copies)
      // must observe the same server relation on its next read.
      void cache.invalidateQueries({ predicate: q => q.queryKey.includes(scope.tenantId) });
      saved(`${row.name}：${action === 'bind' ? '已绑定设备' : action === 'switch' ? '已更换设备，原关系已保留在历史中' : '已解除绑定，历史记录已保留'}`);
    },
  });
  useEffect(() => { lock(command.pending || command.uncertain); return () => lock(false); }, [command.pending, command.uncertain, lock]);
  const frozen = command.pending || !!command.stage || command.uncertain;
  const submit = () => {
    if (!reason.trim()) { command.setIssues([{ path: '/reason', code: 'required', message: '请填写本次绑定变更原因。' }]); return; }
    const input = bindingCommand(scope, action, current, reason, output);
    if (input && outputUsable) command.submit(input);
  };
  return <div className="space-y-4">{!writable && <FormNotice>当前状态或权限已变化，无法执行本次操作。请返回查看最新绑定关系。</FormNotice>}
    {action !== 'bind' && <section className="space-y-2 rounded-lg border p-4"><h3 className="text-xs text-muted-foreground">当前绑定</h3><CurrentEquipment source={current.source} /></section>}
    {!command.stage && <><fieldset className="min-w-0 space-y-4" disabled={frozen || !writable}>{action !== 'unbind' && <><>{selecting ? <CandidatePicker target={target} current={current} selected={selection?.id} choose={output => { setSelection(output); setSelecting(false); }} /> : <Button variant="outline" className="min-h-11" onClick={() => setSelecting(true)}>重新选择设备通道</Button>}</>{selected.isFetching && selection && <LoadingState label="正在核对所选设备通道…" />}{selected.error && <ErrorState message={selected.error.message} onRetry={() => void selected.refetch()} />}{output && !outputUsable && !selected.isFetching && <FormNotice error>所选设备通道已不可用。请重新选择或读取最新记录。</FormNotice>}</>}<PairSummary row={row} output={output} unbind={action === 'unbind'} /><DeviceField name="reason" label="变更原因 *" multiline value={reason} onChange={setReason} issues={command.issues} hint="例如：现场接线核对完成；原设备检修，启用备用设备。" /></fieldset><p className="text-sm text-muted-foreground">提交后立即生效；历史观测仍按原绑定时间归属。{action === 'switch' && '旧关系结束和新关系开始使用同一时刻。'}{action === 'unbind' && '该业务通道将暂不关联设备。'}设备安装记录独立保留。</p><div className="flex flex-wrap gap-2"><Button className="min-h-11" disabled={frozen || !writable || !outputUsable} onClick={submit}>预览并核对</Button><Button className="min-h-11" variant="outline" disabled={frozen} onClick={cancel}>取消</Button></div></>}
    <CommandFeedback command={command}><PairSummary row={row} output={output} unbind={action === 'unbind'} /><p className="whitespace-pre-wrap break-words text-sm">变更原因：{reason}</p><p className="text-sm">本次{actionLabels[action]}立即生效。{action === 'switch' ? '旧绑定结束与新绑定开始使用同一时间边界。' : action === 'unbind' ? '解除后该通道暂不关联设备。' : ''}历史记录与设备安装记录保留。</p></CommandFeedback>
  </div>;
}
