import { ArrowLeft, Plus } from 'lucide-react';
import { PageHeader } from '@infrapro/layouts';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { DirectoryReset, DirectorySearch, DirectoryTable } from '@infrapro/ui/patterns/directory-table';
import { ListFilter, ListPagination } from '@infrapro/ui/patterns/list-controls';
import { useArrayPagination } from '@infrapro/ui/hooks/use-array-pagination';
import { filterUnits, statusOptions, unassigned, unitStatus, type CategoryGroup } from './model';
import type { UnitCatalogSearch } from './search';

type Props = {
  group: CategoryGroup;
  editable: boolean;
  search: UnitCatalogSearch;
  filter: (patch: UnitCatalogSearch) => void;
  back: () => void;
  categoryInfo: () => void;
  createUnit: () => void;
  selectUnit: (id: string) => void;
};

export function CategoryDirectory({ group: { category, units }, editable, search, filter, back, categoryInfo, createUnit, selectUnit }: Props) {
  const term = search.within?.trim().toLowerCase() ?? '';
  const filtered = filterUnits(units, term, search.withinStatus ?? '');
  const page = useArrayPagination(filtered, 20, JSON.stringify([category.key, term, search.withinStatus]));
  const filtering = Boolean(search.within || search.withinStatus);
  return (
    <div>
      <Button size="sm" variant="ghost" className="mb-3 -ml-2" onClick={back}>
        <ArrowLeft aria-hidden="true" />返回类别总览
      </Button>
      <div id="unit-category-heading" tabIndex={-1} className="scroll-mt-20 outline-none">
        <PageHeader title={category.name}
          description={[category.key === unassigned ? '' : category.key, units.length + ' 个单位'].filter(Boolean).join(' · ')}
          actions={<div className="flex flex-wrap gap-2">
            {category.key !== unassigned && <Button variant="outline" onClick={categoryInfo}>类别资料</Button>}
            {editable && <Button onClick={createUnit}><Plus aria-hidden="true" />新建单位</Button>}
          </div>} />
      </div>
      <DirectoryTable label={category.name + '的单位'} rowId={unit => unit.id} items={page.items}
        pagination={<ListPagination label={category.name + '的单位'} {...page.pagination} />}
        empty={filtering ? '没有符合筛选条件的单位' : '这个类别还没有单位'}
        toolbar={<>
          <DirectorySearch label="搜索本类单位名称、符号或编号" value={search.within ?? ''} onChange={within => filter({ within })} />
          <ListFilter label="单位状态" options={statusOptions} value={search.withinStatus ?? ''} onChange={withinStatus => filter({ withinStatus })} />
          {filtering && <DirectoryReset onClick={() => filter({ within: undefined, withinStatus: undefined })} />}
        </>}
        columns={[
          { id: 'name', label: '单位名称', className: 'w-[45%] sm:w-[40%]', cell: unit => <div className="min-w-0 space-y-0.5">
            <Button variant="link" className="h-auto max-w-full justify-start whitespace-normal p-0 text-left text-foreground" onClick={() => selectUnit(unit.id)}>{unit.name}</Button>
            <p className="break-all font-mono text-xs text-muted-foreground">{unit.code}</p>
            <p className="text-xs text-muted-foreground sm:hidden">{unit.latest ? 'V' + unit.latest : '未发布'}{unit.has_draft && ' · 有草稿'}</p>
          </div> },
          { id: 'symbol', label: '符号', cell: unit => <span className="break-all font-mono">{unit.symbol || '—'}</span> },
          { id: 'version', label: '发布版本', className: 'hidden sm:table-cell', cell: unit => <div className="flex flex-wrap items-center gap-1.5">
            {unit.latest ? 'V' + unit.latest : '未发布'}{unit.has_draft && <Badge variant="outline">草稿</Badge>}</div> },
          { id: 'status', label: '使用状态', cell: unit => <Badge variant={unit.usable ? 'secondary' : 'outline'} className="font-normal">{unitStatus(unit)}</Badge> },
        ]}
      />
    </div>
  );
}
