import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { MonitoringQuery } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { useMonitoring } from './context';
import { MonitoringError } from './common';
import { AnalyticState } from './data-tables';
import { observationTime } from './detail-format';
import { count, rangeLabel } from './format';

// The owning monitoring endpoint rechecks the original personal snapshot and
// data rights. Common task metadata never contains this query or its values.
export function FixedQueryResult({ action, input }: { action: 'overview' | 'latest' | 'statistics' | 'trends'; input: MonitoringQuery }) {
 const m=useMonitoring(),[cursor,setCursor]=useState<string>(),[part,setPart]=useState(0);
 const result=useQuery({queryKey:[...m.prefix,'fixed-task-query',action,input,cursor],queryFn:({signal})=>m.client.monitoringQuery(m.scope,action,{...input,...(cursor?{cursor}:{})},signal),retry:false,refetchOnWindowFocus:'always'});
 const value=result.error?undefined:result.data,rows=value?.groups??value?.channels??[];
 const points=value?.trends?.flatMap(series=>series.buckets.flatMap(bucket=>bucket.points.map(point=>({channel:series.channel_id,point}))))??[];
 return <section className="space-y-3 rounded-lg border p-4"><h3 className="font-semibold">固定查询结果</h3><p className="text-sm text-muted-foreground">按原快照和筛选核验。{input.view_range??input.range?rangeLabel((input.view_range??input.range)!):'全部获准历史'}</p><MonitoringError error={result.error} retry={()=>void result.refetch()}/><AnalyticState result={value} loading={result.isPending}/>{value?.state==='ready'&&<><p className="text-xs">固定水位 {value.snapshot.watermark} · 有效期至 {new Date(value.snapshot.expires_at).toLocaleString()}</p>{value.overview&&<dl className="grid grid-cols-2 gap-3 text-sm"><dt>统计样本</dt><dd>{count(value.overview.sample_count)}</dd><dt>在用观测</dt><dd>{count(value.overview.active_count)}</dd><dt>首次观测</dt><dd className="break-all">{value.overview.earliest??'无'}</dd><dt>末次观测</dt><dd className="break-all">{value.overview.latest??'无'}</dd></dl>}{rows.map((row,index)=><div key={`${row.channel_id}:${index}`} className="rounded-md bg-muted/40 p-3 text-sm"><p>通道 {index+1} · 样本 {count(row.n)} · 均值 {row.mean??'—'} · 标准差 {row.population_stddev??'—'}</p><p>最小 {row.minimum?.value??'—'} · 最大 {row.maximum?.value??'—'} · 最新 {row.last?.value??'—'}</p>{'calendar_start' in row&&<p>{row.calendar_start} 至 {row.calendar_end}</p>}<details className="text-xs"><summary>通道编号与枚举状态</summary><p className="break-all">{row.channel_id}</p>{row.states.map(state=><p key={state.value}>{state.value} · {state.count}</p>)}</details></div>)}{points.length>0&&<div className="space-y-2"><p className="text-sm">趋势采样点（固定查询采样，不代表全部明细）</p>{points.slice(part*50,(part+1)*50).map(({channel,point},index)=><p key={`${channel}:${index}`} className="break-all font-mono text-xs">{observationTime(point)} · {point.value??'—'}</p>)}<div className="flex flex-wrap gap-2">{part>0&&<Button variant="outline" onClick={()=>setPart(part-1)}>上一组采样</Button>}{(part+1)*50<points.length&&<Button variant="outline" onClick={()=>setPart(part+1)}>下一组采样</Button>}</div></div>}{value.next_cursor&&<Button variant="outline" onClick={()=>{setCursor(value.next_cursor);setPart(0);}}>下一页查询结果</Button>}</>}</section>;
}
