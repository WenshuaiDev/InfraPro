import { useMemo, useRef, useState } from 'react';
import { getRouteApi } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { FolderPlus, Plus } from 'lucide-react';
import type { EngineeringCategory } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Separator } from '@infrapro/ui/components/separator';
import { DirectoryReset, DirectorySearch } from '@infrapro/ui/patterns/directory-table';
import { EmptyState, ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { ListFilter, ListPagination } from '@infrapro/ui/patterns/list-controls';
import { useArrayPagination } from '@infrapro/ui/hooks/use-array-pagination';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { CategoryForm } from '../category-form';
import { CategoryCard } from './category-card';
import { CategoryDirectory } from './category-directory';
import { CreateUnit } from './create-unit';
import { cardSections, filterGroups, groupUnits, readUnitIndex, statusOptions, type CategoryGroup } from './model';
import type { UnitCatalogSearch } from './search';

const route = getRouteApi('/_authenticated/platform/engineering/$kind');
type Panel = { kind: 'unit' } | { kind: 'category'; record?: EngineeringCategory };

export function PlatformUnitCatalog({ editable, selectUnit }: { editable: boolean; selectUnit: (id: string) => void }) {
  const { account, client } = useIdentity();
  const cache = useQueryClient();
  const search = route.useSearch();
  const navigate = route.useNavigate();
  const [panel, setPanel] = useState<Panel | null>(null);
  const trigger = useRef<HTMLElement | null>(null);
  const prefix = ['platform', account?.id, 'engineering'];
  const categories = useQuery({
    queryKey: [...prefix, 'categories'],
    queryFn: ({ signal }) => client.engineeringCategories(undefined, signal),
  });
  const units = useQuery({
    queryKey: [...prefix, false, 'units', 'index'],
    queryFn: ({ signal }) => readUnitIndex(client, signal),
  });
  const groups = useMemo(() => groupUnits(categories.data ?? [], units.data ?? []), [categories.data, units.data]);
  const detail = groups.find(group => group.category.key === search.category);
  function filter(patch: UnitCatalogSearch) {
    void navigate({ search: previous => ({ ...previous, ...patch }), replace: true, resetScroll: false });
  }
  async function browse(category?: string, replace = false) {
    await navigate({
      search: previous => ({ ...previous, category, within: undefined, withinStatus: undefined }),
      replace, resetScroll: false,
    });
    const target = category ? 'unit-category-heading' : 'unit-category-' + search.category;
    requestAnimationFrame(() => (document.getElementById(target) ?? document.querySelector<HTMLElement>('#unit-catalog-tools input'))?.focus());
  }
  function open(next: Panel) {
    trigger.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setPanel(next);
  }

  if (categories.error || units.error) return <ErrorState title="暂时无法读取单位目录"
    message={(categories.error ?? units.error)!.message}
    onRetry={() => { void categories.refetch(); void units.refetch(); }} />;
  if (!categories.data || !units.data) return <><PageHeader title="单位目录" /><LoadingState label="正在读取类别与单位…" /></>;
  if (search.category && !detail) return <EmptyState title="找不到这个单位类别">
    <Button variant="outline" onClick={() => void browse(undefined, true)}>返回类别总览</Button>
  </EmptyState>;

  return <section aria-label="平台单位目录" className="min-w-0">
    {detail ? <CategoryDirectory group={detail} editable={editable} search={search} filter={filter}
      back={() => void browse()} categoryInfo={() => open({ kind: 'category', record: detail.category })}
      createUnit={() => open({ kind: 'unit' })} selectUnit={selectUnit} />
      : <Overview groups={groups} search={search} filter={filter} enter={key => void browse(key)} selectUnit={selectUnit}
          categories={categories.data.length} units={units.data.length} editable={editable} open={open} />}
    <ResponsivePanel open={Boolean(panel)} onOpenChange={value => { if (!value) setPanel(null); }}
      title={panel?.kind === 'unit' ? '新建单位' : panel?.record ? '类别资料' : '新建类别'}
      description={panel?.kind === 'unit' ? '创建单位草稿后，可继续编辑和发布。' : '维护计量单位及数值指标共用的类别。'}
      returnFocus={trigger} fallbackFocusSelector={detail ? '#unit-category-heading' : '#unit-catalog-tools input'}>
      {panel?.kind === 'category' && <CategoryForm key={panel.record?.key ?? 'new'} category={panel.record}
        editable={editable} unitCount={detail?.units.length}
        done={(key, deleted) => { setPanel(null); void browse(deleted ? undefined : key, deleted); }} />}
      {panel?.kind === 'unit' && editable && <CreateUnit categories={categories.data} defaultCategory={detail?.category.key}
        done={id => { setPanel(null); selectUnit(id); void cache.invalidateQueries({ queryKey: prefix }); }} />}
    </ResponsivePanel>
  </section>;
}

type OverviewProps = {
  groups: CategoryGroup[];
  search: UnitCatalogSearch;
  filter: (patch: UnitCatalogSearch) => void;
  enter: (key: string) => void;
  selectUnit: (id: string) => void;
  categories: number;
  units: number;
  editable: boolean;
  open: (panel: Panel) => void;
};

function Overview({ groups, search, filter, enter, selectUnit, categories, units, editable, open }: OverviewProps) {
  const { q = '', topic = '', status = '' } = search;
  const topics = cardSections(groups);
  const sections = cardSections(filterGroups(groups, q, status), Boolean(q.trim())).filter(section => !topic || section.name === topic);
  const visible = sections.flatMap(section => section.groups);
  const page = useArrayPagination(visible, 50, JSON.stringify([q, topic, status]));
  const onPage = new Set(page.items);
  const filtering = Boolean(q || topic || status);
  const reset = () => filter({ q: undefined, topic: undefined, status: undefined });

  return <div className="@container/unit-cards min-w-0">
    <PageHeader title="单位目录" description={categories + ' 个类别 · ' + units + ' 个单位'}
      actions={editable && <div className="flex flex-wrap gap-2">
        <Button variant="outline" onClick={() => open({ kind: 'category' })}><FolderPlus aria-hidden="true" />新建类别</Button>
        <Button onClick={() => open({ kind: 'unit' })}><Plus aria-hidden="true" />新建单位</Button>
      </div>} />
    <div id="unit-catalog-tools" role="search" aria-label="单位目录搜索筛选" className="mb-6 flex flex-wrap items-center gap-2">
      <DirectorySearch label="搜索类别、单位、符号或编号" value={q} onChange={q => filter({ q })} />
      <ListFilter label="浏览分组" value={topic} onChange={topic => filter({ topic })}
        options={[{ value: '', label: '全部分组' }, ...topics.map(({ name }) => ({ value: name, label: name }))]} />
      <ListFilter label="单位状态" value={status} onChange={status => filter({ status })} options={statusOptions} />
      {filtering && <><DirectoryReset onClick={reset} /><span role="status" className="text-xs text-muted-foreground sm:ml-auto">
        {visible.length} 个类别 · {visible.reduce((n, group) => n + group.matches.length, 0)} 个匹配单位
      </span></>}
    </div>
    {sections.length ? <div className="grid gap-7">{sections.filter(section => section.groups.some(group => onPage.has(group))).map(section => <section key={section.name} aria-label={section.name}>
      <div className="mb-3 flex items-center gap-3">
        <h2 className="shrink-0 text-lg font-semibold tracking-tight">{section.name}</h2>
        <span className="shrink-0 text-xs tabular-nums text-muted-foreground">{section.groups.length} 类</span>
        <Separator className="min-w-0 flex-1" />
      </div>
      <div className="grid grid-cols-1 gap-3 @min-[34rem]/unit-cards:grid-cols-2 @min-[54rem]/unit-cards:grid-cols-3">
        {section.groups.filter(group => onPage.has(group)).map(group => <CategoryCard key={group.category.key} group={group} searching={Boolean(q.trim())}
          enter={() => enter(group.category.key)} selectUnit={selectUnit} />)}
      </div>
    </section>)}</div> : <EmptyState title={filtering ? '没有匹配的类别或单位' : '还没有单位类别'}>
      {filtering && <Button variant="outline" size="sm" onClick={reset}>清除筛选</Button>}
    </EmptyState>}
    <div className="mt-6"><ListPagination label="单位类别" {...page.pagination} /></div>
  </div>;
}
