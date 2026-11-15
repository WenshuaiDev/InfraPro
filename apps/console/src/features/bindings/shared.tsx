import { ArrowRight, Check, CircleAlert, CircleDot, Cpu, LockKeyhole, Unplug } from 'lucide-react';
import type { DeviceOutput, DeviceSourceCurrent } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { cn } from '@infrapro/ui/lib/utils';
import { stateLabels, visibleOutput, type BindingRow } from './model';

export function BindingStatus({ state }: { state: BindingRow['state'] }) {
  const Icon = state === 'bound' ? Check : state === 'attention' ? CircleAlert : state === 'restricted' ? LockKeyhole : Unplug;
  return <Badge variant="outline" className={cn('shrink-0 gap-1 font-normal', state === 'attention' ? 'border-destructive/40 text-destructive' : state === 'bound' ? 'bg-primary/5' : 'bg-muted/40 text-muted-foreground')}><Icon className="size-3" aria-hidden="true" />{stateLabels[state]}</Badge>;
}
export function ChannelIdentity({ row, location = true }: { row: BindingRow; location?: boolean }) {
  return <div className="min-w-0 space-y-1">{location && <p className="break-words text-xs text-muted-foreground">{row.facility_name} / {row.point_name}</p>}<p className="break-words font-medium">{row.name}</p><p className="break-words text-xs text-muted-foreground">{row.code} · {row.metric_name}{row.unit_symbol && ` · ${row.unit_symbol}`}</p></div>;
}
export function Equipment({ output }: { output: DeviceOutput }) {
  return <div className="min-w-0 space-y-1"><p className="break-words text-sm font-medium">{output.device.name}</p><p className="break-words text-xs text-muted-foreground">{output.name} · {output.slot_key} · 第 {output.generation} 代</p><p className="break-all text-xs text-muted-foreground">{output.device.code}</p>{(output.device_status !== 'enabled' || output.ended_at) && <p className="text-xs text-destructive">{output.ended_at ? '该输出代次已结束' : '设备已停用'}，请核对或更换</p>}</div>;
}
export function CurrentEquipment({ source }: { source: DeviceSourceCurrent['source'] }) {
  const output = visibleOutput(source);
  return output ? <Equipment output={output} /> : <p className="text-sm text-muted-foreground">{source.configured ? '已绑定，设备详情受权限限制' : '尚未绑定设备通道'}</p>;
}
export function PairSummary({ row, output, unbind = false }: { row: BindingRow; output?: DeviceOutput; unbind?: boolean }) {
  return <div className="grid min-w-0 items-center gap-4 rounded-lg border bg-muted/20 p-4 sm:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]" aria-label="本次配对关系"><div className="min-w-0"><p className="mb-2 flex items-center gap-2 text-xs text-muted-foreground"><CircleDot className="size-4" aria-hidden="true" />业务通道 · 测什么</p><ChannelIdentity row={row} /></div><ArrowRight className="size-5 rotate-90 text-muted-foreground sm:rotate-0" aria-hidden="true" /><div className="min-w-0"><p className="mb-2 flex items-center gap-2 text-xs text-muted-foreground"><Cpu className="size-4" aria-hidden="true" />设备通道 · 谁来测</p>{unbind ? <p className="text-sm">解除后暂不关联设备</p> : output ? <Equipment output={output} /> : <p className="text-sm text-muted-foreground">请选择设备通道</p>}</div></div>;
}
export function PageButtons({ page, hasNext, busy, previous, next, label }: { page: number; hasNext: boolean; busy: boolean; previous: () => void; next: () => void; label: string }) {
  return <div className="flex flex-wrap items-center justify-between gap-2 border-t pt-3"><Button className="min-h-11" variant="ghost" disabled={!page || busy} onClick={previous}>上一页{label}</Button><span className="text-xs text-muted-foreground">第 {page + 1} 页</span><Button className="min-h-11" variant="ghost" disabled={!hasNext || busy} onClick={next}>下一页{label}</Button></div>;
}
export function DesignMeaning({ row, current }: { row: BindingRow; current: DeviceSourceCurrent }) {
  const s = current.semantics;
  return <details className="rounded-lg border p-3"><summary className="min-h-8 cursor-pointer text-sm font-medium">设计要求 · 观测含义</summary><dl className="mt-3 grid gap-3 text-sm sm:grid-cols-2">{[['指标', row.metric_name], ['单位', row.unit_symbol || '无量纲'], ['值类型', s.value_shape === 'decimal' ? '数值' : s.value_shape === 'enum' ? '枚举状态' : s.value_shape], ['时间含义', s.timestamp_meaning === 'observation_time' ? '观测时刻' : s.timestamp_meaning === 'interval_end' ? '统计时段结束' : s.timestamp_meaning], ['聚合说明', s.aggregation_description || '无'], ['观测方向', s.observation_direction || '无'], ['参考基准', s.reference_basis || '无']].map(([label, value]) => <div key={label}><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1 break-words">{value}</dd></div>)}</dl></details>;
}
