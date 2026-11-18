import type { EngineeringContent, EngineeringField, EngineeringRelation, EngineeringVersion } from '@infrapro/api-client';
import { shapeLabels } from '../model';

export type DetailPanel = 'fields' | 'structure' | 'versions';

export function versionState(version?: Pick<EngineeringVersion, 'usable' | 'enabled'>, metric = false) {
  if (!version) return '草稿，待发布';
  if (version.usable) return metric ? '可新使用' : '新建设施可选';
  return version.enabled ? '依赖不可用' : '已停用';
}

export function fieldSummary(fields: EngineeringField[]) {
  if (!fields.length) return '此类型没有额外的建档资料';
  const required = fields.filter((field) => field.required).length;
  if (required === fields.length) return fields.length + ' 项，均为必填';
  return fields.length + ' 项，' + (required ? required + ' 项必填' : '均可选填');
}

export function fieldGroups(fields: EngineeringField[]) {
  const groups = new Map<string, EngineeringField[]>();
  for (const field of [...fields].sort((a, b) => a.order - b.order)) {
    const group = field.group || '其他资料';
    if (!groups.has(group)) groups.set(group, []);
    groups.get(group)!.push(field);
  }
  return Array.from(groups);
}

export function fieldShape(field: EngineeringField) {
  if (field.shape === 'enum') return '单选';
  if (field.shape === 'multi_enum') return '多选';
  if (field.shape === 'boolean') return '是或否';
  return shapeLabels[field.shape];
}

export function displayValue(field: EngineeringField, value: unknown): string {
  if (Array.isArray(value)) return value.map((item) => displayValue(field, item)).join('、');
  if (typeof value === 'boolean') return value ? '是' : '否';
  if (value == null) return '';
  return field.options?.find((option) => option.key === value)?.name ?? String(value);
}

export function relationQuantity(rule: EngineeringRelation) {
  if (rule.max_count === 0) return '此处不可设置';
  if (rule.max_count === rule.min_count) return '每处 ' + rule.min_count + ' 个';
  if (rule.max_count != null) return '每处 ' + rule.min_count + ' 至 ' + rule.max_count + ' 个';
  return rule.min_count ? '每处至少 ' + rule.min_count + ' 个' : '按需设置，数量不限';
}

export type ExamplePlace = { id: string; kind: 'facility' | 'component' | 'section' | 'borehole'; name: string };
export const placeLabels = { facility: '设施本体', component: '构件', section: '监测断面', borehole: '孔位' };
export const placeMeanings = {
  facility: '一处独立建档的设施。在这里登记整座设施的资料，也可以直接布设测点。',
  component: '设施的一个实体组成部分。建档时可把测点放在实际观测的构件下。',
  section: '在工程中的一个截面上组织监测。可以把这个截面上的多个测点放在一起。',
  borehole: '工程中有明确位置的孔或孔道。可以在同一孔内的不同深度布设测点。',
  point: '测点是实际进行观测的位置。同一位置的不同观测要素可以分别建立监测通道。',
};

// The drawing shows possible direct placements only. Intermediate levels are described in the rules.
export function examplePlaces(content: EngineeringContent): ExamplePlace[] {
  const places: ExamplePlace[] = [{ id: 'root', kind: 'facility', name: content.name || '设施本体' }];
  const direct = new Set(content.relations?.filter((r) => r.parent === 'root' && r.max_count !== 0).map((r) => r.child));
  for (const part of content.components ?? []) {
    if (direct.has(part.key)) places.push({ id: 'component/' + part.key, kind: 'component', name: part.name || part.key });
  }
  if (content.metamodel_version === '2') {
    if (content.structure?.monitoring_section?.supported) places.push({ id: 'section', kind: 'section', name: '监测断面' });
    if (content.structure?.borehole?.supported) places.push({ id: 'borehole', kind: 'borehole', name: '孔位' });
  }
  return places;
}

export function stamp(value: string, time = false) {
  return new Date(value).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit',
    ...(time ? { hour: '2-digit', minute: '2-digit', hour12: false } : {}) });
}
