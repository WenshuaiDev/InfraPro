// THROWAWAY: three facility-first spatial variants on /t/$tenantId/spatial?variant=A|B|C.
// Question: tree + canvas, tree table, or a guided setup for importing and binding spatial material?
import { useState, type ReactNode } from 'react';
import { ArrowRight, Box, Building2, Check, CheckCircle2, ChevronDown, ChevronRight, ChevronsDownUp, CircleDot, Crosshair, FileImage, FolderOpen, FolderTree, Layers, Link2, MapPinned, Search, Upload, X } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { DesignPrototypeSwitcher } from '@infrapro/ui/patterns/design-prototype-switcher';
import { PrototypeScene } from './tree-prototype-scene';
import { assetKindLabels, demoAssets, demoBindings, demoNodes, nodeKindLabels, nodePath, nodeStatus, type DemoAsset, type DemoBinding, type DemoNode, type PrototypeVariant } from './tree-prototype-data';
import './tree-prototype.css';

const variants = [{ key: 'A', name: '树与画布' }, { key: 'B', name: '树形管理表' }, { key: 'C', name: '分步配置' }];
const nodeIcons = { facility: Building2, component: Box, point: CircleDot };
const assetIcons = { layout: FileImage, model: Box, geolayer: MapPinned };
type Model = ReturnType<typeof usePrototype>;
function usePrototype() {
  const [selected, setSelected] = useState('beam1');
  const [assets, setAssets] = useState(demoAssets);
  const [bindings, setBindings] = useState(demoBindings);
  const [expanded, setExpanded] = useState(['bridge', 'span1', 'beam1', 'beam2']);
  const [query, setQuery] = useState('');
  const [onlyPending, setOnlyPending] = useState(false);
  const [chosenAsset, setChosenAsset] = useState('bridge-plan');
  const [panel, setPanel] = useState<'library' | 'import' | 'bind' | 'preview'>();
  const [notice, setNotice] = useState('');
  const [step, setStep] = useState(1);
  const node = demoNodes.find(item => item.id === selected)!;
  const facility = demoNodes.find(item => item.id === node.facility)!;
  const facilityAssets = assets.filter(asset => asset.facility === node.facility);
  const asset = facilityAssets.find(item => item.id === chosenAsset) ?? facilityAssets[0];
  const direct = bindings.filter(binding => binding.node === node.id);
  const current = direct.find(binding => binding.asset === asset?.id);
  const inherited = [...nodePath(node)].reverse().slice(1).find(parent => bindings.some(binding => binding.node === parent.id && binding.asset === asset?.id));
  const select = (id: string) => {
    const next = demoNodes.find(item => item.id === id)!;
    setSelected(id); setExpanded(old => [...new Set([...old, ...nodePath(next).map(item => item.id)])]);
    if (node.facility !== next.facility) setChosenAsset(assets.find(item => item.facility === next.facility)?.id ?? '');
  };
  const filtered = demoNodes.filter(item => (!query || `${item.name} ${item.code}`.toLowerCase().includes(query.toLowerCase())) && (!onlyPending || !bindings.some(binding => binding.node === item.id)));
  const allowed = new Set(filtered.flatMap(item => nodePath(item).map(parent => parent.id)));
  const toggle = (id: string) => setExpanded(old => old.includes(id) ? old.filter(item => item !== id) : [...old, id]);
  const reset = () => { setSelected('beam1'); setAssets(demoAssets); setBindings(demoBindings); setExpanded(['bridge', 'span1', 'beam1', 'beam2']); setQuery(''); setOnlyPending(false); setChosenAsset('bridge-plan'); setPanel(undefined); setStep(1); setNotice('演示已重置。'); };
  return { node, facility, assets, setAssets, facilityAssets, asset, chosenAsset, setChosenAsset, bindings, setBindings, selected, select, expanded, setExpanded, toggle, query, setQuery, onlyPending, setOnlyPending, allowed, panel, setPanel, direct, current, inherited, notice, setNotice, step, setStep, reset };
}

export function SpatialTreePrototype({ variant, change }: { variant: PrototypeVariant; change: (variant: PrototypeVariant) => void }) {
  const m = usePrototype();
  return <section className={`spt spt-variant-${variant.toLowerCase()}`} aria-label="工程空间交互原型">
    <header className="spt-page-header"><div><div className="spt-title"><MapPinned size={23} /><h1>工程空间</h1><span className="spt-prototype-tag">交互原型 · 演示数据</span></div><p>按设施组织空间资料，沿着构件和测点逐层建立空间关联。</p></div><div className="spt-header-actions"><Button variant="outline" onClick={() => m.setPanel('library')}><FolderOpen />{m.facility.name}资料<span className="spt-count">{m.facilityAssets.length}</span></Button><Button onClick={() => m.setPanel('import')}><Upload />导入空间资料</Button></div></header>
    {m.notice && <div role="status" className="spt-notice"><CheckCircle2 size={16} />{m.notice}<button type="button" aria-label="关闭操作提示" onClick={() => m.setNotice('')}><X size={16} /></button></div>}
    {variant === 'A' ? <VariantA m={m} /> : variant === 'B' ? <VariantB m={m} /> : <VariantC m={m} />}
    <div className="spt-prototype-footer"><span>演示操作仅保留在当前页面，刷新可还原。</span><details><summary>查看原型状态</summary><pre>{JSON.stringify({ variant, selectedPath: nodePath(m.node).map(item => item.name), selectedAsset: m.asset?.name, panel: m.panel ?? null, step: m.step, query: m.query, onlyPending: m.onlyPending, expanded: m.expanded, assets: m.assets, bindings: m.bindings }, null, 2)}</pre></details></div>
    <DesignPrototypeSwitcher variants={variants} current={variant} change={key => change(key as PrototypeVariant)} reset={m.reset} />
    <ResponsivePanel open={m.panel === 'library'} onOpenChange={open => { if (!open) m.setPanel(undefined); }} title={`${m.facility.name} · 空间资料`} description="资料统一归属于设施，可供多个构件和测点重复关联。" size="wide"><AssetLibrary m={m} /><div className="spt-panel-note">版本说明：示例关系绑定到指定版本。新增版本后，需检查原有构件关联和测点位置。</div></ResponsivePanel>
    <ResponsivePanel open={m.panel === 'preview'} onOpenChange={open => { if (!open) m.setPanel(undefined); }} title={`${m.node.name} · 空间预览`} description={nodePath(m.node).map(item => item.name).join(' / ')} size="workspace"><div className="spt spt-drawer-content"><NodeCanvas m={m} /></div></ResponsivePanel>
    {m.panel === 'import' && <ImportPanel m={m} />}
    {m.panel === 'bind' && <BindPanel key={m.selected} m={m} />}
  </section>;
}

function Status({ node, m }: { node: DemoNode; m: Model }) {
  const status = nodeStatus(node, m.bindings);
  return <span className={`spt-status ${status === '已绑定' ? 'is-bound' : 'is-pending'}`}>{status === '已绑定' ? <Check size={12} /> : <span className="spt-tiny-dot" />}{status}</span>;
}
function TreeFilters({ m, facilityOnly = false }: { m: Model; facilityOnly?: boolean }) {
  return <div className="spt-tree-filters"><label className="spt-search"><Search size={15} /><input aria-label="搜索设施、构件或测点" placeholder="搜索名称或编码…" value={m.query} onChange={event => m.setQuery(event.target.value)} />{m.query && <button type="button" aria-label="清空搜索" onClick={() => m.setQuery('')}><X size={14} /></button>}</label><div className="spt-tree-filter-row"><button type="button" aria-pressed={!m.onlyPending} onClick={() => m.setOnlyPending(false)}>全部对象</button><button type="button" aria-pressed={m.onlyPending} onClick={() => m.setOnlyPending(true)}>待绑定 / 定位<span>{demoNodes.filter(node => (!facilityOnly || node.facility === m.facility.id) && !m.bindings.some(binding => binding.node === node.id)).length}</span></button></div></div>;
}
function Tree({ m, facilityOnly = false }: { m: Model; facilityOnly?: boolean }) {
  const draw = (node: DemoNode, depth: number): ReactNode => {
    if (!m.allowed.has(node.id)) return null;
    const children = demoNodes.filter(item => item.parent === node.id && m.allowed.has(item.id));
    const open = m.expanded.includes(node.id) || !!m.query || m.onlyPending;
    const Icon = nodeIcons[node.kind];
    return <li key={node.id}><div className={`spt-tree-row ${m.selected === node.id ? 'is-selected' : ''}`}><button type="button" className={`spt-tree-toggle ${!children.length ? 'is-empty' : ''}`} tabIndex={children.length ? 0 : -1} aria-label={`${open ? '收起' : '展开'}${node.name}`} aria-expanded={children.length ? open : undefined} onClick={() => m.toggle(node.id)}>{open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}</button><button type="button" className="spt-tree-select" data-spt-node={node.id} aria-pressed={m.selected === node.id} aria-label={`选择${node.name} ${node.code}`} onClick={() => m.select(node.id)}><Icon size={16} /><span className="spt-tree-name">{node.name}<small>{node.kind === 'point' ? node.code : nodeKindLabels[node.kind]}</small></span><span className={`spt-tree-state ${nodeStatus(node, m.bindings) === '已绑定' ? 'is-bound' : ''}`} title={nodeStatus(node, m.bindings)}>{nodeStatus(node, m.bindings) === '已绑定' ? <Check size={12} /> : <span className="spt-tiny-dot" />}</span></button></div>{open && !!children.length && <ul className={depth > 2 ? 'spt-tree-deep' : undefined}>{children.map(child => draw(child, depth+1))}</ul>}</li>;
  };
  return <div className="spt-tree-scroll" data-prototype-tree><ul className="spt-tree" aria-label="设施、构件与测点树">{demoNodes.filter(node => node.kind === 'facility' && (!facilityOnly || node.id === m.facility.id)).map(node => draw(node, 0))}</ul>{!demoNodes.some(node => m.allowed.has(node.id) && (!facilityOnly || node.facility === m.facility.id)) && <div className="spt-empty"><Search /><strong>没有匹配的对象</strong><span>试试名称、编码或清空筛选。</span></div>}</div>;
}
function TreePanel({ m, facilityOnly = false }: { m: Model; facilityOnly?: boolean }) {
  return <aside className="spt-tree-panel"><div className="spt-panel-heading"><h2><FolderTree size={16} />工程资源</h2><Button size="icon" variant="ghost" aria-label="收起全部节点" onClick={() => m.setExpanded([])}><ChevronsDownUp size={15} /></Button></div><TreeFilters m={m} facilityOnly={facilityOnly} /><Tree m={m} facilityOnly={facilityOnly} /><div className="spt-tree-legend"><span><Check size={12} />已绑定</span><span><i className="spt-tiny-dot" />待绑定 / 定位</span></div></aside>;
}
function Breadcrumb({ m }: { m: Model }) {
  return <nav aria-label="当前资源路径" className="spt-breadcrumb">{nodePath(m.node).map((node, i) => <span key={node.id}>{i > 0 && <ChevronRight size={12} />}<button type="button" onClick={() => m.select(node.id)} aria-current={node.id === m.selected ? 'page' : undefined}>{node.name}</button></span>)}</nav>;
}
function ContextHeader({ m }: { m: Model }) {
  const Icon = nodeIcons[m.node.kind];
  return <div className="spt-context-header"><Breadcrumb m={m} /><div className="spt-context-title"><span className="spt-object-icon"><Icon size={21} /></span><div><h2>{m.node.name}<span>{nodeKindLabels[m.node.kind]}</span></h2><p>{m.node.code} · {m.direct.length} 份直接关联资料</p></div><Status node={m.node} m={m} /><Button onClick={() => m.setPanel(m.facilityAssets.length ? 'bind' : 'import')}><Link2 size={15} />{m.node.kind === 'point' ? '绑定 / 标注测点' : '绑定空间资料'}</Button></div></div>;
}
function AssetSelector({ m }: { m: Model }) {
  return <div className="spt-asset-toolbar"><div><Layers size={15} /><label htmlFor="spt-active-asset">当前资料</label><select id="spt-active-asset" value={m.asset?.id ?? ''} onChange={event => m.setChosenAsset(event.target.value)}>{m.facilityAssets.map(asset => <option key={asset.id} value={asset.id}>{asset.name} · {asset.version}</option>)}</select></div><button type="button" onClick={() => m.setPanel('library')}>设施资料库 <ChevronRight size={14} /></button></div>;
}
function NodeCanvas({ m }: { m: Model }) {
  if (!m.asset) return <div className="spt-empty spt-empty-large"><FolderOpen size={38} /><h2>{m.facility.name}还没有空间资料</h2><p>先为设施导入一份布设图、模型或 GIS 图层，再沿目录绑定。</p><Button onClick={() => m.setPanel('import')}><Upload />为此设施导入资料</Button></div>;
  return <><AssetSelector m={m} /><div className={`spt-relation-line ${!m.current ? 'is-inherited' : ''}`}>{m.current ? <><Link2 size={14} /><span>当前对象已绑定</span><strong>{m.current.detail}</strong></> : <><Layers size={14} /><span>{m.inherited ? `当前预览沿用「${m.inherited.name}」的资料` : '当前预览设施资料'}</span><strong>此对象尚未{m.node.kind === 'point' ? '定位' : '绑定'}</strong></>}</div><PrototypeScene key={m.asset.id} asset={m.asset} node={m.node} bindings={m.bindings} choose={id => { m.setQuery(''); m.setOnlyPending(false); m.select(id); requestAnimationFrame(() => document.querySelector<HTMLElement>(`[data-spt-node="${id}"]`)?.scrollIntoView({ block: 'nearest' })); }} /><div className="spt-node-summary"><div><h3>{m.node.kind === 'point' ? '测点空间位置' : '此对象的空间关联'}</h3><p>{m.current ? `${m.asset.name} · ${m.asset.version} · ${m.current.detail}` : '上级资料提供查看上下文，当前对象的绑定需单独确认。'}</p></div><Button variant="outline" onClick={() => m.setPanel('bind')}>{m.current ? '调整关联' : '建立关联'}<ArrowRight size={14} /></Button></div></>;
}
export function VariantA({ m }: { m: Model }) {
  return <div className="spt-workbench spt-layout-a"><TreePanel m={m} /><section className="spt-detail" aria-label="当前对象空间视图"><ContextHeader m={m} /><NodeCanvas m={m} /></section></div>;
}

export function VariantB({ m }: { m: Model }) {
  const visible = demoNodes.filter(node => m.allowed.has(node.id) && (m.query || m.onlyPending || nodePath(node).slice(0,-1).every(parent => m.expanded.includes(parent.id))));
  const bound = demoNodes.filter(node => m.bindings.some(binding => binding.node === node.id)).length;
  return <div className="spt-layout-b"><div className="spt-register-intro"><div><span className="spt-eyebrow">工程空间台账</span><h2>空间关系，沿树一目了然。</h2><p>从设施逐层展开，检查资料归属、构件关联和测点位置。</p></div><div className="spt-register-count"><strong>{bound}<small> / {demoNodes.length}</small></strong><span>对象已建立空间关联</span></div></div><div className="spt-register"><div className="spt-register-toolbar"><TreeFilters m={m} /><Button variant="outline" onClick={() => m.setExpanded(m.expanded.length === demoNodes.length ? [] : demoNodes.map(node => node.id))}><FolderTree />{m.expanded.length === demoNodes.length ? '收起层级' : '展开全部'}</Button></div><div className="spt-register-scroll"><table><thead><tr><th>设施 / 构件 / 测点</th><th>关联资料</th><th>空间位置</th><th>状态</th><th>操作</th></tr></thead><tbody>{visible.map(node => { const relations = m.bindings.filter(binding => binding.node === node.id); const first = relations[0]; const file = m.assets.find(asset => asset.id === first?.asset); const children = demoNodes.some(item => item.parent === node.id); const Icon = nodeIcons[node.kind]; return <tr key={node.id} className={`${node.id === m.selected ? 'is-selected' : ''} ${node.kind === 'facility' ? 'is-facility' : ''}`}><td><div className="spt-table-name" style={{ paddingLeft: (nodePath(node).length-1)*20 }}><button type="button" className={!children ? 'is-empty' : ''} tabIndex={children ? 0 : -1} aria-label={`${m.expanded.includes(node.id) ? '收起' : '展开'}${node.name}`} onClick={() => m.toggle(node.id)}>{m.expanded.includes(node.id) ? <ChevronDown size={14} /> : <ChevronRight size={14} />}</button><Icon size={16} /><button type="button" onClick={() => m.select(node.id)}>{node.name}<small>{node.code}</small></button></div></td><td>{file ? <>{file.name}{relations.length > 1 && <span className="spt-count">+{relations.length-1}</span>}</> : <span className="spt-muted">{nodeStatus(node, m.bindings) === '待定位' ? '可沿用上级资料' : '尚未关联'}</span>}</td><td className="spt-muted">{first?.detail ?? '—'}</td><td><Status node={node} m={m} /></td><td><div className="spt-table-actions"><button type="button" onClick={() => { m.select(node.id); m.setPanel('preview'); }}>查看</button><button type="button" onClick={() => { m.select(node.id); m.setPanel('bind'); }}>{relations.length ? '调整' : '绑定'}</button></div></td></tr>; })}</tbody></table>{!visible.length && <div className="spt-empty">没有符合筛选条件的对象。</div>}</div><footer className="spt-register-footer"><span>显示 {visible.length} 个对象 · 含搜索结果的上级路径</span><span>资料可复用，空间位置独立维护</span></footer></div></div>;
}

export function VariantC({ m }: { m: Model }) {
  const nodes = demoNodes.filter(node => node.facility === m.facility.id);
  const pending = nodes.filter(node => !m.bindings.some(binding => binding.node === node.id));
  return <div className="spt-layout-c"><aside className="spt-facility-rail"><span className="spt-eyebrow">选择设施</span>{demoNodes.filter(node => node.kind === 'facility').map(node => { const count = m.assets.filter(asset => asset.facility === node.id).length; return <button type="button" key={node.id} aria-pressed={m.facility.id === node.id} onClick={() => { m.select(node.id); m.setStep(count ? 1 : 0); }}><Building2 size={19} /><span>{node.name}<small>{count ? `${count} 份空间资料` : '尚未导入资料'}</small></span><ChevronRight size={15} /></button>; })}<p>每次聚焦一个设施，完成从资料导入到空间定位。</p></aside><section className="spt-guide" aria-label="设施空间配置"><div className="spt-guide-heading"><div><span className="spt-eyebrow">设施空间配置</span><h2>{m.facility.name}</h2></div><span>{nodes.length-pending.length} / {nodes.length} 个对象已绑定</span></div><nav className="spt-stepper" aria-label="设施配置步骤">{['导入设施资料', '逐层绑定对象', '检查空间关系'].map((title, index) => <button type="button" key={title} aria-current={m.step === index ? 'step' : undefined} onClick={() => m.setStep(index)}><span>{index+1}</span><strong>{title}</strong>{index < 2 && <ChevronRight size={16} />}</button>)}</nav>{m.step === 0 ? <div className="spt-guide-library"><h3>先把空间资料放进这个设施</h3><p>一份图纸或模型只需导入一次，后续可绑定到多个节点。</p><AssetLibrary m={m} /><Button onClick={() => m.setStep(1)} disabled={!m.facilityAssets.length}>下一步：选择构件和测点<ArrowRight /></Button></div> : m.step === 1 ? <div className="spt-guide-binding"><div><TreeFilters m={m} facilityOnly /><Tree m={m} facilityOnly /></div><div className="spt-guide-object"><ContextHeader m={m} /><NodeCanvas m={m} /></div></div> : <div className="spt-check-results"><div className="spt-check-summary"><CheckCircle2 size={27} /><div><h3>{pending.length ? `还有 ${pending.length} 个对象待绑定 / 定位` : '此设施的演示对象均已绑定'}</h3><p>已绑定表示已建立至少一条空间关系；是否满足业务要求仍需人工检查。</p></div></div>{pending.map(node => <button className="spt-check-item" type="button" key={node.id} onClick={() => { m.select(node.id); m.setStep(1); m.setPanel('bind'); }}><span><strong>{node.name}</strong><small>{nodePath(node).map(item => item.name).join(' / ')}</small></span><span>去绑定<ArrowRight size={14} /></span></button>)}<div className="spt-panel-note">图纸或模型更新版本后，需要重新检查这些绑定的位置。设施直属测点与多层子构件都保留在真实层级中。</div></div>}</section></div>;
}

function AssetLibrary({ m }: { m: Model }) {
  return <div className="spt-library"><div className="spt-library-heading"><span>{m.facilityAssets.length} 份资料 · {m.facility.name}</span><Button size="sm" variant="outline" onClick={() => m.setPanel('import')}><Upload />导入资料</Button></div>{m.facilityAssets.map(asset => { const Icon = assetIcons[asset.kind]; const count = m.bindings.filter(binding => binding.asset === asset.id).length; return <button type="button" className="spt-library-file" key={asset.id} onClick={() => { m.setChosenAsset(asset.id); m.setPanel('preview'); }}><span className="spt-file-icon"><Icon size={24} /></span><span><strong>{asset.name}</strong><small>{assetKindLabels[asset.kind]} · {asset.version} · {asset.pages} {asset.kind === 'layout' ? '页' : '个视图'}</small></span><span>{count} 个关联</span><ChevronRight size={16} /></button>; })}{!m.facilityAssets.length && <div className="spt-empty"><FolderOpen /><strong>设施资料库为空</strong><span>先导入图纸或模型，再沿树逐层绑定。</span></div>}</div>;
}
function ImportPanel({ m }: { m: Model }) {
  const [name, setName] = useState(`${m.facility.name}监测布设补充图`);
  const [kind, setKind] = useState<DemoAsset['kind']>('layout');
  return <ResponsivePanel open onOpenChange={open => { if (!open) m.setPanel(undefined); }} title="导入设施空间资料" description="先确定资料归属设施，导入后再按需要绑定到构件和测点。" footer={<div className="spt-panel-footer"><span>本次为模拟导入</span><Button disabled={!name.trim()} onClick={() => { const asset: DemoAsset = { id: `demo-${Date.now()}`, facility: m.facility.id, name: name.trim(), kind, version: 'V1', pages: 1 }; m.setAssets(old => [...old, asset]); m.setChosenAsset(asset.id); m.setPanel('library'); m.setNotice(`已模拟导入「${asset.name}」至${m.facility.name}，可继续绑定对象。`); }}>模拟导入<ArrowRight /></Button></div>}><div className="spt-form"><label>归属设施<div className="spt-readonly"><Building2 size={18} />{m.facility.name}<span>由当前节点确定</span></div></label><label>资料类型<select value={kind} onChange={event => setKind(event.target.value as DemoAsset['kind'])}>{Object.entries(assetKindLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label><label>资料名称<input value={name} onChange={event => setName(event.target.value)} /></label><div className="spt-upload-example"><Upload size={30} /><strong>示例资料已就绪</strong><p>原型使用图纸示意，不上传实际文件。</p><span>{kind === 'layout' ? 'PDF / PNG / JPG' : kind === 'model' ? 'IFC / GLB / glTF' : 'GeoJSON'}</span></div><div className="spt-panel-note">导入完成后，资料属于整个设施。选择左侧任意构件或测点，即可复用这份资料建立空间关联。</div></div></ResponsivePanel>;
}
function BindPanel({ m }: { m: Model }) {
  const [assetID, setAssetID] = useState(m.asset?.id ?? '');
  const [anchor, setAnchor] = useState<[number, number]>();
  const asset = m.facilityAssets.find(item => item.id === assetID);
  const isFacility = m.node.kind === 'facility';
  const existing = m.bindings.find(binding => binding.node === m.node.id && binding.asset === assetID);
  const ready = !!asset && (isFacility || !!anchor);
  const action = m.node.kind === 'point' ? '测点位置' : asset?.kind === 'model' ? '模型构件' : '构件区域';
  return <ResponsivePanel open onOpenChange={open => { if (!open) m.setPanel(undefined); }} title={`绑定空间资料 · ${m.node.name}`} description={nodePath(m.node).map(node => node.name).join(' / ')} size="workspace" footer={<div className="spt-panel-footer"><div><strong>{m.node.name} → {asset?.name ?? '尚未选择资料'}</strong><small>{isFacility ? '关联整份资料，供下级对象查看和定位' : anchor ? `已选择${action} · X ${anchor[0]}% / Y ${anchor[1]}%` : `请在预览中选择${action}`}</small></div><Button disabled={!ready} onClick={() => { if (!asset) return; const binding: DemoBinding = { node: m.node.id, asset: asset.id, anchor: anchor ?? [50,50], detail: isFacility ? '整份资料' : asset.kind === 'layout' ? `第 1 页 · ${action}` : action }; m.setBindings(old => [...old.filter(item => !(item.node === binding.node && item.asset === binding.asset)), binding]); m.setChosenAsset(asset.id); m.setPanel(undefined); m.setNotice(`已在原型中${existing ? '更新' : '建立'}「${m.node.name}」与「${asset.name}」的关联。`); }}><Check />确认{existing ? '调整' : '绑定'}</Button></div>}><div className="spt-bind-layout"><aside><div className="spt-mini-step"><span>1</span><h3>选择设施资料</h3></div><p className="spt-muted">{m.facility.name} · {m.facilityAssets.length} 份资料</p>{m.facilityAssets.map(item => { const Icon = assetIcons[item.kind]; return <button type="button" className="spt-bind-asset" aria-pressed={assetID === item.id} key={item.id} onClick={() => { setAssetID(item.id); setAnchor(undefined); }}><Icon size={18} /><span><strong>{item.name}</strong><small>{assetKindLabels[item.kind]} · {item.version}</small></span>{assetID === item.id && <Check size={16} />}</button>; })}{!m.facilityAssets.length && <Button onClick={() => m.setPanel('import')}>先导入设施资料</Button>}<div className="spt-panel-note">选择本设施的既有资料即可，无需为每个节点重复上传。</div></aside><section><div className="spt-mini-step"><span>2</span><h3>{isFacility ? '核对设施与资料' : `指定${action}`}</h3></div><p className="spt-bind-hint">{isFacility ? '设施关联整份资料；构件和测点的具体空间位置在后续逐层设置。' : `点击画布指定${action}，再点击下方“确认绑定”。`}</p>{asset && <PrototypeScene key={asset.id} asset={asset} node={m.node} bindings={m.bindings} placing={!isFacility} anchor={anchor} place={setAnchor} />}{!isFacility && asset && <Button variant="outline" className="spt-use-center" onClick={() => setAnchor([50, 50])}><Crosshair size={15} />使用画布中心作为示例位置</Button>}</section></div></ResponsivePanel>;
}
