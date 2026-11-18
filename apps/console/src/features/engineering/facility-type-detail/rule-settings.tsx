import { useState } from 'react';
import { ChevronDown, Hash, Plus, Trash2 } from 'lucide-react';
import type { EngineeringComponent, EngineeringContent, EngineeringRelation } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { NumberControl, TextControl } from '../controls';
import { upgradeStructureContent } from '../structure-model';

export function CapabilitySettings({ content, kind, edit }: {
  content: EngineeringContent; kind: 'borehole' | 'monitoring_section'; edit: (c: EngineeringContent) => void;
}) {
  const label = kind === 'borehole' ? '孔位' : '监测断面', config = content.structure?.[kind];
  const [confirm, setConfirm] = useState(false);
  if (content.metamodel_version !== '2') return <div className="ft-edit-form"><p className="ft-muted">此旧版尚未声明断面和孔位。可在草稿中补充这两项能力。</p>
    <Button variant="outline" onClick={() => edit(upgradeStructureContent(content))}>启用断面与孔位设置</Button></div>;
  const set = (supported: boolean) => {
    edit({ ...content, structure: { monitoring_section: content.structure?.monitoring_section ?? { supported: false, fields: [] },
      borehole: content.structure?.borehole ?? { supported: false, fields: [] }, [kind]: { supported, fields: supported ? (config?.fields ?? []) : [] } } });
    setConfirm(false);
  };
  return <div className="ft-edit-form"><fieldset className="ft-cap-choice"><legend>是否允许建立{label}</legend>
    <label><input type="radio" name={'support-' + kind} checked={!confirm && config?.supported === true} onChange={() => { if (confirm) setConfirm(false); else set(true); }} />按需建立</label>
    <label><input type="radio" name={'support-' + kind} checked={confirm || config?.supported === false} onChange={() => {
      if (config?.fields?.length) setConfirm(true); else set(false);
    }} />不适用</label></fieldset>
    {confirm && <div className="ft-confirm"><p>关闭后将移除草稿中这类对象的 {config?.fields.length} 项专属字段。</p>
      <div className="ft-actions"><Button variant="outline" onClick={() => setConfirm(false)}>保留</Button><Button onClick={() => set(false)}>确认关闭</Button></div></div>}
  </div>;
}

export function PartSettings({ content, part, edit, select }: {
  content: EngineeringContent; part: EngineeringComponent; edit: (c: EngineeringContent) => void; select: (id: string) => void;
}) {
  const parts = content.components ?? [], all = content.relations ?? [];
  const [remove, setRemove] = useState(false);
  const [newKey, setNewKey] = useState(part.key);
  const key = newKey.trim();
  const canRename = /^[a-z][a-z0-9_]{0,63}$/.test(key) && key !== 'root' && !parts.some((p) => p.key === key);
  function rename() {
    if (!canRename) return;
    edit({
      ...content,
      components: parts.map((p) => p === part ? { ...p, key } : p),
      relations: all.map((r) => ({ ...r, parent: r.parent === part.key ? key : r.parent, child: r.child === part.key ? key : r.child })),
    });
    select('component/' + key);
  }
  function patchPart(patch: Partial<EngineeringComponent>) {
    edit({ ...content, components: parts.map((p) => p === part ? { ...p, ...patch } : p) });
  }
  const parents = all.filter((r) => r.child === part.key);
  const choices = [{ key: 'root', name: content.name || '设施本体' }, ...parts.filter((p) => p.key !== part.key)];
  const unused = choices.filter((c) => !parents.some((r) => r.parent === c.key));
  const patchRule = (rule: EngineeringRelation, patch: Partial<EngineeringRelation>) =>
    edit({ ...content, relations: all.map((r) => r === rule ? { ...r, ...patch } : r) });
  return <div className="ft-edit-form">
    <TextControl label="构件名称" value={part.name} onChange={(name) => patchPart({ name })} />
    <TextControl label="构件说明" value={part.description} onChange={(description) => patchPart({ description })} />
    <div className="ft-inline-head"><h4 className="ft-small-title">归属和数量</h4>
      {!!unused.length && <Button size="sm" variant="outline" onClick={() => edit({ ...content, relations: [...all, { child: part.key, parent: unused[0]!.key, min_count: 0, order: all.length + 1 }] })}><Plus />添加上级</Button>}</div>
    <p className="ft-muted">每一行规定一个上级可以包含多少个此类构件。最多数量可留空。</p>
    {parents.map((r, i) => <div className="ft-relation-edit" key={i}><label className="ft-label">上级对象<select className="ft-select" value={r.parent} onChange={(e) => patchRule(r, { parent: e.target.value })}>
      {choices.filter((c) => c.key === r.parent || unused.includes(c)).map((p) => <option key={p.key} value={p.key}>{p.name}</option>)}</select></label>
      <NumberControl label="最少数量" value={r.min_count} onChange={(n) => patchRule(r, { min_count: n ?? 0 })} />
      <NumberControl label="最多数量" value={r.max_count} onChange={(max_count) => patchRule(r, { max_count })} />
      <Button size="icon" variant="ghost" aria-label={'移除上级 ' + (choices.find((p) => p.key === r.parent)?.name ?? r.parent)}
        onClick={() => edit({ ...content, relations: all.filter((rule) => rule !== r) })}><Trash2 /></Button>
      <details className="ft-rule-order"><summary>显示顺序</summary>
        <NumberControl label="同级排列顺序" value={r.order} onChange={(order) => patchRule(r, { order: order ?? 0 })} /></details></div>)}
    <details className="ft-options"><summary><Hash />构件编号 · {part.key}<ChevronDown /></summary><div className="ft-edit-form">
      <TextControl label="新的构件编号" value={newKey} onChange={setNewKey} hint="修改编号会同步更新本草稿中使用它的归属规则。" />
      <Button variant="outline" disabled={!canRename} onClick={rename}>应用新编号</Button></div></details>
    <div className="ft-actions">
      <Button variant="ghost" onClick={() => setRemove(true)}><Trash2 />移除构件</Button></div>
    {remove && <div className="ft-confirm"><p>从草稿中移除「{part.name}」及其关联的归属规则？</p>
      <div className="ft-actions"><Button variant="outline" onClick={() => setRemove(false)}>取消</Button><Button onClick={() => {
        edit({ ...content, components: parts.filter((p) => p !== part), relations: all.filter((r) => r.parent !== part.key && r.child !== part.key) });
        select('root');
      }}>确认移除</Button></div></div>}
  </div>;
}
