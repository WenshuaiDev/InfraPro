import { useState, type ReactNode } from 'react';
import { ArrowLeft, ArrowRight, BookOpen, Check, ChevronDown, ChevronRight, CircleCheck, ClipboardList, Copy, FilePenLine, Files, GitCompareArrows, History, Layers, ListTree, MoreHorizontal, Power, RefreshCw, ShieldCheck, Tag, Trash2 } from 'lucide-react';
import { Permissions } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { TextControl } from '../controls';
import { PreviewPanel } from '../preview-panel';
import { stamp, versionState } from './facts';
import { FieldWorkspace } from './field-workspace';
import { type Maintenance, type Rights } from './maintenance';
import { Availability, OperationRecords, VersionCompare } from './version-records';

export type ManagerScreen = 'overview' | 'basic' | 'metric' | 'fields' | 'compare' | 'audit' | 'create' | 'publish' | 'availability' | 'copy' | 'discard' | 'check' | 'sample' | 'reload';
const subTitles: Record<Exclude<ManagerScreen, 'overview'>, string> = {
  metric: '取值规则与允许单位', basic: '编辑基本资料', fields: '编辑设施建档字段', compare: '版本对比', audit: '操作记录', create: '从此版本建立草稿',
  publish: '确认发布草稿', availability: '更改使用状态', copy: '复制为独立定义', discard: '删除当前草稿', check: '版本校验', sample: '试验建档样例', reload: '重新读取定义',
};
type Props = {
  model: Maintenance; rights: Rights; screen: ManagerScreen; show: (s: ManagerScreen) => void;
  structure?: () => void; fields?: () => void; metricRules?: ReactNode; openDraft: () => void; draftCreated: () => void;
};
export function VersionManager({ model, rights, screen, show, structure, fields, metricRules, openDraft, draftCreated }: Props) {
  const { definition, releases, selected, isDraft, version, draft } = model;
  const isMetric = model.scope.kind === 'metrics';
  const noun = isMetric ? '指标' : '类型';
  const sampleLabel = isMetric ? '试填指标样例' : '试验建档样例';
  const { can } = useIdentity();
  const canAudit = can(Permissions.PlatformAuditRead);
  const content = isDraft ? draft?.work : version?.content;
  const base = releases.find((r) => r.id === draft?.base_version);
  const [more, setMore] = useState(false);
  const [copyCode, setCopyCode] = useState(''), [copyReason, setCopyReason] = useState('');
  const goto = (s: ManagerScreen) => { setMore(false); show(s); };
  const select = (id: string) => { model.select(id); goto('overview'); };
  const otherVersion = releases.length + Number(!!draft) > 1;
  const label = isDraft ? '当前草稿' : version ? '已发布版本 V' + version.number : '暂无版本';
  const selectedStatus = isDraft ? model.dirty ? '有待保存的修改' : model.validation?.valid ? '校验通过' : '已保存' : versionState(version, isMetric);
  const overview = () => goto('overview');
  const reasonField = <TextControl label="本次变更原因" value={model.reason} onChange={model.setReason} />;
  const source = definition.source ? definition.source.name + ' V' + definition.source.number : definition.source_version;

  return <div className={'ft-manager' + (otherVersion ? '' : ' ft-single-version')}><div className="ft-rule-layout ft-manager-layout">
    {otherVersion && <><nav className="ft-rail ft-history-rail" aria-label="版本时间线"><h3>版本时间线</h3>
      <div className="ft-version-list">
        {draft && <button type="button" className="ft-version-item" aria-pressed={isDraft} onClick={() => select('draft')}>
          <span className="ft-rail-symbol" data-draft><FilePenLine /></span><span><strong>当前草稿</strong><small>{model.dirty ? '待保存' : '修订 ' + draft.revision}</small></span></button>}
        {releases.map((v) => <button type="button" className="ft-version-item" aria-pressed={selected === v.id} key={v.id} onClick={() => select(v.id)}>
          <span className="ft-rail-symbol"><Tag /></span><span><strong>V{v.number}</strong><small>{stamp(v.published_at)}</small></span></button>)}
      </div></nav>
      <label className="ft-mobile-picker ft-label">查看版本<select className="ft-select" value={selected} onChange={(e) => select(e.target.value)}>
        {draft && <option value="draft">当前草稿</option>}{releases.map((r) => <option key={r.id} value={r.id}>V{r.number} · {versionState(r, isMetric)}</option>)}</select></label></>}
    <article className="ft-detail-body ft-version-body" aria-label="所选版本">
      {screen !== 'overview' && <div className="ft-editor-heading"><button type="button" className="ft-text-action" onClick={overview}><ArrowLeft />{label}</button><h3>{screen === 'sample' ? sampleLabel : subTitles[screen]}</h3></div>}
      {screen === 'overview' && <>
        <div className={'ft-release-hero' + (isDraft ? ' ft-is-draft' : '')}>
          <div className="ft-release-mark">{isDraft ? <FilePenLine /> : <Tag />}</div>
          <div className="ft-release-title"><p>{content?.name || noun}</p><h3>{label}</h3>
            {content && <span className="ft-status" data-tone={isDraft ? 'amber' : version?.usable ? 'green' : 'neutral'}><i />{selectedStatus}</span>}</div>
          <div className="ft-hero-bottom"><span>{isDraft && draft ? (base ? '基于 V' + base.number : '首次编制') + ' · 修订 ' + draft.revision : version ? stamp(version.published_at, true) + ' 发布' : '此编号尚无已发布内容'}</span>
            {version && (draft || rights.editable) && <Button onClick={openDraft}><FilePenLine />{draft ? (rights.editable ? '继续草稿' : '查看草稿') : '建立草稿'}</Button>}</div>
        </div>
        {version?.state_reason && !version.enabled && <p className="ft-hint">停用原因 · {version.state_reason}</p>}
        {content && <section><div className="ft-inline-head"><h4>{isDraft && rights.editable ? '编辑草稿' : '版本内容'}</h4></div>
          <div className="ft-tasks">
            {isDraft && rights.editable && <button type="button" onClick={() => goto('basic')}><BookOpen /><span><strong>基本资料</strong><small>{noun}名称、说明和发布说明</small></span><ChevronRight /></button>}
            {isMetric ? <button type="button" onClick={() => goto('metric')}><ClipboardList /><span><strong>取值规则与允许单位</strong><small>{content.metric?.shape === 'enum' ? (content.metric.options?.length ?? 0) + ' 个枚举选项' : (content.metric?.allowed_units?.length ?? 0) + ' 个关联单位'}</small></span><ChevronRight /></button> : <>
            <button type="button" onClick={() => isDraft && rights.editable ? goto('fields') : fields?.()}><ClipboardList /><span><strong>设施建档字段</strong><small>{content.fields?.length ?? 0} 项资料</small></span><ChevronRight /></button>
            <button type="button" onClick={structure}><ListTree /><span><strong>工程结构规则</strong><small>{content.components?.length ?? 0} 类构件 · {content.relations?.length ?? 0} 条归属规则</small></span><ChevronRight /></button></>}
          </div></section>}
        {content && <details className="ft-options"><summary><Layers />发布说明与定义信息<ChevronDown /></summary><dl className="ft-facts">
          <div><dt>发布说明</dt><dd>{content.release_notes || '尚未填写'}</dd></div><div><dt>{noun}编号</dt><dd>{definition.code}</dd></div>
          {content.description && <div><dt>完整说明</dt><dd>{content.description}</dd></div>}
          {source && <div><dt>复制来源</dt><dd>{source}</dd></div>}
          {!!version?.dependencies.length && <div><dt>引用版本</dt><dd>{version.dependencies.map((id) => { const v = model.known.find((k) => k.id === id); return v ? v.content.name + ' V' + v.number : id; }).join('、')}</dd></div>}
        </dl></details>}
        <div className="ft-management-links">
          {canAudit && <button type="button" onClick={() => goto('audit')}><History />操作记录</button>}
          {otherVersion && <button type="button" onClick={() => goto('compare')}><GitCompareArrows />版本对比</button>}
          <button type="button" aria-expanded={more} onClick={() => setMore(!more)}><MoreHorizontal />更多操作</button>
        </div>
        {more && <div className="ft-more-actions">
          {content && <Button variant="ghost" onClick={() => goto('sample')}><ClipboardList />{sampleLabel}</Button>}
          {version && <Button variant="ghost" onClick={() => { goto('check'); void model.validate(); }}><ShieldCheck />校验版本</Button>}
          {version && rights.copyable && <Button variant="ghost" disabled={!version.usable} onClick={() => goto('copy')}><Copy />复制为独立定义</Button>}
          {version && rights.stateable && <Button variant="ghost" onClick={() => goto('availability')}><Power />{version.enabled ? '停用版本' : '恢复版本'}</Button>}
          {isDraft && rights.editable && <Button variant="ghost" onClick={() => goto('discard')}><Trash2 />删除草稿</Button>}
          <Button variant="ghost" onClick={() => goto('reload')}><RefreshCw />重新读取定义</Button>
        </div>}
      </>}
      {screen === 'basic' && isDraft && draft && rights.editable && <div className="ft-edit-form">
        <TextControl label={noun + '名称'} value={draft.work.name} onChange={(name) => model.edit({ ...draft.work, name })} />
        <TextControl label={noun + '说明'} multiline value={draft.work.description} onChange={(description) => model.edit({ ...draft.work, description })} />
        <TextControl label="发布说明" multiline value={draft.work.release_notes} onChange={(release_notes) => model.edit({ ...draft.work, release_notes })} /></div>}
      {screen === 'metric' && metricRules}
      {screen === 'fields' && isDraft && draft && rights.editable && <FieldWorkspace fields={draft.work.fields ?? []} units={model.known} remember={model.remember} change={(fields) => model.edit({ ...draft.work, fields })} />}
      {screen === 'sample' && content && <PreviewPanel key={selected} scope={model.scope} definition={definition} content={content} version={version} dirty={isDraft && model.dirty} />}
      {screen === 'compare' && <VersionCompare model={model} />}
      {screen === 'audit' && canAudit && <OperationRecords model={model} />}
      {screen === 'availability' && rights.stateable && <Availability model={model} done={overview} />}
      {screen === 'check' && version && <p className="ft-muted">校验 V{version.number} 的{isMetric ? '取值规则与关联单位' : '字段定义、结构规则和引用依赖'}。</p>}
      {screen === 'create' && version && rights.editable && <div className="ft-edit-form"><p className="ft-muted">以 V{version.number} 为起点，建立一份可编辑的草稿。</p>
        {reasonField}<Button disabled={!model.reason.trim()} onClick={async () => {
          if (await model.write({ action: 'draft', revision: definition.revision, version_id: version.id })) draftCreated();
        }}>确认建立草稿</Button></div>}
      {screen === 'copy' && rights.copyable && version && <div className="ft-edit-form"><p className="ft-muted">从 V{version.number} 建立独立定义，副本有自己的编号和版本记录。</p>
        <TextControl label="副本编号" value={copyCode} onChange={setCopyCode} hint="小写字母开头，可用数字和下划线，最多 64 位。" />
        <TextControl label="复制原因" value={copyReason} onChange={setCopyReason} />
        {model.dirty && <p className="ft-hint">草稿还有未保存的修改。保存后即可打开独立副本。</p>}
        <Button disabled={model.dirty || !copyReason.trim() || !version.usable || !/^[a-z][a-z0-9_]{0,63}$/.test(copyCode.trim())} onClick={() => void model.write({ action: 'copy', source_version: version.id, code: copyCode.trim() }, copyReason)}><Files />创建副本草稿</Button></div>}
      {screen === 'discard' && isDraft && draft && rights.editable && <div className="ft-edit-form"><p>删除「{draft.work.name}」这份草稿{model.dirty ? '及未保存的修改' : ''}？</p>
        {reasonField}<div className="ft-actions"><Button variant="outline" onClick={overview}>保留草稿</Button><Button variant="destructive" disabled={!model.reason.trim() || model.conflict} onClick={async () => {
          if (await model.write({ action: 'delete', revision: definition.revision })) overview();
        }}>确认删除</Button></div></div>}
      {screen === 'publish' && isDraft && draft && rights.publishable && <div className="ft-edit-form"><div className="ft-review-result"><CircleCheck /><div><h4>{model.validation?.valid ? '草稿已通过服务端校验' : '请重新校验草稿'}</h4>
        <p>发布后将生成固定版本 V{model.nextNumber}，供{isMetric ? '新建通道与模板' : '新建设施'}选用。</p></div></div>
        <dl className="ft-facts"><div><dt>{noun}名称</dt><dd>{draft.work.name}</dd></div><div><dt>发布说明</dt><dd>{draft.work.release_notes || '尚未填写'}</dd></div></dl>
        {reasonField}<div className="ft-actions"><Button variant="outline" onClick={overview}>返回草稿</Button><Button disabled={!model.validation?.valid || model.dirty || !model.reason.trim()} onClick={async () => {
          if (await model.write({ action: 'publish', revision: definition.revision })) overview();
        }}><Check />确认发布 V{model.nextNumber}</Button></div></div>}
      {screen === 'reload' && <div className="ft-edit-form"><p>{model.dirty ? '重新读取会放弃本页尚未保存的修改，并载入最新草稿。' : '读取最新的草稿、发布版本和使用状态。'}</p>
        <div className="ft-actions"><Button variant="outline" onClick={overview}>取消</Button><Button onClick={async () => { if (await model.reload()) overview(); }}>确认重新读取</Button></div></div>}
    </article>
  </div></div>;
}

export function DraftActions({ model, rights, onOverview, onPublish, atOverview }: {
  model: Maintenance; rights: Rights; onOverview: () => void; onPublish: () => void; atOverview: boolean;
}) {
  if (!model.isDraft) return null;
  return <div className="ft-savebar">
    {model.dirty && rights.editable && <TextControl label="本次变更原因" value={model.reason} onChange={model.setReason} />}
    <div className="ft-actions">
      {!atOverview && <Button variant="outline" onClick={onOverview}><ArrowLeft />草稿概览</Button>}
      {model.dirty && rights.editable ? <Button disabled={!model.reason.trim() || model.conflict} onClick={() => void model.save()}>保存草稿</Button> :
        model.validation?.valid && rights.publishable ? <Button onClick={onPublish}>发布草稿<ArrowRight /></Button> :
          <Button disabled={model.dirty} onClick={() => { onOverview(); void model.validate(); }}><ShieldCheck />校验草稿</Button>}
    </div></div>;
}
