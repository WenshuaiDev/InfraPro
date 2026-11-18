import type { DerivedObservation, MonitoringDetailPage } from '@infrapro/api-client';

export const calculationStatusNames: Record<DerivedObservation['calculation_status'], string> = {
  valid: '有效结果', baseline_invalid: '基准失效', noncausal_baseline: '基准晚于结果',
  configuration_boundary: '跨配置边界', invalid_input: '输入不可用', missing_input: '缺少输入',
  insufficient_samples: '样本不足', gap_exceeded: '样本间隔过大', age_exceeded: '样本过旧',
  division_by_zero: '除数为零', numeric_out_of_range: '数值超出范围', numeric_underflow: '数值精度下溢', precision_unresolved: '精度无法确定',
};
export const reasonNames: Record<string, string> = { ...calculationStatusNames, job_lease_lost: '工作进程的执行租约已失效。', job_attempts_exhausted: '工作进程的执行租约已到期，已达到自动尝试上限。', authorization_changed: '执行期间授权发生变化。', execution_principal_unavailable: '原执行资格已失效。', task_execution_failed: '执行失败，详细原因受当前权限限制。', empty_window: '空窗口', quality_excluded: '已排除不符合质量要求的样本', rounded: '按配置精度舍入' };
export type DetailRow = MonitoringDetailPage['items'][number];
export function observationTime(row: DetailRow | null | undefined) { return row?.source_kind === 'derived' ? row.output_at : row?.observed_at; }
export function observationQuality(row: DetailRow) { return row.source_kind === 'derived' ? row.data_quality : row.quality; }
export function observationKind(row: DetailRow) { return row.source_kind === 'derived' ? 'decimal' : row.value_kind; }
export function observationRevoked(row: DetailRow) { return row.source_kind === 'direct' && row.revoked; }
export function plotCoefficient(value: string, scale: number): bigint { const negative = value.startsWith('-'); const [whole, fraction = ''] = (negative ? value.slice(1) : value).split('.'); return BigInt(`${whole}${fraction.padEnd(scale, '0')}`) * (negative ? -1n : 1n); }
export function detailRowKey(row: DetailRow) {
  return row.source_kind === 'derived' ? `${row.result_id}:${row.generation_id}` : `${row.observation_id}:${row.adoption_id}`;
}
