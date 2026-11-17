import { Link } from '@tanstack/react-router';
import { CheckCircle2, CircleAlert, Cpu, History, LockKeyhole, MapPin, Unplug } from 'lucide-react';
import type { DeviceSourceCurrent } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { cn } from '@infrapro/ui/lib/utils';
import { useDevices } from '../devices/context';
import { visibleOutput, type BindingRow } from './model';
import type { BindingEdit } from './editor';

export function CurrentBinding({ row, current, edit }: { row: BindingRow; current: DeviceSourceCurrent; edit: (value: BindingEdit) => void }) {
  const { scope, available, canRead, canSource } = useDevices();
  const output = visibleOutput(current.source);
  const restricted = current.source.configured && current.source.visibility === 'restricted';
  const attention = current.target_retired || !!output && (output.device_status !== 'enabled' || !!output.ended_at);
  const bound = current.source.configured;
  const Icon = restricted ? LockKeyhole : attention ? CircleAlert : bound ? CheckCircle2 : Unplug;
  const title = restricted ? '已绑定 · 设备详情受限' : bound ? (attention ? '当前绑定 · 需要处理' : '当前绑定 · 正在生效') : current.target_retired ? '尚未绑定 · 通道已退役' : '尚未绑定设备通道';
  return <section aria-label="当前绑定设备" className={cn('binding-current mx-3 mb-3 rounded-lg border p-3', bound && !restricted && !attention ? 'border-emerald-200 border-l-4 border-l-emerald-600 bg-emerald-50/80 dark:border-emerald-900 dark:border-l-emerald-500 dark:bg-emerald-950/30' : attention ? 'border-amber-300 border-l-4 border-l-amber-600 bg-amber-50/80 dark:border-amber-800 dark:bg-amber-950/30' : 'border-dashed bg-muted/30')}>
    <div className={cn('mb-3 flex items-center justify-between gap-3 text-sm font-semibold', !attention && bound && !restricted && 'text-emerald-800 dark:text-emerald-200', attention && 'text-amber-900 dark:text-amber-200')}><h3 className="flex items-center gap-2"><Icon className="size-4 shrink-0" aria-hidden="true" />{title}</h3>{bound && <Badge variant="outline" className="shrink-0 bg-background/60 font-normal">{restricted ? '详情受限' : attention ? '需处理' : '已绑定'}</Badge>}</div>
    {output ? <><div className="flex min-w-0 items-center gap-3"><div className="hidden size-10 shrink-0 items-center justify-center rounded-lg border bg-background/80 sm:flex"><Cpu className="size-5 text-muted-foreground" aria-hidden="true" /></div><div className="min-w-0"><p className="break-words text-sm font-medium">{output.device.name}</p><p className="mt-1 break-all text-lg font-semibold tracking-wide" data-current-device-code>{output.device.code}</p></div></div><p className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-current/10 pt-2 text-xs"><span className="break-words">{output.name}</span><span className="break-all text-muted-foreground">{output.slot_key}</span><span className="text-muted-foreground">第 {output.generation} 代</span></p></> : <p className="text-sm text-muted-foreground">{restricted ? '需取得原设备的查看和使用权限后，才能查看详情或调整关系。' : current.target_retired ? '业务通道及上级资源恢复为正常状态后，才能建立绑定。' : '选择下方可用设备通道，核对后建立绑定。'}</p>}
    {attention && <p className="mt-2 text-xs text-amber-900 dark:text-amber-200">{current.target_retired ? '业务通道或上级资源已退役，已有关系与历史仍保留。' : output?.ended_at ? '当前设备输出代次已结束，请核对或更换。' : '设备已停用，当前绑定仍保留，请核对或更换。'}</p>}
    <div className="mt-2 flex flex-wrap items-center gap-1"><Button variant="ghost" size="sm" className="min-h-9 px-2 text-xs" onClick={() => edit({ row })}><History className="size-3.5" aria-hidden="true" />关系与历史</Button><Button variant="ghost" size="sm" className="min-h-9 px-2 text-xs" asChild><Link to="/t/$tenantId/resources/facilities/$facilityId" params={{ tenantId: scope.tenantId, facilityId: row.facility_id }} search={{ point: row.point_id, view: 'bindings' }}><MapPin className="size-3.5" aria-hidden="true" />在设施中查看</Link></Button>{current.actions.unbind && canRead && canSource && <Button variant="ghost" size="sm" className="ml-auto min-h-9 px-2 text-xs text-destructive" disabled={!available} onClick={() => edit({ row, action: 'unbind' })}>解除绑定</Button>}</div>
  </section>;
}
