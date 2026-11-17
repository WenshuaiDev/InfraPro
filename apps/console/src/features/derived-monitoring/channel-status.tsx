import { useQuery } from '@tanstack/react-query';
import { ApiError, type MonitoringAnalyticResult, type DerivedChannel } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { querySlot } from '../monitoring/query-hooks';
import { calculationStatusNames } from '../monitoring/detail-format';
import { rangeLabel } from '../monitoring/format';
import { useDerived } from './common';
import { QuantityLabel } from './rule-summary';

export function ChannelStatus({channel,compact=false}:{channel:DerivedChannel;compact?:boolean}) {
 const m=useDerived();
 const result=useQuery({queryKey:[...m.prefix,'derived-latest',channel.resource.id,channel.config_revision,channel.control_revision],queryFn:({signal})=>querySlot(signal,async()=>{
  const selected=await m.client.monitoringSelect(m.scope,{channel_ids:[channel.resource.id]},signal);
  return m.client.monitoringQuery(m.scope,'latest',{selection_id:selected.id,all_history:true,qualities:['good','unmarked','suspect','bad'],timezone:'UTC',status:'active',limit:1},signal);
 }),enabled:m.canRead,refetchInterval:q=>(q.state.data as MonitoringAnalyticResult|undefined)?.state==='pending'?2000:channel.control_state==='enabled'?15000:false,retry:false});
 if(!m.canRead)return <p className="text-xs text-muted-foreground">未授予监测数据读取权限。</p>;
 if(result.error)return <div className="space-y-1 text-xs"><p role="status">{result.error instanceof ApiError&&[403,404].includes(result.error.status??0)?'当前未获准读取完整输入依据，结果已隐藏。':result.error.message}</p><Button variant="ghost" size="sm" onClick={()=>void result.refetch()}>重新读取状态</Button></div>;
 if(!result.data||result.data.state==='pending')return <p role="status" className="text-xs text-muted-foreground">{result.data?'正在后台核验完整结果…':'正在读取当前结果…'}</p>;
 if(result.data.state!=='ready')return <p role="alert" className="text-xs">结果查询{result.data.state==='failed'?'失败':'需要处理'}，请查看计算任务。</p>;
 const c=result.data.channels?.[0];if(!c)return <p className="text-xs">暂无获准当前结果。</p>;
 const latest=c.latest_output,valid=c.last_valid_output??c.last;
 const spans=result.data.coverage??[];
 if(compact)return <div className="max-w-56 space-y-1 text-xs"><p className="break-all font-mono text-base font-medium">{latest?.value??'—'}</p><p className="text-muted-foreground"><QuantityLabel quantity={channel.resource.quantity_spec??undefined}/></p><p className="text-muted-foreground">{latest?calculationStatusNames[latest.calculation_status]:'尚无输出'}</p>{latest&&<p title={latest.output_at} className="text-muted-foreground">{new Date(latest.output_at).toLocaleString('zh-CN',{hour12:false})}</p>}{!!c.pending_ranges?.length&&<p className="text-amber-700 dark:text-amber-300">部分范围等待更新</p>}</div>;

 return <div className={`space-y-2 text-xs rounded-lg bg-muted/30 p-4`}>
  <p className="text-sm font-medium">输出单位：<QuantityLabel quantity={channel.resource.quantity_spec??undefined}/></p><p>最新输出：<span className="font-mono">{latest?.output_at??'尚无输出'}</span>{latest&&<> · {calculationStatusNames[latest.calculation_status]} · <span className="break-all font-mono">{latest.value??'无结果'}</span></>}</p>
  <p>上一有效值：<span className="break-all font-mono">{valid?.value??'—'}</span>{valid&&<span className="block font-mono">{'output_at' in valid?valid.output_at:valid.observed_at}</span>}</p>
  <p>已计算至：<span className="font-mono">{c.computed_through??'尚未计算'}</span></p>
  <p>输入已到达：<span className="font-mono">{c.input_through??'范围内尚无输入'}</span></p>
  {!!c.pending_ranges?.length&&<p>等待更新：{c.pending_ranges.map(rangeLabel).join('；')}</p>}
  {!compact&&<details><summary className="min-h-11 cursor-pointer py-3">已计算范围与尚未计算区间</summary>{spans.map((v,i)=><p key={i}>{v.state==='current'?'当前完整':v.state==='pending'?'待更新':v.state==='recomputing'?'正在重算':'尚未计算'} · {rangeLabel(v.range)}</p>)}</details>}
  <p className="text-muted-foreground">截至本次固定查询快照；范围终点不含。</p>
 </div>;
}
