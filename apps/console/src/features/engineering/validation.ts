import type { EngineeringContent, EngineeringIssue, EngineeringSample } from '@infrapro/api-client';
import { validateFields } from '../engineering-fields/validation';
export { fieldRule, validateFields, defaults } from '../engineering-fields/validation';
const issue = (code: string, message: string, path: string, key = ''): EngineeringIssue => ({ code, message, path, key, severity: 'error' });
export function validateSample(c: EngineeringContent, sample: EngineeringSample): EngineeringIssue[] {
 const issues = validateFields(c.fields ?? [], sample.values, '/sample/values');
 if (sample.nodes.length > 500) return [...issues, issue('input_limit', '样例最多 500 个构件。', '/sample/nodes')];
 const nodes = new Map(sample.nodes.map((n) => [n.id, n]));
 for (const [index, n] of sample.nodes.entries()) { const path = `/sample/nodes/${index}`; const cat = c.components?.find((v) => v.key === n.category); if (cat) issues.push(...validateFields(cat.fields, n.values, `${path}/values`)); else issues.push(issue('unknown_component', '未定义的构件类别。', `${path}/category`, n.id));
 if (sample.nodes.filter((v) => v.id === n.id).length > 1 || !n.id || n.id === 'root') issues.push(issue('duplicate_node', '构件标识无效或重复。', `${path}/id`, n.id));
 const parent = n.parent === 'root' ? 'root' : nodes.get(n.parent)?.category;
 if (!parent) issues.push(issue('parent_missing', '父级样例不存在。', `${path}/parent`, n.id)); else if (!c.relations?.some((r) => r.parent === parent && r.child === n.category)) issues.push(issue('illegal_parent', '此父子类别关系不被类型定义允许。', `${path}/parent`, n.id));
 const seen = new Set<string>(); let cursor = n.id; while (cursor !== 'root' && nodes.has(cursor)) { if (seen.has(cursor)) { issues.push(issue('sample_cycle', '样例组成存在环。', `${path}/parent`, n.id)); break; } seen.add(cursor); cursor = nodes.get(cursor)!.parent; }
 }
 for (const n of [{ id: 'root', category: 'root' }, ...sample.nodes]) for (const r of c.relations ?? []) if (r.parent === n.category) { const count = sample.nodes.filter((v) => v.parent === n.id && v.category === r.child).length; if (count < r.min_count || (r.max_count !== undefined && count > r.max_count)) issues.push(issue('component_count', `构件 ${r.child} 的数量 ${count} 不符合定义。`, n.id === 'root' ? '/sample/nodes' : `/sample/nodes/${sample.nodes.findIndex((v) => v.id === n.id)}`, r.child)); }
 return issues;
}
