import { useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { FileText, FolderOpen, ListFilter, Plus, Search, Settings2 } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { LoadingState } from '@infrapro/ui/patterns/feedback';
import { MonitoringPageHeader } from '../monitoring/page-header';
import { NewAnalysis } from './new-analysis';
import { AnalysisEditor } from './analysis-editor';
import { ArtifactDetail } from './artifact-detail';
import { reportTime } from './analysis-format';
import { CommandError, SelectField, useDerived } from './common';
import type { DerivedSearch } from './search';

export function AnalysisPage({id, view, version, export_id, ...prefill}: DerivedSearch) {
  const m = useDerived(), navigate = useNavigate(), cache = useQueryClient();
  const [query, setQuery] = useState(''), [state, setState] = useState<'' | 'active' | 'archived'>('');
  const [notice, setNotice] = useState('');
  const canResults = m.allows('analysis.results.read'), canConfigs = m.allows('analysis.read');
  const canCreate = canConfigs && m.editable && m.allows('analysis.manage');
  const configsView = canConfigs && (!canResults || view === 'configs' || (!view && !!id));
  const go = (mode: 'configs' | 'artifacts', next?: string, fixed?: string) =>
    navigate({to: m.analysisPath, search: {...prefill, view: mode, id: next, version: fixed}, resetScroll: false});
  const filter = {q: query, state: state || undefined};
  const configs = usePagedQuery({queryKey: [...m.prefix, 'analysis-configs', filter], enabled: configsView,
    queryFn: ({cursor, limit, signal}) => m.client.analysisConfigs(m.scope, {...filter, cursor, limit}, signal)});
  const artifacts = usePagedQuery({queryKey: [...m.prefix, 'analysis-artifacts', filter], enabled: canResults && !configsView,
    queryFn: ({cursor, limit, signal}) => m.client.analysisArtifacts(m.scope, {...filter, cursor, limit}, signal)});
  const list = configsView ? configs : artifacts;
  const selected = id ?? (configsView ? configs.data?.items[0]?.id : artifacts.data?.items[0]?.artifact.id);
  const config = useQuery({queryKey: [...m.prefix, 'analysis-config', selected], enabled: configsView && !!selected && selected !== 'new',
    queryFn: ({signal}) => m.client.analysisConfig(m.scope, selected!, signal)});
  const entries = configsView ? configs.data?.items.map(v => ({id: v.id, name: v.name,
    state: v.status === 'archived' ? '已归档' : v.latest_version === '0' ? '草稿' : '方案 V' + v.latest_version,
    meta: v.inputs_restricted ? '输入读取受限' : v.code})) : artifacts.data?.items.map(({artifact, version: v}) => ({
    id: artifact.id, name: artifact.name, state: '成果 V' + v.version_no + (artifact.status === 'archived' ? ' · 已归档' : ''),
    meta: reportTime(v.output_range.from_ns, 'date') + ' 至 ' + reportTime(v.output_range.to_ns, 'date')}));
  const switchView = () => {
    setQuery(''); setState(''); setNotice('');
    void go(configsView ? 'artifacts' : 'configs');
  };
  const afterDelete = async (name: string) => {
    setNotice('已删除“' + name + '”及其全部版本。');
    // Refresh dependent version, comparison and export reads as well as the ledger.
    await cache.invalidateQueries({queryKey: m.prefix});
    await go('artifacts');
  };
  return <div className="@container/analysis mx-auto min-w-0 max-w-[90rem] space-y-5 pb-6">
    <MonitoringPageHeader workspaceTasks title="历史监测分析" description="复核历史监测成果，或沿用已保存的方案计算另一段时间。" actions={<>
      {canConfigs && canResults && <Button variant="outline" onClick={switchView}>{configsView ? <FolderOpen/> : <Settings2/>}{configsView ? '返回成果' : '分析方案'}</Button>}
      {canCreate && <Button onClick={() => { setNotice(''); void go('configs', 'new'); }}><Plus/>新建分析</Button>}
    </>}/>
    {notice && <p role="status" className="rounded-lg border bg-muted/40 px-4 py-2 text-sm">{notice}</p>}
    {!(canConfigs || canResults) ? <p role="status">当前身份没有历史分析读取权限。</p> :
    <div className="grid min-w-0 items-start gap-5 @[54rem]/analysis:grid-cols-[minmax(14rem,0.48fr)_minmax(0,1.52fr)]">
      <aside aria-label={configsView ? '分析方案目录' : '成果目录'} className="min-w-0 space-y-3 border-t-2 border-teal-700 pt-3 dark:border-teal-400">
        <h2 className="flex items-center gap-2 font-semibold"><FolderOpen size={18}/>{configsView ? '分析方案目录' : '成果目录'}</h2>
        <div className="flex items-start gap-1"><label className="relative min-w-0 flex-1"><Search aria-hidden className="absolute top-3 left-3 size-4 text-muted-foreground"/>
          <Input type="search" aria-label={configsView ? '搜索分析配置' : '搜索分析成果'} placeholder={configsView ? '搜索方案名称或编码' : '搜索成果名称'} className="min-h-10 pl-9" value={query} onChange={e => setQuery(e.target.value)}/></label>
          <details className="group relative"><summary aria-label="筛选归档状态" className="grid size-10 cursor-pointer list-none place-items-center rounded-md border hover:bg-muted"><ListFilter size={17}/></summary>
            <div className="absolute right-0 z-10 w-44 rounded-lg border bg-popover p-3 shadow-md"><SelectField label="归档状态" value={state} onChange={setState} options={[{value:'', label:'全部状态'}, {value:'active', label:'未归档'}, {value:'archived', label:'已归档'}]}/></div></details></div>
        {state && <button className="text-xs underline" onClick={() => setState('')}>{state === 'active' ? '未归档' : '已归档'} · 清除筛选</button>}
        <CommandError error={list.error}/>
        {list.isPending && !list.error && <LoadingState/>}
        <div className="max-h-64 overflow-y-auto @[54rem]/analysis:max-h-[65vh]">{entries?.map(item =>
          <button type="button" key={item.id} aria-pressed={selected === item.id} onClick={() => { setNotice(''); void go(configsView ? 'configs' : 'artifacts',item.id); }}
            className="grid w-full gap-1 border-b border-l-[3px] border-l-transparent px-3 py-3 text-left hover:bg-muted/60 aria-pressed:border-l-teal-700 aria-pressed:bg-teal-50 dark:aria-pressed:border-l-teal-400 dark:aria-pressed:bg-teal-950/55">
            <span className="flex items-center gap-1.5 text-xs text-teal-800 dark:text-teal-300"><FileText size={14}/>{item.state}</span>
            <strong className="break-words text-sm">{item.name}</strong><span className="text-xs text-muted-foreground">{item.meta}</span>
          </button>)}</div>
        {entries?.length === 0 && !list.error && <p className="py-2 text-sm text-muted-foreground">{query || state ? '没有符合筛选条件的记录。' : configsView ? '还没有分析方案。' : '还没有已生成的成果。'}</p>}
        {list.data && (list.pagination.total === undefined || list.pagination.total > list.pagination.pageSize || list.pagination.page > 0) && <ListPagination label={configsView ? '方案' : '成果'} {...list.pagination}/>}
      </aside>
      <div className="min-w-0">{!selected ? !list.isPending && <div className="rounded-xl border border-dashed p-8 text-center"><h2 className="font-semibold">{query || state ? '没有匹配记录' : configsView ? '建立第一个分析方案' : '暂无历史成果'}</h2>
        <p className="mt-2 text-sm text-muted-foreground">{query || state ? '更换名称或归档状态后再查找。' : '从分析方案选择方法和测点，指定时段后运行即可生成成果。'}</p></div> :
        !configsView ? <ArtifactDetail key={selected} id={selected} version={version} exportID={export_id} onVersion={v => void go('artifacts',selected,v)} onDeleted={afterDelete}/> :
        selected === 'new' ? canCreate ? <NewAnalysis key="new" prefill={prefill} onSelect={v => go('configs',v)}/> : <p role="status">当前身份无法新建分析。</p> :
        <><CommandError error={config.error}/>{config.isPending ? <LoadingState/> : config.data && !config.error && <AnalysisEditor key={config.data.id} config={config.data} prefill={prefill} onSelect={v => go('configs',v)}/>}</>}
      </div>
    </div>}
  </div>;
}
