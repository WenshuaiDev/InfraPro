import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { useState } from 'react';
import { Check, Cpu } from 'lucide-react';
import { deviceSemanticDifferences, deviceSemanticLabels, type DeviceOutput, type DeviceSourceCurrent, type DeviceSourceTarget } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Checkbox } from '@infrapro/ui/components/checkbox';
import { DirectorySearch } from '@infrapro/ui/patterns/directory-table';
import { EmptyState, ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { cn } from '@infrapro/ui/lib/utils';
import { useCursorList } from '../../hooks/use-cursor-list';
import { useDevices } from '../devices/context';
import { OutputMeaning } from '../devices/fields';
import { Equipment } from './shared';

export function CandidatePicker({ target, current, selected, choose, fill = false }: { target: DeviceSourceTarget; current: DeviceSourceCurrent; selected?: string; choose: (output: DeviceOutput) => void; fill?: boolean }) {
  const { client, scope, prefix, available, canSource, canRead } = useDevices();
  const [search, setSearch] = useState('');
  const [compatible, setCompatible] = useState(true);
  const enabled = canRead && canSource && (current.actions.bind || current.actions.switch);
  const list = useCursorList({ initialPageSize: 8, queryKey: [...prefix, 'source-candidates', target, current.source_revision, search, compatible], enabled,
    load: (cursor, signal, limit) => client.deviceSourceCandidates(scope, target, { q: search.trim(), compatible_only: compatible, cursor, limit }, signal) });
  if (!enabled) return <EmptyState title={current.target_retired ? '通道或上级资源已退役' : '当前绑定不可维护'} description={current.source.visibility === 'restricted' ? '需取得原设备的查看和使用权限后才能更换或解除。' : '需要设备查看及来源维护权限；退役通道可保留历史或解除已有关系。'} />;
  return <div className={cn(fill ? 'flex h-full min-h-0 flex-col' : 'space-y-4')}><div className={cn('shrink-0 space-y-2', fill && 'border-b px-3 pb-3')}><div className="flex items-center justify-between gap-3"><h3 className="text-sm font-semibold">{current.source.configured ? '更换设备通道' : '选择可用设备通道'}</h3><span className="text-xs text-muted-foreground">可用设备优先</span></div><div className="flex flex-wrap items-center gap-x-3 gap-y-1"><div className="min-w-0 flex-1 [&>div]:w-full"><DirectorySearch label="搜索设备名称或编号" value={search} onChange={setSearch} /></div><label className="flex min-h-9 cursor-pointer items-center gap-2 text-xs"><Checkbox checked={compatible} onCheckedChange={v => setCompatible(v === true)} />仅看观测含义一致</label></div></div>
    <div className={cn(fill ? 'binding-candidates-scroll p-3' : 'max-h-[24rem] overflow-y-auto pr-1')} role="region" aria-label="候选设备通道" tabIndex={0}>
      {list.isPending ? <LoadingState label="正在查找设备通道…" /> : list.error ? <ErrorState message={list.error.message} onRetry={() => { list.go(0); void list.refetch(); }} /> : !list.items.length ? <EmptyState title={current.source.configured ? '没有其他符合条件的设备通道' : '没有符合条件的设备通道'} description={search ? '试试其他设备名称或编号，或关闭兼容筛选查看具体差异。' : current.source.configured ? '当前设备已在上方单独显示。可调整筛选，或先在设备台账登记备用输出。' : '请先在设备台账登记设备并配置输出的观测含义，再回到这里绑定。'} /> : <div className="space-y-2">{list.items.map(candidate => <article key={candidate.output.id} className={cn('space-y-2 rounded-lg border p-3', !candidate.available && 'bg-muted/15', selected === candidate.output.id && 'border-primary/60 bg-primary/5')}><div className="flex items-start justify-between gap-3"><div className="flex min-w-0 gap-2"><Cpu className="mt-1 size-4 shrink-0 text-muted-foreground" aria-hidden="true" /><Equipment output={candidate.output} /></div><Button className="min-h-9 shrink-0" size="sm" variant={selected === candidate.output.id ? 'default' : 'outline'} aria-pressed={selected === candidate.output.id} aria-label={`选择 ${candidate.output.device.name} / ${candidate.output.name}`} disabled={!available || !candidate.available || !candidate.compatible} onClick={() => choose(candidate.output)}>{selected === candidate.output.id ? <><Check className="size-4" aria-hidden="true" />已选</> : candidate.available ? '选择' : candidate.output.occupied ? '已占用' : '不可选'}</Button></div>{candidate.available ? <p className="flex items-center gap-1 text-xs text-emerald-800 dark:text-emerald-200"><Check className="size-3" aria-hidden="true" />可用 · 观测含义一致</p> : candidate.issues.length > 0 && <ul className="space-y-1 text-xs text-muted-foreground">{candidate.issues.map((issue, i) => <li key={i}>{issue.message}</li>)}</ul>}<details className="text-xs"><summary className="min-h-6 cursor-pointer text-muted-foreground underline underline-offset-4">核对观测含义</summary><div className="mt-2 space-y-2"><OutputMeaning output={candidate.output} /><p>{deviceSemanticDifferences(candidate.output, current.semantics).map(path => deviceSemanticLabels[path.slice(1) as keyof typeof deviceSemanticLabels]).join('、') || '七项观测含义完全一致'}</p></div></details></article>)}</div>}
    </div>
    <div className={cn('shrink-0', fill && 'px-3 pb-2')}><ListPagination label="设备" {...list.pagination} previousLabel="上一页设备" nextLabel="下一页设备" /></div>
  </div>;
}
