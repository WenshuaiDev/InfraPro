import { useQuery } from '@tanstack/react-query';
import { getRouteApi } from '@tanstack/react-router';
import { useIdentity } from '@infrapro/api-client/react';
import { BookOpen, ChevronRight, CloudSun, Columns3, FolderOpen, Gauge, Grid2X2, Landmark, Layers, ListTree, Mountain, Plus, Route as Road, ScanLine, Waypoints } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { DirectoryReset, DirectorySearch } from '@infrapro/ui/patterns/directory-table';
import { ErrorState, EmptyState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { ListFilter, ListPagination } from '@infrapro/ui/patterns/list-controls';
import { facilityOrder, metricScope, metricShapes } from './facts';
import './metric-catalog.css';

const route = getRouteApi('/_authenticated/platform/engineering/$kind');
const icons = [ScanLine, Landmark, Layers, Road, Columns3, Mountain, CloudSun];
const label = (topic: string) => topic === '__unassigned' ? '未归类' : topic || '全部设施';
export function MetricDirectory({ select, create }: { select: (id: string) => void; create?: () => void }) {
  const { client, account, authorizationEpoch } = useIdentity();
  const { q = '', status = '', topic = '', metricPage = 1, metricSize = 20 } = route.useSearch();
  const navigate = route.useNavigate();
  const filter = (patch: { q?: string; status?: string; topic?: string; metricPage?: number; metricSize?: number }) =>
    void navigate({ search: previous => ({ ...previous, metricPage: undefined, ...patch }), replace: true, resetScroll: false });
  const query = useQuery({ queryKey: ['platform', account?.id, 'engineering', 'metric-directory', authorizationEpoch, q, status, topic, metricPage, metricSize],
    queryFn: ({ signal }) => client.engineeringList(metricScope, { q, status, facility_topic: topic, offset: (metricPage - 1) * metricSize, limit: metricSize }, signal) });
  const counts = query.data?.facility_counts ?? {};
  const topics = [...new Set([...facilityOrder, ...Object.keys(counts).filter(key => !['__all','__unassigned'].includes(key)), ...(topic ? [topic] : []), '__unassigned'])].filter(key => key !== '__all');
  const reset = () => filter({ q: undefined, status: undefined, topic: undefined });
  return <section className="met-page" aria-label="指标目录"><header className="met-heading"><div><p className="met-eyebrow"><BookOpen />工程配置 / 公共目录</p>
    <h1>指标目录</h1><p>按设施查找指标，查看取值规则与允许使用的单位。</p></div>
    {create && <Button id="create-definition" onClick={create}><Plus />新建指标</Button>}</header>
    <div className="met-guide"><aside aria-label="按设施浏览"><h2><Waypoints />设施索引</h2>
      <label className="met-mobile">浏览设施<select value={topic} onChange={event => filter({ topic: event.target.value || undefined })}>
        {['', ...topics].map(key => <option key={key} value={key}>{label(key)} · {counts[key || '__all'] ?? 0}</option>)}</select></label>
      <nav aria-label="设施索引">{['', ...topics].map(key => { const Icon = !key ? Grid2X2 : icons[facilityOrder.indexOf(key)] ?? FolderOpen;
        return <button key={key} type="button" aria-pressed={key === topic} onClick={() => filter({ topic: key || undefined })}><Icon />{label(key)}<small>{counts[key || '__all'] ?? '—'}</small></button>; })}</nav>
      <p>同一指标可用于多类设施。此索引用于查找，不限制实际选用。</p></aside>
      <section className="met-results"><p className="met-eyebrow">指标索引</p><h2>{topic ? label(topic) + '指标' : '全部指标'}{query.data && <small>{query.data.total} 项</small>}</h2>
        <div className="met-tools"><DirectorySearch label="搜索指标名称或编号" value={q} onChange={value => filter({ q: value || undefined })} />
          <ListFilter label="可用状态" value={status} onChange={value => filter({ status: value || undefined })} options={[{ value: '', label: '全部状态' }, { value: 'usable', label: '可新使用' }, { value: 'draft', label: '有草稿' }, { value: 'unavailable', label: '不可新使用' }]} />
          {(topic || q || status) && <DirectoryReset onClick={reset} />}</div>
        {query.isError ? <ErrorState message={query.error.message} onRetry={() => void query.refetch()} /> : !query.data ? <LoadingState label="正在读取指标…" /> : query.data.items.length ? <ul className="met-list">
          {query.data.items.map(item => { const rule = item.metric_index, Icon = rule?.shape === 'enum' ? ListTree : Gauge;
            return <li key={item.id}><button aria-label={item.name || item.code} type="button" onClick={() => select(item.id)}><span className="met-icon"><Icon /></span>
              <span className="met-name"><strong>{item.name || '未命名指标'}</strong><code>{item.code}</code></span>
              <span className="met-rule">{metricShapes[rule?.shape ?? ''] || '待配置'} · {rule?.shape === 'enum' ? '无单位' : rule?.standard_symbol || '未设标准单位'}</span>
              <span className="met-status" data-usable={item.usable}>{rule?.latest_usable ? '可新使用' : item.usable ? '旧版可选' : item.latest ? '不可新使用' : '待发布'}<small>{item.latest ? 'V' + item.latest + (item.has_draft ? ' · 有草稿' : '') : '草稿'}</small></span><ChevronRight className="met-chevron" />
            </button></li>; })}</ul> : <EmptyState title="没有符合条件的指标">{(topic || q || status) && <Button variant="outline" onClick={reset}>清除筛选</Button>}</EmptyState>}
        {query.data && <ListPagination label="指标目录" unit="项" total={query.data.total} page={metricPage - 1} pageSize={metricSize} fetching={query.isFetching}
          go={page => filter({ metricPage: page ? page + 1 : undefined })} setPageSize={size => filter({ metricSize: size })} />}
      </section></div></section>;
}
