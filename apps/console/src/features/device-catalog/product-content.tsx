import { Fragment, useId, useState } from 'react';
import { ChevronDown, ChevronRight } from 'lucide-react';
import type { DeviceProductContent, DeviceProductSlot, EngineeringVersion } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@infrapro/ui/components/table';
import { EmptyState } from '@infrapro/ui/patterns/feedback';

const shapes = { integer: '整数', decimal: '小数', enum: '枚举' };
const timestampLabel = (value: string) => value === 'interval_end' ? '统计区间结束时刻' : '观测时刻';
function definitionLabel(id: string, definitions: EngineeringVersion[], fallback: string) {
  const definition = definitions.find((item) => item.id === id);
  return definition ? `${definition.content.name} · v${definition.number}` : `${fallback}（${id || '未选择'}）`;
}

export function ProductInformation({ content }: { content: DeviceProductContent }) {
  return <dl className="grid gap-6 rounded-xl border p-5 sm:grid-cols-2">
    {(['name', 'model', 'purpose', 'description', 'release_notes'] as const).map((key) => <div key={key} className={`min-w-0 space-y-2 ${key === 'description' || key === 'release_notes' ? 'sm:col-span-2' : ''}`}>
      <dt className="text-sm text-muted-foreground">{({ name: '产品名称', model: '型号', purpose: '用途', description: '产品说明', release_notes: '版本说明' })[key]}</dt>
      <dd className="whitespace-pre-wrap break-words text-sm leading-relaxed">{content[key] || '未填写'}</dd>
    </div>)}
  </dl>;
}

export function ProductOutputs({ content, definitions }: { content: DeviceProductContent; definitions: EngineeringVersion[] }) {
  const [expanded, setExpanded] = useState<string[]>([]);
  const id = useId();
  if (!content.output_slots.length) return <EmptyState title="此产品没有输出槽" description="安装附件等产品可以不定义数据输出。" />;
  return <div className="space-y-4">
    <div><h3 className="font-semibold">输出定义</h3><p className="mt-1 text-sm text-muted-foreground">按输出槽对照观测含义，展开查看说明。可配置槽在设备上确定具体含义。</p></div>
    <div className="overflow-hidden rounded-lg border"><Table aria-label="产品输出槽" className="min-w-[44rem]">
      <TableHeader><TableRow className="bg-muted/40"><TableHead className="px-4">输出名称 / 标识</TableHead><TableHead>模式</TableHead><TableHead>指标 / 允许形态</TableHead><TableHead>单位</TableHead><TableHead>时间含义</TableHead><TableHead><span className="sr-only">补充说明</span></TableHead></TableRow></TableHeader>
      <TableBody>{content.output_slots.map((slot, index) => {
        const open = expanded.includes(slot.key);
        return <Fragment key={slot.key}><TableRow>
          <TableCell className="max-w-56 whitespace-normal px-4 py-4"><p className="break-words font-medium">{slot.name}</p><p className="mt-1 break-all font-mono text-xs text-muted-foreground">{slot.key}</p></TableCell>
          <TableCell><Badge variant="outline">{slot.mode === 'fixed' ? '固定' : '可配置'}</Badge></TableCell>
          <TableCell className="max-w-64 break-words whitespace-normal">{slot.mode === 'fixed' ? definitionLabel(slot.metric_version_id, definitions, '固定指标版本') : slot.allowed_shapes.map((shape) => shapes[shape]).join('、')}</TableCell>
          <TableCell className="max-w-48 break-words whitespace-normal">{slot.mode === 'fixed' ? slot.unit_version_id ? definitionLabel(slot.unit_version_id, definitions, '固定单位版本') : '无单位（枚举）' : '配置时确定'}</TableCell>
          <TableCell>{slot.mode === 'fixed' ? timestampLabel(slot.timestamp_meaning) : '配置时确定'}</TableCell>
          <TableCell><Button variant="ghost" size="icon" aria-label={`${open ? '收起' : '展开'}${slot.name}说明`} aria-expanded={open} aria-controls={`${id}-${index}`} onClick={() => setExpanded(open ? expanded.filter((key) => key !== slot.key) : [...expanded, slot.key])}>{open ? <ChevronDown aria-hidden="true" /> : <ChevronRight aria-hidden="true" />}</Button></TableCell>
        </TableRow><TableRow id={`${id}-${index}`} hidden={!open} className="bg-muted/30"><TableCell colSpan={6} className="whitespace-normal px-4 py-4"><dl className="grid gap-4 text-sm sm:grid-cols-2"><div><dt className="mb-1 text-muted-foreground">输出槽说明</dt><dd className="whitespace-pre-wrap break-words">{slot.description || '未填写'}</dd></div>{slot.mode === 'fixed' && <div><dt className="mb-1 text-muted-foreground">统计说明</dt><dd className="whitespace-pre-wrap break-words">{slot.aggregation_description || '未填写'}</dd></div>}</dl></TableCell></TableRow></Fragment>;
      })}</TableBody>
    </Table></div>
  </div>;
}

function slotFields(slot: DeviceProductSlot, definitions: EngineeringVersion[]): Record<string, string> {
  return {
    名称: slot.name, 模式: slot.mode === 'fixed' ? '固定' : '可配置', 说明: slot.description ?? '', 顺序: String(slot.order ?? 0),
    ...(slot.mode === 'fixed' ? { 指标: definitionLabel(slot.metric_version_id, definitions, '固定指标版本'), 单位: slot.unit_version_id ? definitionLabel(slot.unit_version_id, definitions, '固定单位版本') : '无单位（枚举）', 时间含义: timestampLabel(slot.timestamp_meaning), 统计说明: slot.aggregation_description ?? '' }
      : { 允许形态: slot.allowed_shapes.map((shape) => shapes[shape]).join('、') }),
  };
}

export function ProductDiff({ before, after, definitions, beforeLabel, afterLabel }: {
  before: DeviceProductContent; after: DeviceProductContent; definitions: EngineeringVersion[]; beforeLabel: string; afterLabel: string;
}) {
  const rows: { name: string; before: string; after: string }[] = [];
  for (const key of ['name', 'model', 'purpose', 'description', 'release_notes'] as const) {
    if (before[key] !== after[key]) rows.push({ name: ({ name: '产品名称', model: '型号', purpose: '用途', description: '产品说明', release_notes: '版本说明' })[key], before: before[key], after: after[key] });
  }
  const keys = new Set([...before.output_slots, ...after.output_slots].map((slot) => slot.key));
  for (const key of keys) {
    const a = before.output_slots.find((slot) => slot.key === key);
    const b = after.output_slots.find((slot) => slot.key === key);
    const left = a ? slotFields(a, definitions) : {};
    const right = b ? slotFields(b, definitions) : {};
    for (const field of new Set([...Object.keys(left), ...Object.keys(right)])) {
      if (left[field] !== right[field]) rows.push({ name: `${b?.name ?? a?.name}（${key}）· ${field}`, before: a ? left[field] ?? '不适用' : '未定义', after: b ? right[field] ?? '不适用' : '已移除' });
    }
  }
  return rows.length ? <div className="overflow-hidden rounded-lg border"><Table aria-label="产品版本差异" className="min-w-[34rem] table-fixed"><TableHeader><TableRow className="bg-muted/40"><TableHead className="w-1/4 px-4">变更字段</TableHead><TableHead>{beforeLabel}</TableHead><TableHead>{afterLabel}</TableHead></TableRow></TableHeader><TableBody>{rows.map((row) => <TableRow key={row.name}><TableCell className="whitespace-normal break-words px-4 py-4 align-top font-medium">{row.name}</TableCell><TableCell className="whitespace-pre-wrap break-words py-4 align-top text-muted-foreground">{row.before || '未填写'}</TableCell><TableCell className="whitespace-pre-wrap break-words py-4 align-top">{row.after || '未填写'}</TableCell></TableRow>)}</TableBody></Table></div>
    : <EmptyState title="内容相同" description="所选两个版本没有产品信息或输出槽定义的变化。" />;
}
