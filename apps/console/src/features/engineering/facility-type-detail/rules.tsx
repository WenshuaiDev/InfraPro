import { useRef, useState } from 'react';
import { ArrowDown, Box, Check, ChevronRight, ClipboardList, CornerDownRight, GitBranch, Layers, MapPin, PencilLine, Plus, ScanLine, Search, Waypoints } from 'lucide-react';
import type { EngineeringContent, EngineeringField, EngineeringVersion } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { TextControl } from '../controls';
import { FieldDetails } from './details';
import { placeMeanings, relationQuantity } from './facts';
import { FieldWorkspace } from './field-workspace';
import { CapabilitySettings, PartSettings } from './rule-settings';

type Props = {
  content: EngineeringContent; target: string; startEditing: boolean; setTarget: (key: string) => void;
  units: EngineeringVersion[]; remember: (v: EngineeringVersion) => void; edit?: (content: EngineeringContent) => void;
};
const capKeys = { section: 'monitoring_section', borehole: 'borehole' } as const;
const tools = { facility: Layers, component: Box, section: ScanLine, borehole: Waypoints };
type Location = { id: string; name: string; kind: keyof typeof tools };
type RuleTab = 'relations' | 'fields' | 'edit';

export function RuleExplorer({ content, target, setTarget, startEditing, units, remember, edit }: Props) {
  const parts = content.components ?? [];
  const rules = [...(content.relations ?? [])].sort((a, b) => a.order - b.order);
  const facility: Location = { id: 'root', name: content.name || '设施本体', kind: 'facility' };
  const components: Location[] = parts.map((p) => ({ id: 'component/' + p.key, name: p.name || p.key, kind: 'component' }));
  const caps: Location[] = [{ id: 'section', name: '监测断面', kind: 'section' }, { id: 'borehole', name: '孔位', kind: 'borehole' }];
  const locations = [facility, ...components, ...caps];
  const current = locations.find((p) => p.id === target) ?? components[0] ?? facility;
  const part = current.kind === 'component' ? parts.find((p) => 'component/' + p.key === current.id) : undefined;
  const capKey = current.kind === 'section' || current.kind === 'borehole' ? capKeys[current.kind] : undefined;
  const cap = capKey && content.metamodel_version === '2' ? content.structure?.[capKey] : undefined;
  const fields = part?.fields ?? (capKey ? cap?.fields : content.fields) ?? [];
  const heading = useRef<HTMLHeadingElement>(null);
  const [query, search] = useState('');
  const [tab, setTab] = useState<RuleTab>(edit && startEditing && (capKey || part) ? 'edit' : 'relations');
  const [create, setCreate] = useState(false);
  const [newName, setName] = useState(''), [newKey, setKey] = useState(''), [parent, setParent] = useState('root');
  const Symbol = tools[current.kind];
  const ownerKey = part?.key ?? 'root';
  const parents = part ? rules.filter((r) => r.child === ownerKey && r.max_count !== 0) : [];
  const children = capKey ? [] : rules.filter((r) => r.parent === ownerKey && r.max_count !== 0);
  const excluded = capKey ? [] : rules.filter((r) => r.max_count === 0 && (r.parent === ownerKey || r.child === ownerKey));
  const available = components.filter((r) => r.name.includes(query.trim()) || r.id.slice(10).toLowerCase().includes(query.trim().toLowerCase()));
  const commonQuantity = children.length > 1 && children.every((r) => r.min_count === children[0]!.min_count && r.max_count === children[0]!.max_count)
    ? relationQuantity(children[0]!) : '';
  const modelCap = (id: string) => content.metamodel_version === '2' ? content.structure?.[id === 'section' ? 'monitoring_section' : 'borehole']?.supported : undefined;
  const capState = (value?: boolean) => value == null ? '尚未声明' : value ? '可按需建立' : '不适用';
  function choose(id: string) { setCreate(false); setTab('relations'); setTarget(id); }
  function navigate(key: string) {
    choose(key === 'root' ? key : 'component/' + key);
    requestAnimationFrame(() => {
      heading.current?.focus({ preventScroll: true });
      heading.current?.closest('[data-slot="drawer-scroll-area"]')?.scrollTo({ top: 0 });
    });
  }
  const nameOf = (key: string) => key === 'root' ? facility.name : parts.find((p) => p.key === key)?.name || key;
  function changeFields(next: EngineeringField[]) {
    if (!edit) return;
    if (part) edit({ ...content, components: parts.map((p) => p === part ? { ...p, fields: next } : p) });
    else if (capKey && cap && content.structure) edit({ ...content, structure: { ...content.structure, [capKey]: { ...cap, fields: next } } });
    else if (!capKey) edit({ ...content, fields: next });
  }
  function addPart() {
    const key = newKey.trim(), name = newName.trim();
    if (!edit || !name || !/^[a-z][a-z0-9_]{0,63}$/.test(key) || key === 'root' || parts.some((p) => p.key === key)) return;
    edit({ ...content, components: [...parts, { key, name, description: '', fields: [] }],
      relations: [...(content.relations ?? []), { parent, child: key, min_count: 0, order: rules.length + 1 }] });
    choose('component/' + key);
  }
  function showAdd() { setCreate(true); setName(''); setKey(''); setParent(ownerKey); }
  const navButton = (p: Location) => {
    const Icon = tools[p.kind];
    return <button type="button" key={p.id} aria-pressed={!create && current.id === p.id} className="ft-nav-item" onClick={() => choose(p.id)}>
      <Icon /><span>{p.name}</span>{!create && current.id === p.id && <ChevronRight className="ft-nav-arrow" />}</button>;
  };
  return <section className="ft-rule-layout" aria-label="工程对象规则">
    <nav className="ft-rail ft-rule-rail" aria-label="工程对象目录">
      <h3>工程对象</h3>{navButton(facility)}
      <div className="ft-rail-label"><span>构件类别</span><span>{parts.length}</span></div>
      {!!parts.length && <label className="ft-search"><Search /><input aria-label="搜索构件" placeholder="搜索构件" value={query} onChange={(e) => search(e.target.value)} /></label>}
      <div className="ft-scroll-list">{available.map(navButton)}
        {!available.length && <p className="ft-hint">{parts.length ? '未找到匹配的构件。' : '此类型没有构件。'}</p>}</div>
      {edit && <button type="button" className="ft-text-action" onClick={showAdd}><Plus />添加构件</button>}
      <div className="ft-rail-label"><span>监测位置</span></div>{caps.map(navButton)}
    </nav>
    <div className="ft-mobile-picker"><label className="ft-label">选择工程对象<select className="ft-select" value={current.id} onChange={(e) => choose(e.target.value)}>
      <option value="root">{facility.name} · 设施本体</option><optgroup label="构件类别">{components.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</optgroup>
      <optgroup label="监测位置">{caps.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</optgroup></select></label>
      {edit && <Button variant="outline" onClick={showAdd}><Plus />添加构件</Button>}</div>
    <article className="ft-detail-body" aria-label="当前对象规则">
      {create && edit ? <div className="ft-edit-form"><h3 className="ft-detail-title">添加构件</h3>
        <TextControl label="新构件名称" value={newName} onChange={setName} />
        <TextControl label="新构件编号" value={newKey} onChange={setKey} hint="使用唯一的小写编号，以字母开头。" />
        <label className="ft-label">归属对象<select className="ft-select" value={parent} onChange={(e) => setParent(e.target.value)}><option value="root">{facility.name}</option>
          {parts.map((p) => <option key={p.key} value={p.key}>{p.name}</option>)}</select></label>
        <div className="ft-actions"><Button variant="outline" onClick={() => setCreate(false)}>取消</Button><Button disabled={!newName.trim() || !/^[a-z][a-z0-9_]{0,63}$/.test(newKey.trim()) || newKey.trim() === 'root' || parts.some((p) => p.key === newKey.trim())} onClick={addPart}>加入草稿</Button></div>
      </div> : <>
        <header className="ft-object-head"><span className="ft-object-symbol"><Symbol /></span><div><p>{part ? '实体构件' : capKey ? '监测位置' : '设施本体'}</p>
          <h3 ref={heading} tabIndex={-1}>{current.name}</h3></div></header>
        <p className="ft-object-desc">{part?.description || placeMeanings[current.kind]}</p>
        <div className="ft-subnav" role="group" aria-label="对象视图">
          <button type="button" aria-pressed={tab === 'relations'} onClick={() => setTab('relations')}><GitBranch />{capKey ? '布设规则' : '归属关系'}</button>
          {(!capKey || cap?.supported) && <button type="button" aria-pressed={tab === 'fields'} onClick={() => setTab('fields')}><ClipboardList />建档资料 <small>{fields.length}</small></button>}
          {edit && (part || capKey) && <button type="button" aria-pressed={tab === 'edit'} onClick={() => setTab('edit')}><PencilLine />编辑规则</button>}
        </div>
        {tab === 'fields' && (edit ? <FieldWorkspace key={current.id} fields={fields} units={units} remember={remember} change={changeFields} /> : <FieldDetails fields={fields} units={units} />)}
        {tab === 'edit' && edit && part && <PartSettings key={current.id} content={content} part={part} edit={edit} select={choose} />}
        {tab === 'edit' && edit && capKey && <CapabilitySettings key={current.id} content={content} kind={capKey} edit={edit} />}
        {tab === 'relations' && (capKey ? <div className="ft-capability">
          <span className="ft-status" data-tone={cap?.supported ? 'green' : 'neutral'}>{cap?.supported && <Check />}{capState(cap?.supported)}</span>
          <h4>{cap?.supported ? '可在' + current.name + '下布设测点' : cap?.supported === false ? '此类型不设' + current.name : '此版本尚未声明' + current.name + '能力'}</h4>
          <p>{cap?.supported ? '建档时可按实际位置设在设施、构件或其他合法工程部位下。' : '测点可以直接归属到设施本体。'}</p>
        </div> : <>
          <div className="ft-map">
            {part && <><p className="ft-map-label">可归属的上级</p><div className="ft-parent-set">{parents.length ? parents.map((r, i) => <button type="button" key={i} onClick={() => navigate(r.parent)}>
              {r.parent === 'root' ? <Layers /> : <Box />}<strong>{nameOf(r.parent)}</strong><span>{relationQuantity(r)}</span><ChevronRight /></button>) : <span className="ft-hint">尚无可用上级</span>}</div>
              <ArrowDown className="ft-wire-arrow" /></>}
            <div className="ft-current-node"><Symbol /><strong>{current.name}</strong></div>
            <div className="ft-point-caption"><MapPin /><span>{part ? '可在此构件下布设测点' : '可直接在设施下布设测点'}</span></div>
          </div>
          {!!children.length && <section className="ft-child-section"><div className="ft-inline-head"><h4>可包含的构件</h4><small>{children.length} 类</small></div>
            {commonQuantity && <p className="ft-child-quantity">{commonQuantity === '按需设置，数量不限'
              ? '每处' + current.name + '都可按需设置以下构件，各类数量不限。' : '每类构件的数量要求相同，' + commonQuantity + '。'}</p>}
            <ul className="ft-children">{children.map((r, i) => <li key={i}><button type="button" onClick={() => navigate(r.child)}>
              <CornerDownRight /><span><strong>{nameOf(r.child)}</strong>{!commonQuantity && <small>{relationQuantity(r)}</small>}</span><ChevronRight /></button></li>)}</ul></section>}
          {!children.length && <p className="ft-hint">{part ? '此构件下没有配置子构件。' : '此版本没有配置直属构件。'}</p>}
          {!!excluded.length && <div className="ft-rule-note ft-hint"><strong>数量上限为零</strong>
            {excluded.map((r, i) => <p key={i}>{nameOf(r.parent)}下不可设置{nameOf(r.child)}。</p>)}</div>}
          {current.kind === 'facility' && <div className="ft-cap-links">{caps.map((p) => <button type="button" key={p.id} onClick={() => choose(p.id)}>
            {p.kind === 'section' ? <ScanLine /> : <Waypoints />}<strong>{p.name}</strong><span>{capState(modelCap(p.id))}</span><ChevronRight /></button>)}</div>}
          {part && caps.some((c) => modelCap(c.id)) && <p className="ft-hint ft-rule-note">断面或孔位可以穿插在构件之间。数量按最近的上级构件计算，顶层按整座设施计算。</p>}
        </>)}
      </>}
    </article>
  </section>;
}
