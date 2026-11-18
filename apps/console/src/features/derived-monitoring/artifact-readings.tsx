import { useState } from 'react';
import { useQueries, useQuery } from '@tanstack/react-query';
import { ChevronDown, FileCheck2 } from 'lucide-react';
import type { AnalysisArtifactDetail, AnalysisArtifactRows } from '@infrapro/api-client';
import { ListPagination, type ListPage } from '@infrapro/ui/patterns/list-controls';
import { LoadingState } from '@infrapro/ui/patterns/feedback';
import { calculationStatusNames, reasonNames } from '../monitoring/detail-format';
import { MAX_NS, qualityNames } from '../monitoring/format';
import { QuantityLabel } from './rule-summary';
import { reportTime, inputStateLabel, sampleOffset } from './analysis-format';
import { CommandError, useDerived } from './common';

const statusLabels: Record<string, string> = calculationStatusNames;
type Row = AnalysisArtifactRows['items'][number];
type Inputs = Record<string, {label: string; unit: string}>;
type Output = AnalysisArtifactDetail['outputs'][number];

// eslint-disable-next-line react-refresh/only-export-components
export function useArtifactInputs(detail: AnalysisArtifactDetail): Inputs {
  const m = useDerived();
  const bindings = Object.entries(detail.config.body.input_bindings);
  const channels = useQueries({queries: bindings.map(([,v]) => ({
    queryKey: [...m.prefix, 'bound-channel', v.channel_id],
    queryFn: ({signal}: {signal: AbortSignal}) => m.client.monitoringDirectory(m.scope, {channel_id:v.channel_id}, signal),
    enabled: m.canRead && m.canNavigate,
  }))});
  return Object.fromEntries(bindings.map(([key,v], i) => {
    const c = channels[i]?.data?.items[0];
    return [v.channel_id, {label:c ? c.owner_name + ' · ' + c.name : detail.config.body.rule.inputs.find(s => s.key === key)?.name || key, unit:c?.unit ?? ''}];
  }));
}

function ReadingMeaning({detail, row, output, inputs}: {detail: AnalysisArtifactDetail; row: Row; output: Output; inputs: Inputs}) {
  const rule = detail.config.body.rule, node = rule.nodes.find(n => n.id === output.node);
  const labels = (node?.inputs ?? []).map(ref => {
    const key = ref.replace(/^input:/, ''), channel = detail.config.body.input_bindings[key]?.channel_id;
    return channel ? inputs[channel]?.label : undefined;
  });
  if (row.value == null) return <p>{row.reason_flags.map(k => reasonNames[k] || k).join('、') || statusLabels[row.calculation_status] || row.calculation_status}</p>;
  if (node?.op === 'subtract' && labels[0] && labels[1]) {
    const zero = /^-?0(?:\.0+)?$/.test(row.value);
    return <><p>{zero ? '两处读数相同。' : (row.value.startsWith('-') ? '第一处读数比第二处小 ' : '第一处读数比第二处大 ') + row.value.replace(/^-/, '').replace(/(\.\d*?[1-9])0+$|\.0+$/, '$1') + ' '} {!zero && <QuantityLabel quantity={output.quantity_spec}/>}</p><p className="mt-1 text-xs font-normal text-muted-foreground">{labels[0]} − {labels[1]}</p></>;
  }
  if (node?.op === 'window_mean') return <p>{row.count} 个合格样本参与平均，排除 {row.excluded_count} 个。</p>;
  if (node?.op === 'difference') return <p>{node.params.difference_mode === 'lag' ? '本次读数减去按固定滞后时长匹配的读数。' : '本次读数减去前一次有效读数。'}</p>;
  return <p>{output.name || output.key} · {qualityNames[row.quality] || row.quality}</p>;
}

export function ArtifactHeadline({detail, inputs}: {detail: AnalysisArtifactDetail; inputs: Inputs}) {
  return <div className="space-y-3">
    <p className="flex items-center gap-2 text-sm font-medium text-teal-800 dark:text-teal-300"><FileCheck2 size={17}/>固定成果 V{detail.version.version_no}{detail.version.partial && ' · 获准片段'}{detail.artifact.status === 'archived' && ' · 已归档'}</p>
    <h2 className="text-xl font-semibold tracking-tight break-words md:text-2xl">{detail.artifact.name}</h2>
    <p className="break-words text-sm text-muted-foreground">{reportTime(detail.version.output_range.from_ns)} 至 {reportTime(detail.version.output_range.to_ns)}（上海时间） · 方案 V{detail.config.version_no}</p>
    {detail.latest_outputs?.map(row => {
      const output = detail.outputs.find(o => o.key === row.output_key);
      if (!output) return null;
      return <div key={row.output_key} className="grid min-w-0 items-center gap-x-6 gap-y-3 rounded-xl bg-teal-50 p-4 dark:bg-teal-950/50 @[42rem]/analysis:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <div><p className="text-xs text-muted-foreground">{output.name || output.key} · 最近输出</p>
          <div className="my-1 break-all font-mono text-2xl font-semibold tracking-tight md:text-3xl">{row.value ?? '无结果'} {row.value != null && <span className="whitespace-nowrap font-sans text-sm font-normal"><QuantityLabel quantity={output.quantity_spec}/></span>}</div>
          <p className="text-xs text-muted-foreground">{reportTime(row.at_ns)}</p></div>
        <div className="break-words text-sm font-medium"><ReadingMeaning detail={detail} row={row} output={output} inputs={inputs}/></div>
      </div>;
    })}
    {detail.version.output_rows === '0' && <p className="rounded-lg bg-muted/40 p-3 text-sm">本次时段没有产生输出。</p>}
    <p className="text-sm text-muted-foreground">{detail.version.output_rows != null && '共 ' + detail.version.output_rows + ' 条输出 · '}{inputStateLabel(detail.version.input_state)}</p>
  </div>;
}

function RowEvidence({row, detail, inputs}: {row: Row; detail: AnalysisArtifactDetail; inputs: Inputs}) {
  const m = useDerived();
  const at = BigInt(row.at_ns);
  const range = row.at_ns === detail.version.output_range.to_ns ? detail.version.output_range :
    {from_ns:row.at_ns, to_ns:at === MAX_NS ? null : (at + 1n).toString()};
  const query = useQuery({queryKey:[...m.prefix, 'artifact-row-evidence', detail.version.id, row.output_key, row.at_ns],
    queryFn:({signal}) => m.client.analysisArtifactLineage(m.scope, detail.artifact.id, {
      version_id:detail.version.id, outputs:[row.output_key], output_range:range, include_samples:true,
    }, signal), retry:false, refetchInterval:30000});
  return <div className="my-2 space-y-2 rounded-lg bg-muted/45 p-3 text-sm">
    <p>{row.count} 个合格样本 · {row.excluded_count} 个排除{row.rounded && ' · 已按固定精度舍入'}</p>
    {row.input_first_ns && row.input_last_ns && <p>实际输入时间 {reportTime(row.input_first_ns)} 至 {reportTime(row.input_last_ns)}</p>}
    {!!row.reason_flags.length && <p>{row.reason_flags.map(k => reasonNames[k] || k).join('、')}</p>}
    <CommandError error={query.error}/>{query.isPending && <LoadingState/>}
    {query.data && !query.error && <><p className="font-medium">固定快照中的候选样本</p>
      {!query.data.samples?.length && <p>该输出所需的范围内没有输入样本。</p>}
      <ul className="divide-y">{query.data.samples?.map(s => {
        const source = inputs[s.channel_id];
        return <li key={s.channel_id + s.sample_id} className="space-y-0.5 py-2"><strong>{source?.label || s.channel_id}</strong>
          <p className="break-all">{s.value ?? '无结果'} {source?.unit} · {qualityNames[s.quality] || s.quality} · {s.source_kind === 'direct' ? '原始观测' : '正式派生'}</p>
          <p className="text-xs text-muted-foreground">{reportTime(s.at_ns)} · {sampleOffset(s.at_ns, row.at_ns)}{s.matched === true ? ' · 按规则命中' : s.matched === false ? ' · 未命中' : ''}</p></li>;
      })}</ul>{query.data.samples_truncated && <p>仅展示前 25 个候选样本，完整清单可在导出中获取。</p>}
      <p className="break-all text-xs text-muted-foreground">固定快照 {query.data.raw_snapshot.id}</p></>}
  </div>;
}

export function ArtifactReadings({items, detail, inputs, pagination}: {items: Row[]; detail: AnalysisArtifactDetail; inputs: Inputs; pagination: ListPage}) {
  const [expanded, setExpanded] = useState('');
  return <div className="min-w-0"><div className="overflow-x-auto"><table aria-label="计算输出与匹配解释" className="w-full table-fixed text-left text-sm">
    <thead className="border-b text-xs text-muted-foreground"><tr><th className="w-1/3 py-2">输出时刻</th><th>数值与质量</th><th className="w-20 text-right">计算依据</th></tr></thead>
    {items.map(row => {
      const open = expanded === row.result_id, output = detail.outputs.find(o => o.key === row.output_key);
      return <tbody key={row.result_id} className="border-b"><tr><td className="py-3 pr-1 text-xs">{reportTime(row.at_ns)}</td>
        <td className="py-3 pr-2"><strong className="break-all tabular-nums">{row.value ?? '无结果'}</strong> {row.value != null && <span className="text-xs"><QuantityLabel quantity={output?.quantity_spec}/></span>}
          <small className="block text-muted-foreground">{output?.name || row.output_key} · {qualityNames[row.quality] || row.quality} · {statusLabels[row.calculation_status] || row.calculation_status}</small></td>
        <td className="text-right"><button type="button" aria-label={'查看 ' + reportTime(row.at_ns) + ' ' + (output?.name || row.output_key) + ' 的依据'} aria-expanded={open} onClick={() => setExpanded(open ? '' : row.result_id)}
          className="inline-flex min-h-11 items-center gap-1 text-xs font-medium text-teal-800 dark:text-teal-300">{row.value === null ? '缺值原因' : '看依据'}<ChevronDown size={14}/></button></td></tr>
        {open && <tr><td colSpan={3}><RowEvidence row={row} detail={detail} inputs={inputs}/></td></tr>}</tbody>;
    })}</table></div>
    {items.length === 0 && <p className="py-4 text-sm text-muted-foreground">此范围没有输出时刻。</p>}
    <div className="mt-4"><ListPagination label="结果" {...pagination}/></div>
  </div>;
}
