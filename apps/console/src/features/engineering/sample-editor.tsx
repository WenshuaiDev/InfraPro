import { useState } from 'react';
import type { EngineeringContent, EngineeringField, EngineeringIssue, EngineeringSample, EngineeringVersion } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { ConfirmDialog } from '@infrapro/ui/patterns/confirm-dialog';
import { requestUUID } from '../tenancy/membership-utils';
import { Section, SelectControl, ValueInput } from './controls';
import { defaults } from './validation';
import { legalSampleParent, nodeCategory, nodeKind, nodeParent, sampleAncestors, sampleFields, type SampleKind, type SampleNode } from './structure-sample';
import { structureLabels } from './structure-model';
function DynamicFields({ fields, values, onChange, path, issues, known }: { fields: EngineeringField[]; values: Record<string, unknown>; onChange: (v: Record<string, unknown>) => void; path: string; issues: EngineeringIssue[]; known: EngineeringVersion[] }) {
  return <div className="grid gap-4 sm:grid-cols-2">{[...fields].sort((a, b) => a.order - b.order).map((f, index) => <div className="space-y-2" key={`${f.key}:${index}`}><ValueInput id={`${path}/${f.key}`} field={f} value={values[f.key]} onChange={(v) => { const next = { ...values }; if (v === undefined) delete next[f.key]; else next[f.key] = v; onChange(next); }} />{f.unit_version && <p className="text-xs text-muted-foreground">单位：{known.find((v) => v.id === f.unit_version)?.content.name} · 固定 V{known.find((v) => v.id === f.unit_version)?.number}</p>}{issues.filter((v) => v.path === `${path}/${f.key}`).map((v, i) => <p role="alert" className="text-sm text-destructive" key={i}>{v.message}</p>)}</div>)}</div>;
}
export function SampleEditor({ content, sample, onChange, issues, known }: { content: EngineeringContent; sample: EngineeringSample; onChange: (v: EngineeringSample) => void; issues: EngineeringIssue[]; known: EngineeringVersion[] }) {
  const [category, setCategory] = useState('');
  const [kind, setKind] = useState<SampleKind>('component');
  const [parent, setParent] = useState('root');
  const [remove, setRemove] = useState('');
  const modern = content.metamodel_version === '2';
  const categories = content.components ?? [];
  const label = (n: SampleNode) => `${nodeKind(n) === 'component' ? categories.find((c) => c.key === nodeCategory(n))?.name ?? '构件' : structureLabels[nodeKind(n)]} · 样例 ${sample.nodes.indexOf(n) + 1}`;
  const containers = sample.nodes.filter((n) => nodeKind(n) !== 'point');
  const kinds: SampleKind[] = ['component', ...(modern ? ['point' as const, ...(['monitoring_section', 'borehole'] as const).filter((k) => content.structure?.[k]?.supported)] : [])];
  const parentOptions = [{ value: 'root', label: '设施根对象' }, ...containers.map((n) => ({ value: n.id, label: label(n) }))];
  const updateNode = (id: string, patch: Partial<SampleNode>) => onChange({ ...sample, nodes: sample.nodes.map((n) => n.id === id ? { ...n, ...patch } : n) });
  const allowed = legalSampleParent(content, sample.nodes, kind, category, parent);
  return <div className="space-y-4"><Section title="设施根对象样例"><DynamicFields fields={content.fields ?? []} values={sample.values} onChange={(values) => onChange({ ...sample, values })} path="/sample/values" issues={issues} known={known} /></Section><Section title={modern ? '混合工程树样例' : '样例构件树'} id="/sample/nodes"><div className="grid items-end gap-3 sm:grid-cols-2"><SelectControl label="新增节点的父级" value={parent} onChange={(v) => { setParent(v); setCategory(''); }} options={parentOptions} />{modern && <SelectControl label="样例节点种类" value={kind} options={kinds.map((k) => ({ value: k, label: structureLabels[k] }))} onChange={(value) => { const next = kinds.find((k) => k === value); if (next) { setKind(next); setCategory(''); } }} />}{kind === 'component' && <SelectControl label="新增样例构件类别" value={category} onChange={setCategory} options={categories.filter((c) => legalSampleParent(content, sample.nodes, kind, c.key, parent)).map((c) => ({ value: c.key, label: c.name }))} />}<Button type="button" disabled={!allowed || sample.nodes.length >= 500} onClick={() => { const node: SampleNode = { id: requestUUID(), parent, category: kind === 'component' ? category : '', values: {}, ...(modern ? { kind, parent_ref: parent, component_category_key: kind === 'component' ? category : '' } : {}) }; node.values = defaults(sampleFields(content, node)); onChange({ ...sample, nodes: [...sample.nodes, node] }); }}>添加样例{structureLabels[kind]}</Button></div><p className="text-sm text-muted-foreground">样例最多 500 个节点，仅用于建模参考；不限制实际工程必须按此路径建档。</p>{sample.nodes.map((n, index) => {
    const subtree = new Set(sample.nodes.filter((v) => sampleAncestors(sample.nodes, v.id).some((a) => a.id === n.id)).map((v) => v.id));
    const options = parentOptions.filter((p) => !subtree.has(p.value) && legalSampleParent(content, sample.nodes, nodeKind(n), nodeCategory(n), p.value));
    const path = sampleAncestors(sample.nodes, nodeParent(n)).reverse().map(label).join(' / ');
    return <Section key={n.id} title={label(n)} id={`/sample/nodes/${index}`} action={<ConfirmDialog trigger={<Button type="button" variant="ghost">删除样例 {index + 1}</Button>} open={remove === n.id} onOpenChange={(open) => setRemove(open ? n.id : '')} title="删除样例子树" description={`将删除 ${subtree.size} 个样例节点，不影响正式工程数据。`} onConfirm={() => { onChange({ ...sample, nodes: sample.nodes.filter((v) => !subtree.has(v.id)) }); setRemove(''); }} />}><p className="break-words text-xs text-muted-foreground">设施根对象{path ? ` / ${path}` : ''}</p><SelectControl label={`样例 ${index + 1} 的父级`} value={nodeParent(n)} onChange={(parent) => updateNode(n.id, { parent, ...(modern ? { parent_ref: parent } : {}) })} options={options} /><DynamicFields fields={sampleFields(content, n)} values={n.values} onChange={(values) => updateNode(n.id, { values })} path={`/sample/nodes/${index}/values`} issues={issues} known={known} /></Section>;
  })}{!sample.nodes.length && <p className="text-sm text-muted-foreground">还没有样例节点，可先建立结构，再验证完整性。</p>}</Section></div>;
}
