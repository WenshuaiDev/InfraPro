import { useState, type ReactNode } from 'react';
import { useQueries, useQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import type { EngineeringSummary } from '@infrapro/api-client';
import { ArrowDownUp, ArrowRight, BookOpen, ChevronRight, CloudSun, Columns3, FlaskConical, FolderOpen, GalleryVerticalEnd, Gauge, Grid2X2, Hash, Landmark, Layers, ListTree, Mountain, Plus, Route, ScanLine, Settings2, Sun, Waypoints, Wind } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { DirectorySearch, DirectoryTable } from '@infrapro/ui/patterns/directory-table';
import { EmptyState, ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { ListFilter, ListPagination } from '@infrapro/ui/patterns/list-controls';
import { useArrayPagination } from '@infrapro/ui/hooks/use-array-pagination';
import { detailQuery, example, exampleLabel, facilities, facilitiesFor, hasStatus, itemStatus, readIndex, recordView, sampleOrder, shapes, standardSymbol, states, type RecordView, type Status, type Variant } from './model';

type Row = { item: EngineeringSummary; view?: RecordView; error: boolean };
type Entries = { rows: Row[]; select: (id: string) => void };
const topicIcons = [ScanLine, Landmark, Layers, Route, Columns3, Mountain, CloudSun, FolderOpen];
const subtitles = { A: '横向核对指标的取值形态、标准单位和可用状态。', B: '先选设施，再查看指标如何取值、使用什么单位。', C: '从一条记录示例出发，了解数字或状态背后的定义。' };
export function MetricIcon({ code, shape }: { code: string; shape?: string }) {
  const Icon = /illuminance|luminance/.test(code) ? Sun : /smoke|dust|fraction|concentration/.test(code) ? Wind : /displacement|settlement/.test(code) ? ArrowDownUp : shape === 'enum' ? ListTree : shape === 'integer' ? Hash : Gauge;
  return <Icon aria-hidden="true" />;
}
export function StatusMark({ status }: { status: string }) {
  return <span className="mc-status" data-tone={status === '可新使用' ? 'usable' : status === '待发布' ? 'draft' : 'neutral'}><i aria-hidden="true" />{status}</span>;
}
function RuleCaption({ row }: { row: Row }) {
  return <>{row.view ? `${shapes[row.view.rule.shape]} · ${standardSymbol(row.view)}` : row.error ? '规则暂不可读' : '正在读取规则…'}</>;
}

function Ledger({ rows, select, tools }: Entries & { tools: ReactNode }) {
  return <DirectoryTable label="指标规则台账" items={rows} rowId={row => row.item.id} empty="没有符合条件的指标" toolbar={<div className="mc-ledger-tools"><h2><GalleryVerticalEnd />规则台账</h2>{tools}</div>} columns={[
    { id: 'name', label: '指标名称 / 编号', className: 'w-2/3 lg:w-[36%]', cell: row => <div><button className="mc-name" type="button" onClick={() => select(row.item.id)}>{row.item.name || row.item.code}</button><code className="mc-code">{row.item.code}</code><span className="mc-small-only"><RuleCaption row={row} /></span></div> },
    { id: 'shape', label: '取值形态', secondary: true, cell: row => row.view ? shapes[row.view.rule.shape] : row.error ? '未能读取' : '…' },
    { id: 'unit', label: '标准单位', secondary: true, cell: row => <span className="mc-unit">{row.view ? standardSymbol(row.view) : row.error ? '未能读取' : '…'}</span> },
    { id: 'version', label: '发布版本', secondary: true, cell: ({ item }) => <>{item.latest ? 'V' + item.latest : '未发布'}{item.has_draft && ' · 有草稿'}</> },
    { id: 'state', label: '可用状态', cell: ({ item }) => <StatusMark status={itemStatus(item)} /> },
  ]} />;
}

function GuideEntries({ rows, select }: Entries) {
  return <ul className="mc-guide-list">{rows.map(row => <li key={row.item.id}>
    <button type="button" aria-label={'查看' + (row.item.name || row.item.code)} onClick={() => select(row.item.id)}>
      <span className="mc-stamp"><MetricIcon code={row.item.code} shape={row.view?.rule.shape} /></span>
      <span className="mc-row-name"><strong>{row.item.name || '未命名指标'}</strong><code className="mc-code">{row.item.code}</code></span>
      <span className="mc-row-rule"><RuleCaption row={row} /></span>
      <span className="mc-row-state"><StatusMark status={itemStatus(row.item)} /><small>{row.item.latest ? 'V' + row.item.latest + (row.item.has_draft ? ' · 有草稿' : '') : '草稿'}</small></span>
      <ChevronRight className="mc-end" aria-hidden="true" />
    </button></li>)}</ul>;
}

function SampleCards({ rows, select, start }: Entries & { start: number }) {
  return <div className="mc-card-grid">{rows.map(({ item, view, error }, index) => <button key={item.id} type="button" className="mc-sample-card" aria-label={'试读' + (item.name || item.code)} onClick={() => select(item.id)}>
    <span className="mc-card-top"><span>样例 {String(start + index + 1).padStart(2, '0')}</span><MetricIcon code={item.code} shape={view?.rule.shape} /></span>
    <span className="mc-card-value">{view ? exampleLabel(view, example(view)) : error ? '不可读' : '…'}{view && view.rule.shape !== 'enum' && <small>{standardSymbol(view)}</small>}</span>
    <span className="mc-card-title">{item.name || item.code}</span><code className="mc-code">{item.code}</code>
    <span className="mc-card-foot"><span>{view ? shapes[view.rule.shape] : '规则'} · {itemStatus(item)}</span><ArrowRight aria-hidden="true" /></span>
  </button>)}</div>;
}

export function Directory({ variant, active, select, editable, manage }: { variant: Variant; active: boolean; select: Entries['select']; editable: boolean; manage: (panel: 'create' | 'categories') => void }) {
  const { account, client } = useIdentity();
  const [query, setQuery] = useState(''), [status, setStatus] = useState<Status>('all'), [topic, setTopic] = useState('');
  const index = useQuery({ queryKey: ['platform', account?.id, 'engineering', 'metrics', 'prototype-index'], queryFn: ({ signal }) => readIndex(client, signal), enabled: active, staleTime: 120_000 });
  const all = index.data ?? [];
  const term = query.trim().toLocaleLowerCase();
  const matchingNames = all.filter(item => !term || `${item.name} ${item.code}`.toLocaleLowerCase().includes(term));
  const byStatus = matchingNames.filter(item => hasStatus(item, status));
  const inTopic = (item: EngineeringSummary) => !topic || facilitiesFor(item.code).includes(topic);
  const filtered = byStatus.filter(inTopic);
  const sorted = variant === 'C' ? sampleOrder(filtered) : filtered;
  const { items, pagination } = useArrayPagination(sorted, 6, JSON.stringify([query, status, topic]));
  const details = useQueries({ queries: active ? items.map(item => detailQuery(client, account?.id, item.id)) : [] });
  if (!active) return null;
  const rows = items.map((item, index) => ({ item, view: recordView(details[index]?.error ? undefined : details[index]?.data), error: !!details[index]?.error }));
  const reset = () => { setQuery(''); setStatus('all'); setTopic(''); };
  const count = (name: string) => byStatus.filter(item => facilitiesFor(item.code).includes(name)).length;
  const statusOptions = (Object.keys(states) as Status[]).map(key => ({ value: key, label: states[key] }));
  const empty = <EmptyState title={all.length ? '没有符合条件的指标' : '目录中暂时没有指标'} description={all.length ? '换个名称、编号或筛选条件再试。' : undefined}>{!!all.length && <Button variant="outline" onClick={reset}>清除筛选</Button>}</EmptyState>;
  const tools = <div className="mc-toolbar" role="search" aria-label="查找指标">
    <DirectorySearch label="搜索指标名称或编号" value={query} onChange={setQuery} />
    {variant !== 'A' && <ListFilter label="可用状态" value={status === 'all' ? '' : status} onChange={value => setStatus((value || 'all') as Status)} options={statusOptions.map(option => ({ ...option, value: option.value === 'all' ? '' : option.value }))} />}
    {variant !== 'B' && <ListFilter label="设施范围" value={topic} onChange={setTopic} options={[{ value: '', label: '全部设施' }, ...facilities.map(name => ({ value: name, label: name }))]} />}
    {(query || topic || status !== 'all') && <Button variant="ghost" size="sm" onClick={reset}>清除筛选</Button>}
  </div>;
  const pager = <ListPagination label="指标" {...pagination} pageSizes={[6, 10, 20]} unit="项" />;
  return <div>
    <header className="mc-page-head"><div><p className="mc-kicker"><BookOpen />工程配置 <span>/</span> 公共目录</p><div className="mc-title-line"><h1>指标目录</h1>{index.data && <span className="mc-counter">{all.length} 项</span>}</div><p className="mc-lede">{subtitles[variant]}</p></div>
      <div className="mc-head-actions"><Button variant="outline" onClick={() => manage('categories')}><Settings2 />单位类别</Button>{editable && <Button onClick={() => manage('create')}><Plus />新建指标</Button>}</div>
    </header>
    {index.isError ? <ErrorState message={index.error.message} onRetry={() => void index.refetch()} /> : !index.data ? <LoadingState label="正在读取指标目录…" /> : variant === 'A' ? <>
      <div className="mc-state-strip" aria-label="按状态筛选">{statusOptions.map(({ value, label }) => <button key={value} type="button" aria-pressed={status === value} onClick={() => setStatus(value)}>{label}<b>{matchingNames.filter(item => inTopic(item) && hasStatus(item, value)).length}</b></button>)}</div>
      <section className="mc-ledger"><Ledger rows={rows} select={select} tools={tools} /></section>
      <div className="mc-pager">{pager}</div>
    </> : variant === 'B' ? <div className="mc-guide">
      <aside className="mc-rail" aria-label="按设施浏览"><div className="mc-rail-label"><Waypoints />设施索引</div>
        <label className="mc-mobile-picker">浏览设施<select value={topic} onChange={event => setTopic(event.target.value)}><option value="">全部设施 · {byStatus.length}</option>{facilities.map(name => <option key={name} value={name}>{name} · {count(name)}</option>)}</select></label>
        <div className="mc-rail-items"><button type="button" aria-pressed={!topic} onClick={() => setTopic('')}><Grid2X2 />全部设施<b>{byStatus.length}</b></button>
        {facilities.map((name, index) => { const Icon = topicIcons[index]!; return <button type="button" key={name} aria-pressed={topic === name} onClick={() => setTopic(name)}><Icon />{name}<b>{count(name)}</b></button>; })}</div>
        <p className="mc-rail-note">同一指标可出现在多个设施下。此索引用于查找，不限制实际选用。</p></aside>
      <section className="mc-guide-main"><div className="mc-section-line"><div><p className="mc-overline">指标索引</p><h2>{topic ? topic + '指标' : '全部指标'}<span className="mc-count-inline">{filtered.length} 项</span></h2></div></div>
        {tools}{rows.length ? <GuideEntries rows={rows} select={select} /> : empty}<div className="mc-pager">{pager}</div></section>
    </div> : <section className="mc-handbook"><div className="mc-section-line"><div><p className="mc-overline"><FlaskConical />读值练习</p><h2>一条记录，怎样读？</h2><p className="mc-muted">示例按定义生成，不是现场实测值。点开一张卡即可试填。</p></div>{tools}</div>
      {rows.length ? <SampleCards rows={rows} select={select} start={pagination.page * pagination.pageSize} /> : empty}<div className="mc-pager">{pager}</div></section>}
  </div>;
}
