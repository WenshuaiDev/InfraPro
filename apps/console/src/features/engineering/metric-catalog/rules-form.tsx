import type { EngineeringContent } from '@infrapro/api-client';
import { MetricRuleEditor } from './rule-editor';
import { useMetricUnitChoices } from './use-metric-unit-choices';

export function MetricRulesForm({ content, change }: { content: EngineeringContent; change: (next: EngineeringContent) => void }) {
  const units = useMetricUnitChoices();
  return <MetricRuleEditor content={content} change={change} {...units} />;
}
