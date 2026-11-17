import type { EngineeringContent, EngineeringIssue, EngineeringSample } from '@infrapro/api-client';
import { validateFields } from '../engineering-fields/validation';
import { legalSampleParent, nearestSampleComponent, nodeCategory, nodeKind, nodeParent, sampleFields } from './structure-sample';
export { fieldRule, validateFields, defaults } from '../engineering-fields/validation';
const issue = (code: string, message: string, path: string, key = ''): EngineeringIssue => ({ code, message, path, key, severity: 'error' });
export function validateSample(content: EngineeringContent, sample: EngineeringSample): EngineeringIssue[] {
  const issues = validateFields(content.fields ?? [], sample.values, '/sample/values');
  if (sample.nodes.length > 500) return [...issues, issue('input_limit', '建模样例最多 500 个节点。', '/sample/nodes')];
  const nodes = new Map(sample.nodes.map((n) => [n.id, n]));
  const modern = content.metamodel_version === '2';
  for (const [index, n] of sample.nodes.entries()) {
    const path = `/sample/nodes/${index}`;
    const kind = nodeKind(n);
    issues.push(...validateFields(sampleFields(content, n), n.values, `${path}/values`));
    if (sample.nodes.filter((v) => v.id === n.id).length > 1 || !n.id || n.id === 'root') issues.push(issue('duplicate_node', '节点标识无效或重复。', `${path}/id`, n.id));
    if (modern && (!n.kind || !n.parent_ref)) issues.push(issue('structure_ref_required', '请选择节点种类和父级。', path, n.id));
    if (kind !== 'component' && nodeCategory(n)) issues.push(issue('structure_kind', '只有构件指定构件类别。', path, n.id));
    if (!legalSampleParent(content, sample.nodes, kind, nodeCategory(n), nodeParent(n))) issues.push(issue('illegal_parent', '此节点种类或最近构件关系不被类型允许。', `${path}/parent_ref`, n.id));
    const seen = new Set<string>();
    let cursor = n.id;
    while (cursor !== 'root' && nodes.has(cursor)) { if (seen.has(cursor)) { issues.push(issue('sample_cycle', '样例结构存在环。', `${path}/parent_ref`, n.id)); break; } seen.add(cursor); cursor = nodeParent(nodes.get(cursor)!); }
  }
  const parents = [{ id: 'root', category: 'root' }, ...sample.nodes.filter((n) => nodeKind(n) === 'component').map((n) => ({ id: n.id, category: nodeCategory(n) }))];
  for (const n of parents) for (const relation of content.relations ?? []) if (relation.parent === n.category) {
    const count = sample.nodes.filter((v) => nodeKind(v) === 'component' && nodeCategory(v) === relation.child && (modern ? nearestSampleComponent(sample.nodes, nodeParent(v)) : nodeParent(v)) === n.id).length;
    if (count < relation.min_count || relation.max_count !== undefined && count > relation.max_count) issues.push(issue('component_count', `构件 ${relation.child} 的数量 ${count} 不符合定义。`, '/sample/nodes', relation.child));
  }
  return issues;
}
