import type { ApiClient, EngineeringContent, EngineeringDefinition, EngineeringMetric, EngineeringSummary, EngineeringVersion } from '@infrapro/api-client';
import { facilityIndex } from './facility-index';

export const variants = [
  { key: 'A', name: '规则台账' }, { key: 'B', name: '主题导览' }, { key: 'C', name: '样例手册' },
] as const;
export type Variant = typeof variants[number]['key'];
export const scope = { kind: 'metrics' } as const;
export const facilities = ['隧道', '桥梁', '路基', '路面', '涵洞', '边坡', '气象监测站', '未编入索引'];
export const facilitiesFor = (code: string) => facilityIndex.get(code) ?? ['未编入索引'];
export const shapes = { decimal: '小数', integer: '整数', enum: '枚举' };
export const states = { all: '全部指标', usable: '可新使用', draft: '有草稿', unavailable: '不可新使用' };
export type Status = keyof typeof states;
export const hasStatus = (item: EngineeringSummary, status: Status) => status === 'all' || (status === 'draft' ? item.has_draft : status === 'usable' ? item.usable : !item.usable);
export const itemStatus = (item: EngineeringSummary) => item.usable ? '可新使用' : item.latest ? '不可新使用' : '待发布';

export async function readIndex(client: ApiClient, signal: AbortSignal) {
  const rows = new Map<string, EngineeringSummary>();
  let offset = 0;
  for (;;) {
    const page = await client.engineeringList(scope, { offset, limit: 100 }, signal);
    for (const item of page.items) rows.set(item.id, item);
    offset += page.items.length;
    if (!page.items.length || offset >= page.total) return [...rows.values()];
  }
}

export function detailQuery(client: ApiClient, account: string | undefined, id: string) {
  return { queryKey: ['platform', account, 'engineering', false, 'metrics', 'detail', id],
    queryFn: ({ signal }: { signal: AbortSignal }) => client.engineeringDetail(scope, id, signal), staleTime: 120_000, retry: false as const };
}

export type RecordView = { definition: EngineeringDefinition; content: EngineeringContent; rule: EngineeringMetric; version?: EngineeringVersion };
export function recordView(definition?: EngineeringDefinition, selection?: string): RecordView | undefined {
  if (!definition) return;
  const version = selection === 'draft' ? undefined : definition.versions.find(item => item.id === selection) ?? definition.versions[0];
  const content = version?.content ?? definition.draft?.content;
  if (content?.metric) return { definition, content, rule: content.metric, version };
}
export const versionState = (version?: EngineeringVersion) => !version ? '待发布' : version.usable ? '可新使用' : version.enabled ? '依赖不可用' : '已停用';
export const stamp = (value: string) => new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date(value));
export function description(content: EngineeringContent) {
  const lines = content.description.trim().split(/\n+/).map(line => line.trim());
  const field = (prefix: string) => lines.find(line => line.startsWith(prefix))?.slice(prefix.length).replace(/。$/u, '') ?? '';
  return { intro: (lines.find(line => line.startsWith('定义：')) ?? lines[0] ?? '').replace(/^定义[：:]\s*/u, ''), facilities: field('适用设施：'), category: field('类别：') };
}
export function unitRef(view: RecordView, id: string) {
  const unit = view.definition.dependencies.find(item => item.kind === 'units' && item.id === id);
  return { name: unit?.content.name ?? (id ? '未解析的单位版本' : '未设置'), symbol: unit?.content.unit?.symbol ?? '', number: unit?.number, category: unit?.unit_category?.name };
}
export const standardSymbol = (view: RecordView) => view.rule.shape === 'enum' ? '无单位' : unitRef(view, view.rule.standard_unit).symbol || unitRef(view, view.rule.standard_unit).name;
export function bounds(rule: EngineeringMetric) {
  if (rule.shape === 'enum') return `${rule.options?.length ?? 0} 个枚举选项`;
  if (rule.minimum != null && rule.maximum != null) return `${rule.minimum} ≤ 值 ≤ ${rule.maximum}`;
  if (rule.minimum != null) return `值 ≥ ${rule.minimum}`;
  if (rule.maximum != null) return `值 ≤ ${rule.maximum}`;
  return '未设上下限';
}
const samples: Record<string, number> = { metric_illuminance: 120, metric_luminance: 4.5, metric_smoke_extinction: 0.006, metric_settlement: 3.2, metric_displacement: 2.4, metric_dust_concentration: 0.12, metric_oxygen_volume_fraction: 20.9 };
export function example(view: RecordView) {
  const { rule } = view;
  if (rule.shape === 'enum') return rule.options?.[0]?.key ?? '';
  let value = samples[view.definition.code] ?? (rule.shape === 'integer' ? 3 : 2.5);
  if (rule.minimum != null) value = Math.max(rule.minimum, value);
  if (rule.maximum != null) value = Math.min(rule.maximum, value);
  return String(rule.shape === 'integer' ? Math.trunc(value) : value);
}
export function exampleLabel(view: RecordView, value: string) {
  return view.rule.shape === 'enum' ? view.rule.options?.find(option => option.key === value)?.name ?? (value || '尚无选项') : value || '—';
}
export type Trial = { raw: string; unit: string };
export const initialTrial = (view: RecordView): Trial => ({ raw: example(view), unit: view.rule.shape === 'enum' ? '' : view.rule.standard_unit });
const featured = ['metric_illuminance', 'metric_bridge_condition_grade', 'metric_displacement', 'metric_luminance', 'metric_precipitation_type', 'metric_settlement'];
export function sampleOrder(rows: EngineeringSummary[]) {
  return [...rows.filter(row => featured.includes(row.code)).sort((left, right) => featured.indexOf(left.code) - featured.indexOf(right.code)), ...rows.filter(row => !featured.includes(row.code))];
}
