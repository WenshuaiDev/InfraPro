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
import { Equipment, PageButtons } from './shared';

export function CandidatePicker({ target, current, selected, choose }: { target: DeviceSourceTarget; current: DeviceSourceCurrent; selected?: string; choose: (output: DeviceOutput) => void }) {
  const { client, scope, prefix, available, canSource, canRead } = useDevices();
  const [search, setSearch] = useState('');
  const [compatible, setCompatible] = useState(true);
  const enabled = canRead && canSource && (current.actions.bind || current.actions.switch);
  const list = useCursorList({ queryKey: [...prefix, 'source-candidates', target, search, compatible], enabled,
    load: (cursor, signal) => client.deviceSourceCandidates(scope, target, { q: search.trim(), compatible_only: compatible, cursor, limit: 8 }, signal) });
  if (!enabled) return <EmptyState title={current.target_retired ? '通道或上级资源已退役' : '当前绑定不可维护'} description={current.source.visibility === 'restricted' ? '需取得原设备的查看和使用权限后才能更换或解除。' : '需要设备查看及来源维护权限；退役通道可保留历史或解除已有关系。'} />;
  return <div className="space-y-4"><DirectorySearch label="搜索设备名称或编号" value={search} onChange={setSearch} /><label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm"><Checkbox checked={compatible} onCheckedChange={v => setCompatible(v === true)} />仅看观测含义一致的设备通道</label>
    {list.isPending ? <LoadingState label="正在查找设备通道…" /> : list.error ? <ErrorState message={list.error.message} onRetry={() => { list.go(0); void list.refetch(); }} /> : !list.items.length ? <EmptyState title="没有符合条件的设备通道" description={search ? '试试其他设备名称或编号，或关闭兼容筛选查看具体差异。' : '请先在设备台账登记设备并配置输出的观测含义，再回到这里绑定。'} /> : <div className="max-h-[24rem] space-y-3 overflow-y-auto pr-1">{list.items.map(candidate => <article key={candidate.output.id} className={cn('space-y-3 rounded-lg border p-4', selected === candidate.output.id && 'border-primary bg-primary/5')}><div className="flex items-start justify-between gap-3"><div className="flex min-w-0 gap-2"><Cpu className="mt-1 size-4 shrink-0 text-muted-foreground" aria-hidden="true" /><Equipment output={candidate.output} /></div><Button className="min-h-11 shrink-0" size="sm" variant={selected === candidate.output.id ? 'default' : 'outline'} aria-pressed={selected === candidate.output.id} aria-label={`选择 ${candidate.output.device.name} / ${candidate.output.name}`} disabled={!available || !candidate.available || !candidate.compatible} onClick={() => choose(candidate.output)}>{selected === candidate.output.id ? <><Check className="size-4" aria-hidden="true" />已选</> : '选择'}</Button></div>{candidate.issues.length > 0 && <ul className="space-y-1 text-sm text-muted-foreground">{candidate.issues.map((issue, i) => <li key={i}>{issue.message}</li>)}</ul>}<details className="text-sm"><summary className="min-h-8 cursor-pointer text-muted-foreground underline underline-offset-4">核对观测含义</summary><div className="mt-2 space-y-2"><OutputMeaning output={candidate.output} /><p>{deviceSemanticDifferences(candidate.output, current.semantics).map(path => deviceSemanticLabels[path.slice(1) as keyof typeof deviceSemanticLabels]).join('、') || '七项观测含义完全一致'}</p></div></details></article>)}</div>}
    <PageButtons page={list.page} hasNext={list.hasNext} busy={list.isFetching} previous={() => list.go(list.page - 1)} next={() => void list.next()} label="设备" />
  </div>;
}
