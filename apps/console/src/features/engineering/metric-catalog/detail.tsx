import { useRef, useState } from 'react';
import { ArrowLeft, BookOpen, ChevronRight, ClipboardList, FlaskConical, History } from 'lucide-react';
import type { EngineeringDefinition, EngineeringMetric } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { UnsavedChanges } from '../../../app/unsaved-changes';
import { versionState } from '../facility-type-detail/facts';
import { useMaintenance, type Rights } from '../facility-type-detail/maintenance';
import { DraftActions, VersionManager, type ManagerScreen } from '../facility-type-detail/version-manager';
import { unitLabel, type UnitChoice } from '../metric-units';
import { PreviewPanel } from '../preview-panel';
import { metricFacts, metricScope, metricShapes } from './facts';
import { MetricRuleEditor } from './rule-editor';
import { useMetricUnitChoices } from './use-metric-unit-choices';
import '../facility-type-detail/detail.css';
import '../facility-type-detail/workspace.css';
import './metric-catalog.css';

function RuleFacts({ metric, known }: { metric: EngineeringMetric; known: Map<string, UnitChoice> }) {
  const describe = (id: string) => { const unit = known.get(id); return unit ? unitLabel(unit) : id ? '未能读取的固定单位版本' : '尚未配置'; };
  const { minimum, maximum } = metric;
  const bounds = minimum == null ? maximum == null ? '未设上下限' : '≤ ' + maximum : maximum == null ? '≥ ' + minimum : minimum + ' 至 ' + maximum + '（含边界）';
  return <dl className="met-facts"><div><dt>取值形态</dt><dd>{metricShapes[metric.shape]}</dd></div>
    {metric.shape === 'enum' ? <><div><dt>物理单位</dt><dd>无</dd></div><div><dt>允许选项</dt><dd>{metric.options?.length ? <ul>{metric.options.map(option => <li key={option.key}>{option.name} <code>{option.key}</code></li>)}</ul> : '尚未配置'}</dd></div></> :
      <><div><dt>数值范围</dt><dd>{bounds}</dd></div><div><dt>标准单位</dt><dd>{describe(metric.standard_unit)}</dd></div>
        <div><dt>允许单位</dt><dd>{metric.allowed_units?.length ? <ul className="met-unit-chips">{metric.allowed_units.map(id => <li key={id}>{describe(id)}</li>)}</ul> : '尚未配置'}</dd></div></>}
  </dl>;
}

type Props = {
  definition: EngineeringDefinition; back: () => void; rights: Rights; initialPanel?: 'structure' | 'versions';
  refresh: () => Promise<EngineeringDefinition>; copied: (id: string) => void; removed: () => void;
};

export default function MetricDetail({ definition, back, rights, initialPanel, refresh, copied, removed }: Props) {
  const model = useMaintenance({ definition, rights, refresh, copied, removed, scope: metricScope });
  const { draft, version, isDraft, message, pending } = model;
  const units = useMetricUnitChoices(model.known);
  const content = isDraft ? draft?.work : version?.content;
  const metric = content?.metric;
  const facts = content && metricFacts(content);
  const [panel, setPanel] = useState<'sample' | 'versions' | null>(initialPanel === 'versions' || (isDraft && rights.editable) ? 'versions' : null);
  const [screen, show] = useState<ManagerScreen>('overview');
  const trigger = useRef<HTMLElement | null>(null);
  const inspect = (next: 'sample' | 'versions') => {
    trigger.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    show('overview'); setPanel(next);
  };
  const draftActions = panel === 'versions' && isDraft && ['overview', 'basic', 'metric'].includes(screen);
  const name = content?.name || definition.code;
  const revision = version ? 'V' + version.number : '草稿';
  const rules = metric && <RuleFacts metric={metric} known={units.known} />;
  const footer = <div className="ft-panel ft-feedback-footer">
    {model.busy && <p role="status">正在处理…</p>}
    {message && <div role={message.error ? 'alert' : 'status'} className={message.error ? 'ft-operation-error' : 'ft-hint'}>{message.text}
      {!!message.issues?.length && <ul>{message.issues.map((issue, i) => <li key={i}>{issue.message}{issue.key && ' · ' + issue.key}</li>)}</ul>}</div>}
    {pending && !model.busy && <Button variant="outline" onClick={async () => { if (await model.reconcile()) show('overview'); }}>核对操作结果</Button>}
    {model.conflict && !pending && screen !== 'reload' && <Button variant="outline" onClick={() => show('reload')}>服务端草稿已有变更，重新读取</Button>}
    {draftActions && <fieldset disabled={model.busy || !!pending}><DraftActions model={model} rights={rights} atOverview={screen === 'overview'} onOverview={() => show('overview')} onPublish={() => show('publish')} /></fieldset>}
  </div>;
  return <section className="met-page"><UnsavedChanges dirty={model.dirty} />
    <div className="ft-path"><button id="met-back" type="button" aria-label="返回目录" onClick={back}><ArrowLeft />指标目录</button><ChevronRight /><span>指标详情</span></div>
    <header className="met-heading"><div><div className="met-title"><h1>{name}</h1><span className="ft-version" data-usable={!!version?.usable}>{revision} · {isDraft ? model.dirty ? '待保存' : '待发布' : versionState(version, true)}</span></div>
      {facts?.intro && <p>{facts.intro}</p>}<code>{definition.code}</code></div><Button variant="outline" onClick={() => inspect('versions')}><History />版本与管理</Button></header>
    {metric && facts ? <><div className="met-sheet"><section><p className="met-eyebrow"><ClipboardList />规格表</p><h2>记录规格</h2>{rules}</section>
      <aside><p className="met-eyebrow"><BookOpen />使用说明</p><h3>在哪里使用</h3><dl><div><dt>适用设施</dt><dd>{facts.topics.join('、') || '未归类'}</dd></div>
        {facts.category && <div><dt>业务类别</dt><dd>{facts.category}</dd></div>}</dl>
        <p className="met-note">{metric.shape === 'enum' ? '每条记录保存一个选项编码。' : '每条通道从允许单位中固定一个。观测、导入和查询沿用通道单位，保留原始数值。'}</p>
        <Button variant="outline" onClick={() => inspect('sample')}><FlaskConical />试填样例</Button></aside></div>
      {content?.description && <details className="met-description"><summary>查看完整定义说明</summary><p>{content.description}</p></details>}</> : <p>此定义尚无可查看的取值规则。</p>}
    <ResponsivePanel open={!!panel} size={panel === 'versions' ? 'wide' : 'default'} returnFocus={trigger} fallbackFocusSelector="#met-back"
      onOpenChange={open => { if (!open && !model.busy) setPanel(null); }} title={panel === 'sample' ? '试填样例' : '版本与管理'}
      description={name + ' · ' + (panel === 'sample' ? revision : model.releases.length + ' 个已发布版本')}
      footer={panel === 'versions' && (draftActions || message || pending || model.busy || model.conflict) ? footer : undefined}>
      {panel === 'sample' && content && <PreviewPanel key={model.selected} scope={metricScope} definition={definition} content={content} version={version} dirty={isDraft && model.dirty} />}
      {panel === 'versions' && <fieldset className="ft-panel" disabled={model.busy || !!pending}><VersionManager key={model.selected} model={model} rights={rights} screen={screen} show={show}
        openDraft={() => { if (draft) { model.select('draft'); show('overview'); } else show('create'); }}
        draftCreated={() => show('overview')} metricRules={isDraft && rights.editable && content ? <MetricRuleEditor content={content} change={model.edit} {...units} /> : rules} /></fieldset>}
    </ResponsivePanel></section>;
}
