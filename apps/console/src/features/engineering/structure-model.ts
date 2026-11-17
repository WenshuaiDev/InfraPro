import type { EngineeringContent, EngineeringItem } from '@infrapro/api-client';

export const structureLabels = { facility: '基础设施', component: '构件', monitoring_section: '监测断面', borehole: '孔位', point: '测点' };
export type TemplateTarget = EngineeringItem['targets'][number];
export function targetLabel(target: TemplateTarget, type?: EngineeringContent) {
  if (target.applicability === 'not_applicable') return `不适用：${target.reason ?? '未填写原因'}`;
  if (!target.anchor_kind) return target.component === 'root' ? '设施根对象' : type?.components?.find((c) => c.key === target.component)?.name ?? target.component;
  const name = target.anchor_kind === 'component' ? type?.components?.find((c) => c.key === target.component)?.name ?? '未指定构件类别' : structureLabels[target.anchor_kind];
  return `${name} · ${target.point_scope === 'self' ? '自身测点' : '后代测点'} · ${target.quantity_basis === 'per_anchor' ? '每个锚点' : '每个测点'}${target.requires_borehole_ancestor ? ' · 须有孔位祖先' : ''}`;
}
export function upgradeStructureContent(content: EngineeringContent): EngineeringContent {
  return { ...content, metamodel_version: '2', ...(content.template ? { template: { ...content.template, items: content.template.items.map((item) => ({ ...item, targets: item.targets.map((target): TemplateTarget => ({ ...target, component: target.component === 'root' ? '' : target.component, applicability: 'applicable', anchor_kind: target.component === 'root' ? 'facility' : 'component', point_scope: 'descendant_points', quantity_basis: 'per_anchor' })) })) } } : { structure: { monitoring_section: { supported: false, fields: [] }, borehole: { supported: false, fields: [] } } }) };
}
