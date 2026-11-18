import type { EngineeringContent, EngineeringMetric } from '@infrapro/api-client';
import { CheckControl, NumberControl, OptionsEditor, SelectControl } from '../controls';
import type { UnitChoice } from '../metric-units';
import { facilityOrder, metricFacts } from './facts';
import { UnitEditor } from './unit-editor';

export function MetricRuleEditor({ content, change, known, remember }: { content: EngineeringContent; change: (content: EngineeringContent) => void; known: Map<string, UnitChoice>; remember: (choice: UnitChoice) => void }) {
  const metric = content.metric;
  if (!metric) return null;
  const update = (patch: Partial<EngineeringMetric>) => change({ ...content, metric: { ...metric, ...patch } });
  const { topics } = metricFacts(content);
  return <section className="space-y-4">
    <fieldset className="space-y-2"><legend className="mb-2 font-semibold">设施索引</legend><div className="flex flex-wrap gap-x-5 gap-y-2">
      {[...new Set([...facilityOrder, ...topics])].map(label => <CheckControl key={label} label={label} value={topics.includes(label)} onChange={checked => update({ facility_topics: checked ? [...topics, label] : topics.filter(topic => topic !== label) })} />)}
    </div><p className="text-xs text-muted-foreground">仅用于目录导航，可多选；不限制通道采用。</p></fieldset>
    <SelectControl label="指标数据形态" value={metric.shape} options={[{ value: 'decimal', label: '单值小数' }, { value: 'integer', label: '单值整数' }, { value: 'enum', label: '枚举状态' }]}
      onChange={shape => { if (!shape || shape === metric.shape) return; change({ ...content, metric: shape !== 'enum' && metric.shape !== 'enum' ? { ...metric, shape: shape as 'integer' | 'decimal' } :
        { shape: shape as EngineeringMetric['shape'], facility_topics: metric.facility_topics, standard_unit: '', allowed_units: [], category: '', options: [] } }); }} />
    {metric.shape === 'enum' ? <><p className="text-sm">枚举记录状态编码，不关联单位。</p><OptionsEditor value={metric.options ?? []} onChange={options => update({ options })} /></> :
      <><UnitEditor metric={metric} known={known} remember={remember} change={next => change({ ...content, metric: next })} />
        <div className="grid gap-3 sm:grid-cols-2"><NumberControl label="指标下限" value={metric.minimum} onChange={minimum => update({ minimum })} /><NumberControl label="指标上限" value={metric.maximum} onChange={maximum => update({ maximum })} /></div>
        <p className="text-xs text-muted-foreground">边界按记录原值校验，不进行跨单位换算。</p></>}
  </section>;
}
