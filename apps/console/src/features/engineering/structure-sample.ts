import type { EngineeringContent, EngineeringSample } from '@infrapro/api-client';
export type SampleNode = EngineeringSample['nodes'][number];
export type SampleKind = NonNullable<SampleNode['kind']>;
export const nodeKind = (node: SampleNode): SampleKind => node.kind ?? 'component';
export const nodeParent = (node: SampleNode): string => node.parent_ref || node.parent;
export const nodeCategory = (node: SampleNode): string => node.component_category_key || node.category;
export function sampleAncestors(nodes: SampleNode[], id: string): SampleNode[] {
  const out: SampleNode[] = [];
  const byId = new Map(nodes.map((n) => [n.id, n]));
  const seen = new Set<string>();
  let cursor = byId.get(id);
  while (cursor && !seen.has(cursor.id)) { seen.add(cursor.id); out.push(cursor); cursor = byId.get(nodeParent(cursor)); }
  return out;
}
export function nearestSampleComponent(nodes: SampleNode[], parent: string): string {
  return sampleAncestors(nodes, parent).find((n) => nodeKind(n) === 'component')?.id ?? 'root';
}
export function sampleFields(content: EngineeringContent, node: SampleNode) {
  const kind = nodeKind(node);
  if (kind === 'component') return content.components?.find((c) => c.key === nodeCategory(node))?.fields ?? [];
  if (kind === 'point') return [];
  return content.structure?.[kind]?.fields ?? [];
}
export function legalSampleParent(content: EngineeringContent, nodes: SampleNode[], kind: SampleKind, category: string, parent: string): boolean {
  if (parent !== 'root' && (!nodes.some((n) => n.id === parent) || nodeKind(nodes.find((n) => n.id === parent)!) === 'point')) return false;
  if (kind !== 'component') return content.metamodel_version === '2' && (kind === 'point' || content.structure?.[kind]?.supported === true);
  const nearest = content.metamodel_version === '2' ? nearestSampleComponent(nodes, parent) : parent;
  const parentCategory = nearest === 'root' ? 'root' : nodeCategory(nodes.find((n) => n.id === nearest)!);
  return content.relations?.some((r) => r.parent === parentCategory && r.child === category) ?? false;
}
