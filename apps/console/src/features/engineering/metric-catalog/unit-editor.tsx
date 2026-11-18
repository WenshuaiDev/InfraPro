import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import type { EngineeringMetric } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { DirectorySearch } from '@infrapro/ui/patterns/directory-table';
import { ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { unitLabel, type UnitChoice } from '../metric-units';

type Props = { metric: EngineeringMetric; known: Map<string, UnitChoice>; change: (rule: EngineeringMetric) => void; remember: (unit: UnitChoice) => void };
export function UnitEditor({ metric, known, change, remember }: Props) {
  const { client, account } = useIdentity();
  const [open, setOpen] = useState(!metric.allowed_units?.length);
  const [search, setSearch] = useState(''), [page, setPage] = useState(0), [size, setSize] = useState(10);
  const ids = metric.allowed_units ?? [];
  const category = ids.length ? metric.category : '';
  const choices = useQuery({
    queryKey: ['platform', account?.id, 'engineering', 'units', 'choices', category, search, page, size],
    queryFn: ({ signal }) => client.engineeringList({ kind: 'units' }, { unit_choices: '1', unit_category: category, q: search, offset: page * size, limit: size }, signal),
    enabled: open,
  });
  const add = (unit: UnitChoice) => {
    if (ids.includes(unit.id) || (category && unit.category !== category)) return;
    remember(unit);
    change({ ...metric, category: unit.category, standard_unit: metric.standard_unit || unit.id, allowed_units: [...ids, unit.id] });
    if (!ids.length) { setSearch(''); setPage(0); }
  };
  const remove = (id: string) => {
    const next = ids.filter(selected => selected !== id);
    change({ ...metric, category: next.length ? metric.category : '', standard_unit: id === metric.standard_unit ? (next[0] ?? '') : metric.standard_unit, allowed_units: next });
    if (!next.length) setPage(0);
  };
  return <section aria-label="关联单位" className="space-y-4 rounded-lg border p-4"><div className="space-y-1"><h3 className="font-semibold">关联单位</h3>
    <p className="text-sm text-muted-foreground">可关联多个同类单位。标准单位是新建通道时的默认值，记录不会自动换算。</p></div>
    {ids.length ? <ul className="divide-y">{ids.map(id => { const unit = known.get(id); return <li key={id} className="flex flex-wrap items-center gap-2 py-2 text-sm">
      <span className="min-w-0 flex-1">{unit ? unitLabel(unit) : '未能读取的固定单位版本'}</span>
      {id === metric.standard_unit ? <strong className="text-xs">标准单位</strong> : <Button variant="ghost" size="sm" type="button" aria-label={'将' + (unit?.name || id) + '设为标准单位'} onClick={() => change({ ...metric, standard_unit: id })}>设为标准</Button>}
      <Button variant="outline" size="sm" type="button" aria-label={'移除' + (unit?.name || id)} disabled={id === metric.standard_unit && ids.length > 1} onClick={() => remove(id)}>移除</Button>
    </li>; })}</ul> : <p className="text-sm">尚未关联单位。第一个选中的单位会成为标准单位。</p>}
    <Button type="button" variant="outline" aria-expanded={open} onClick={() => setOpen(!open)}>{open ? '收起单位目录' : '从单位目录添加'}</Button>
    {open && <div className="space-y-3"><DirectorySearch label="搜索单位名称、符号或编号" value={search} onChange={value => { setSearch(value); setPage(0); }} />
      {choices.isError ? <ErrorState message={choices.error.message} onRetry={() => void choices.refetch()} /> : !choices.data ? <LoadingState label="正在读取可关联单位…" /> :
        <><ul className="divide-y">{choices.data.items.flatMap(item => (item.unit_versions ?? []).map(unit => <li key={unit.id} className="flex items-center justify-between gap-3 py-2 text-sm">
          <span>{unitLabel(unit)}<small className="block text-muted-foreground">{item.code}</small></span><Button size="sm" variant="outline" type="button" disabled={ids.includes(unit.id)} aria-label={`关联${unit.name} V${unit.number}`} onClick={() => add(unit)}>{ids.includes(unit.id) ? '已关联' : '关联'}</Button></li>))}</ul>
          {!choices.data.total && <p className="text-sm text-muted-foreground">没有符合条件的已发布单位。可到单位目录维护后再来关联。</p>}
          <ListPagination label="可关联单位" page={page} pageSize={size} total={choices.data.total} go={setPage} setPageSize={value => { setSize(value); setPage(0); }} /></>}
    </div>}</section>;
}
