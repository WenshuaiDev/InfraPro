import { useRef, useState } from 'react';
import { ArrowLeft, ChevronRight, History, PencilLine } from 'lucide-react';
import type { EngineeringDefinition } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { UnsavedChanges } from '../../../app/unsaved-changes';
import { FieldDetails } from './details';
import { fieldSummary, versionState, type DetailPanel } from './facts';
import { FieldWorkspace } from './field-workspace';
import { useMaintenance, type Rights } from './maintenance';
import { RuleExplorer } from './rules';
import { DraftActions, VersionManager, type ManagerScreen } from './version-manager';
import { StructureDiagram } from './structure-diagram';
import './detail.css';
import './workspace.css';

const titles = { fields: '设施建档资料', structure: '工程结构规则', versions: '版本与管理' };
type Props = {
  definition: EngineeringDefinition; back: () => void; rights: Rights; initialPanel?: 'structure' | 'versions';
  refresh: () => Promise<EngineeringDefinition>; copied: (id: string) => void; removed: () => void;
};
export default function FacilityTypeDetail({ definition, back, rights, initialPanel, refresh, copied, removed }: Props) {
  const model = useMaintenance({ definition, rights, refresh, copied, removed });
  const { draft, version, isDraft, message, pending } = model;
  const [panel, setPanel] = useState<DetailPanel | null>(initialPanel ?? null);
  const [screen, show] = useState<ManagerScreen>('overview');
  const [target, setTarget] = useState('');
  const [editTarget, setEditTarget] = useState(false);
  const [fromVersion, setFromVersion] = useState(false);
  const [draftTarget, setDraftTarget] = useState<DetailPanel>('versions');
  const trigger = useRef<HTMLElement | null>(null);
  const content = isDraft ? draft?.work : version?.content;
  const fields = content?.fields ?? [];
  function inspect(next: DetailPanel, place = '') {
    trigger.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setEditTarget(false); setFromVersion(false); setTarget(place); show('overview'); setPanel(next);
  }
  function manage(next: ManagerScreen = 'overview') { setFromVersion(false); show(next); setPanel('versions'); }
  function openFromVersion(next: DetailPanel) { setEditTarget(false); setTarget(''); setFromVersion(true); setPanel(next); }
  function openDraft() {
    if (draft) { model.select('draft'); setEditTarget(true); show('overview'); return; }
    setDraftTarget(panel ?? 'versions'); setPanel('versions'); show('create');
  }
  const description = content?.description?.trim() ?? '';
  const intro = description.match(/^.+?[。！？](?:\s|$)?/u)?.[0]?.trim() ?? description;
  const context = (content?.name || definition.code) + ' · ' + (isDraft ? '草稿' : version ? 'V' + version.number : '尚无版本');
  const draftActions = isDraft && (panel !== 'versions' || ['overview', 'fields', 'basic'].includes(screen));
  const contextualEdit = version && rights.editable && (panel === 'structure' || panel === 'fields');
  const footer = <div className="ft-panel ft-feedback-footer">
    {model.busy && <p role="status" className="ft-hint">正在处理，请稍候…</p>}
    {message && <div role={message.error ? 'alert' : 'status'} className={message.error ? 'ft-operation-error' : 'ft-hint'}>
      <p>{message.text}</p>{!!message.issues?.length && <ul>{message.issues.map((issue, i) => <li key={i}>{issue.message}{issue.key && ' · ' + issue.key}</li>)}</ul>}
    </div>}
    {pending && !model.busy && <Button variant="outline" onClick={async () => { if (await model.reconcile()) manage(); }}>
      {pending.result ? '重新读取操作结果' : '核对操作回执'}</Button>}
    {model.conflict && !pending && screen !== 'reload' && <div className="ft-actions"><p className="ft-hint">服务端草稿已有变更，本页的输入已保留。</p><Button variant="outline" onClick={() => manage('reload')}>重新读取</Button></div>}
    <fieldset disabled={model.busy || !!pending} className="ft-footer">{draftActions ?
      <DraftActions model={model} rights={rights} atOverview={panel === 'versions' && screen === 'overview'} onOverview={() => manage()} onPublish={() => manage('publish')} /> :
      <div className="ft-actions">{fromVersion && <Button variant="outline" onClick={() => manage()}><ArrowLeft />返回版本</Button>}
        {contextualEdit && <Button onClick={openDraft}><PencilLine />{draft ? '打开已有草稿' : panel === 'structure' ? '建草稿编辑规则' : '建草稿编辑字段'}</Button>}</div>}</fieldset>
  </div>;
  const hasFooter = draftActions || contextualEdit || fromVersion || message || model.busy || pending || model.conflict;
  return <section className="ft-detail-page">
    <UnsavedChanges dirty={model.dirty} />
    <div className="ft-path"><button id="ft-back" aria-label="返回目录" type="button" onClick={back}><ArrowLeft aria-hidden="true" />设施类型目录</button>
      <ChevronRight aria-hidden="true" /><span>类型详情</span></div>
    <header className="ft-header"><div className="min-w-0"><div className="ft-title"><h1>{content?.name || definition.code}</h1>
      {!!content && <span className="ft-version" data-usable={!!version?.usable}>{version && 'V' + version.number + ' · '}{isDraft && model.dirty ? '草稿 · 待保存' : versionState(version)}</span>}</div>
      {intro && <p>{intro}</p>}</div><Button variant="outline" onClick={() => inspect('versions')}><History aria-hidden="true" />版本与管理</Button></header>
    {content ? <StructureDiagram key={model.selected} weather={definition.code === 'facility_weather_station'} content={content} inspect={inspect} /> : <p>此类型暂无草稿或已发布版本。</p>}
    <ResponsivePanel open={panel !== null} onOpenChange={(open) => { if (!open && !model.busy) setPanel(null); }}
      size={panel === 'fields' && !isDraft ? 'default' : 'wide'} returnFocus={trigger} fallbackFocusSelector="#ft-back"
      title={panel ? titles[panel] : '类型详情'} description={panel === 'versions' ? (content?.name || definition.code) + ' · ' + model.releases.length + ' 个已发布版本' : context + (panel === 'fields' ? ' · ' + fieldSummary(fields) : '')} footer={hasFooter ? footer : undefined}>
      <fieldset className="ft-panel" disabled={model.busy || !!pending} aria-busy={model.busy}>
        {panel === 'fields' && (isDraft && draft && rights.editable ? <FieldWorkspace fields={fields} change={(next) => model.edit({ ...draft.work, fields: next })} units={model.known} remember={model.remember} /> :
          <FieldDetails fields={fields} units={model.known} />)}
        {panel === 'structure' && content && <RuleExplorer key={model.selected} content={content} units={model.known} remember={model.remember} target={target} setTarget={setTarget} startEditing={editTarget} edit={isDraft && rights.editable ? model.edit : undefined} />}
        {panel === 'versions' && <VersionManager key={model.selected} model={model} rights={rights} screen={screen} show={show} openDraft={openDraft}
          draftCreated={() => { setPanel(draftTarget); show('overview'); setEditTarget(draftTarget === 'structure'); }}
          structure={() => openFromVersion('structure')} fields={() => openFromVersion('fields')} />}
      </fieldset>
    </ResponsivePanel>
  </section>;
}
