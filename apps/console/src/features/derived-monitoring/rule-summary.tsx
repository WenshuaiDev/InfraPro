import { useQuery } from '@tanstack/react-query';
import type { DerivedQuantity, DerivedRule } from '@infrapro/api-client';
import { useDerived } from './common';

const operatorNames: Record<string, string> = { add: '求和', subtract: '差值', multiply: '乘积', divide: '比值', abs: '绝对值', min: '最小值', max: '最大值', weighted_mean: '加权均值', formula: '公式', baseline_delta: '基准变化', baseline_ratio: '基准变化率', difference: '时间差分', rate: '变化速率', window_count: '窗口计数', window_sum: '窗口求和', window_min: '窗口最小值', window_max: '窗口最大值', window_range: '窗口极差', window_mean: '窗口均值', window_variance: '窗口方差', window_stddev: '窗口标准差', window_median: '窗口中位数', window_quantile: '窗口分位数' };
export function QuantityLabel({ quantity }: { quantity?: DerivedQuantity }) {
  const m = useDerived();
  const unit = useQuery({ queryKey: [...m.prefix, 'fixed-public-version', 'units', quantity?.base_unit_version_id], queryFn: ({ signal }) => m.client.resourceCatalogVersion(m.scope, 'units', quantity!.base_unit_version_id!, signal), enabled: !!quantity?.base_unit_version_id });
  if (!quantity) return <span>单位由规则推导</span>;
  const symbol = unit.data?.version.content.unit?.symbol ?? unit.data?.version.content.name ?? (unit.isFetching ? '读取单位…' : '固定单位');
  const power = (value: number) => Math.abs(value) === 1 ? '' : Math.abs(value) === 2 ? '²' : Math.abs(value) === 3 ? '³' : `^${Math.abs(value)}`;
  const numerator: string[] = [], denominator: string[] = [];
  if (quantity.base_power !== 0) (quantity.base_power > 0 ? numerator : denominator).push(symbol + power(quantity.base_power));
  if (quantity.base_power === 0 && quantity.dimensionless_kind === 'count') numerator.push('计数');
  if (quantity.time_power !== 0) (quantity.time_power > 0 ? numerator : denominator).push(({second:'s',minute:'min',hour:'h',day:'d'})[quantity.time_unit ?? 'second'] + power(quantity.time_power));
  const label = numerator.join(' · ') || (denominator.length ? '1' : '无量纲');
  return <span>{label}{denominator.length ? ` / ${denominator.length > 1 ? `(${denominator.join(' · ')})` : denominator[0]}` : ''}</span>;
}
export function RuleSummary({ rule, compact = false }: { rule: DerivedRule; compact?: boolean }) {
  if (compact) return <div className="space-y-1 text-sm"><p>{rule.nodes.map(n => operatorNames[n.op] ?? n.op).join(' → ')}</p><p className="text-xs text-muted-foreground">{rule.inputs.length} 个输入 · {rule.nodes.length} 个步骤 · <QuantityLabel quantity={rule.outputs[0]?.quantity_spec} /></p></div>;
  return <aside aria-label="规则摘要" className="space-y-5 rounded-xl border bg-muted/20 p-5 xl:sticky xl:top-4">
    <div><h2 className="font-semibold">规则摘要</h2><p className="mt-1 text-xs text-muted-foreground">随编辑更新，发布以保存后的校验结果为准。</p></div>
    <div className="space-y-2"><h3 className="text-xs font-medium text-muted-foreground">输入要求</h3>{rule.inputs.map((input, i) => <div key={i} className="rounded-lg border bg-background px-3 py-2 text-sm"><span className="font-medium">{input.name || input.key}</span><span className="ml-2 text-xs text-muted-foreground">{input.key}</span><p className="text-xs text-muted-foreground"><QuantityLabel quantity={input.quantity_spec} /> · {input.time_semantics.kind === 'instant' ? '瞬时样本' : input.time_semantics.kind === 'interval_end' ? '区间结束' : '派生窗口'}</p></div>)}</div>
    <div className="space-y-2"><h3 className="text-xs font-medium text-muted-foreground">计算顺序</h3>{rule.nodes.map((node, i) => <p key={i} className="break-words text-sm"><span className="mr-2 text-muted-foreground">{i + 1}.</span>{operatorNames[node.op] ?? node.op}<span className="ml-2 text-xs text-muted-foreground">{node.id}</span></p>)}</div>
    <div className="space-y-2 border-t pt-4"><h3 className="text-xs font-medium text-muted-foreground">输出</h3>{rule.outputs.map((output, i) => <div key={i} className="text-sm"><p className="font-medium">{output.name || output.key}</p><p className="text-xs text-muted-foreground"><QuantityLabel quantity={output.quantity_spec} /> · {output.numeric_contract.scale} 位小数</p></div>)}</div>
    <p className="text-xs text-muted-foreground">固定版本可供多个通道复用。发布新版本不会自动替换现有通道的规则。</p>
  </aside>;
}
