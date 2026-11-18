import { useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import { Permissions, type EngineeringContent } from '@infrapro/api-client';
import { ArrowLeft, ArrowRight, BookOpen, Check, ChevronRight, ClipboardList, FileClock, FlaskConical, GitBranch, History, ListTree, PencilLine, Ruler, Shapes } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { bounds, description, detailQuery, example, exampleLabel, initialTrial, recordView, shapes, stamp, standardSymbol, unitRef, versionState, type RecordView, type Variant } from './model';
import { SampleBench } from './sample';

function Units({ view }: { view: RecordView }) {
  if (view.rule.shape === 'enum') return <>无物理单位</>;
  return <span className="mc-chips">{view.rule.allowed_units?.length ? view.rule.allowed_units.map(id => { const unit = unitRef(view, id); return <span key={id}>{unit.name} {unit.symbol}{unit.number ? ` · V${unit.number}` : ''}</span>; }) : '尚未配置允许单位'}</span>;
}
function Options({ view }: { view: RecordView }) {
  return view.rule.options?.length ? <ul className="mc-options">{view.rule.options.map(option => <li key={option.key}><strong>{option.name}</strong><code>{option.key}</code></li>)}</ul> : <p className="mc-note">尚未定义选项，不能从此规则生成有效的枚举样例。</p>;
}
function RuleFacts({ view }: { view: RecordView }) {
  const { rule } = view, unit = unitRef(view, rule.standard_unit);
  return <dl className="mc-facts"><div><dt>取值形态</dt><dd>{shapes[rule.shape]}</dd></div>{rule.shape === 'enum' ? <><div><dt>物理单位</dt><dd>不使用</dd></div><div><dt>允许选项</dt><dd><Options view={view} /></dd></div></> : <>
    <div><dt>数值范围</dt><dd>{bounds(rule)}</dd></div><div><dt>标准单位</dt><dd>{unit.name} {unit.symbol}{unit.number && ` · V${unit.number}`}</dd></div>
    <div><dt>允许单位</dt><dd><Units view={view} /></dd></div><div><dt>单位类别</dt><dd>{unit.category || rule.category || '尚未填写'}</dd></div></>}</dl>;
}
function LedgerDetail({ view, inspect }: { view: RecordView; inspect: () => void }) {
  const info = description(view.content);
  return <div className="mc-sheet"><section><p className="mc-overline"><ClipboardList />规格表</p><h2>记录规格</h2><RuleFacts view={view} /></section>
    <aside><p className="mc-overline"><BookOpen />使用说明</p><h3>在哪里使用</h3><dl className="mc-facts"><div><dt>适用设施</dt><dd>{info.facilities || '定义未注明'}</dd></div><div><dt>业务类别</dt><dd>{info.category || '定义未注明'}</dd></div></dl>
      <p className="mc-note">{view.rule.shape === 'enum' ? '记录选项编码，不附加单位。' : '记录单位须与通道单位一致。允许单位不表示自动换算。'}</p><Button variant="outline" onClick={inspect}><FlaskConical />试填样例</Button></aside></div>;
}

function RuleMap({ view, inspect }: { view: RecordView; inspect: () => void }) {
  const [step, setStep] = useState(0);
  const { rule } = view, isEnum = rule.shape === 'enum';
  const steps = [
    { label: '取值形态', value: isEnum ? '单个枚举状态' : `单值${shapes[rule.shape]}`, icon: Shapes },
    { label: isEnum ? '枚举选项' : '单位约束', value: isEnum ? `${rule.options?.length ?? 0} 个选项` : `标准单位 · ${standardSymbol(view)}`, icon: isEnum ? ListTree : Ruler },
    { label: '记录样例', value: isEnum ? '试选一个状态' : '试填数值与单位', icon: FlaskConical },
  ];
  const selected = steps[step]!;
  return <section><div className="mc-section-line"><div><p className="mc-overline"><GitBranch />规则导览</p><h2>一条记录由什么组成？</h2><p className="mc-muted">点选一项规则，查看这个版本如何约束记录。</p></div></div>
    <div className="mc-rule-map"><nav aria-label="指标记录规则"><p className="mc-rail-label">记录规则</p><ol>{steps.map(({ label, value, icon: Icon }, index) => <li key={label}><button type="button" aria-pressed={step === index} onClick={() => setStep(index)}><span className="mc-step-icon"><Icon /></span><span><strong>{label}</strong><small>{value}</small></span><ChevronRight className="mc-end" /></button></li>)}</ol></nav>
    <article className="mc-inspector" aria-live="polite"><p className="mc-overline"><span className="mc-sequence">0{step + 1} / 03</span>{selected.label}</p>
      {step === 0 ? <><h3>{selected.value}</h3><p>{isEnum ? '每条记录只能选一个已定义的状态，保存对应编码。' : rule.shape === 'integer' ? '每条记录填写一个整数，不接受小数。' : '每条记录填写一个数值，可以带小数。'}</p>
        <dl className="mc-facts"><div><dt>{isEnum ? '选项范围' : '数值边界'}</dt><dd>{bounds(rule)}</dd></div><div><dt>{isEnum ? '记录单位' : '标准单位'}</dt><dd>{standardSymbol(view)}</dd></div><div><dt>记录示例</dt><dd>{exampleLabel(view, example(view))}{!isEnum && ' ' + standardSymbol(view)}</dd></div></dl></>
        : step === 1 ? <><h3>{isEnum ? '可记录哪些状态' : '可使用哪些单位'}</h3>{isEnum ? <Options view={view} /> : <><p>标准单位为 {unitRef(view, rule.standard_unit).name}（{standardSymbol(view)}）。记录时须使用下列固定单位版本之一。</p><Units view={view} /><p className="mc-note">目录只声明允许关系，不自动换算单位。</p></>}</>
        : <><h3>读一条{view.content.name}样例</h3><div className="mc-reading"><span>{exampleLabel(view, example(view))}</span>{!isEnum && <small>{standardSymbol(view)}</small>}</div><p>这是按当前规则构造的示例，不是实测值。</p></>}
      <div className="mc-inspector-foot">{step < 2 ? <Button variant="outline" onClick={() => setStep(step + 1)}>下一项：{steps[step + 1]!.label}<ArrowRight /></Button> : <Button onClick={inspect}><FlaskConical />试填样例</Button>}</div>
    </article></div></section>;
}

function AuditSnippet({ id }: { id: string }) {
  const { account, client, authorization_revision, authorizationEpoch } = useIdentity();
  const logs = useQuery({ queryKey: ['metric-prototype-log', account?.id, authorization_revision, authorizationEpoch, id], queryFn: ({ signal }) => client.auditEvents({ kind: 'platform' }, { object_type: 'engineering_definition', object_id: id, limit: 5 }, signal), retry: false });
  return logs.isError ? <p role="alert">未能读取可见操作记录。</p> : !logs.data ? <LoadingState label="正在读取操作记录…" /> : logs.data.items.length ? <ol className="mc-history">{logs.data.items.map(event => <li key={event.id}><strong>{event.action_label}</strong><small>{stamp(event.occurred_at)} · {event.actor.name || '未记录经办人'}</small>{event.reason && <p>{event.reason}</p>}</li>)}</ol> : <p className="mc-muted">暂无可见操作记录。</p>;
}

function DetailBody({ view, variant }: { view: RecordView; variant: Variant }) {
  const [sampleOpen, setSampleOpen] = useState(false), [trial, change] = useState(() => initialTrial(view));
  const trigger = useRef<HTMLElement | null>(null);
  const inspect = () => { trigger.current = document.activeElement as HTMLElement; setSampleOpen(true); };
  const bench = <SampleBench view={view} trial={trial} change={change} />;
  return <>{variant === 'A' ? <LedgerDetail view={view} inspect={inspect} /> : variant === 'B' ? <RuleMap view={view} inspect={inspect} /> : <section>
    <div className="mc-section-line"><div><p className="mc-overline"><FlaskConical />样例手册</p><h2>这条记录符合定义吗？</h2><p className="mc-muted">改动输入，右侧逐项解释核对结果。</p></div></div>{bench}
    <details className="mc-disclosure"><summary>查看完整取值规则</summary><RuleFacts view={view} /></details></section>}
    <details className="mc-disclosure"><summary>查看完整定义说明</summary><p>{view.content.description || '未填写定义说明。'}</p></details>
    <ResponsivePanel open={sampleOpen} onOpenChange={setSampleOpen} title="试填样例" description={view.content.name + ' · 仅在此页面核对'} size="wide" returnFocus={trigger}><div className="mc-panel">{bench}</div></ResponsivePanel>
  </>;
}

export function MetricDetail({ id, variant, back, editable }: { id: string; variant: Variant; back: () => void; editable: boolean }) {
  const { client, account, can } = useIdentity();
  const detail = useQuery(detailQuery(client, account?.id, id));
  const [selection, setSelection] = useState<string>(), [open, setOpen] = useState(false), [audit, setAudit] = useState(false);
  const [trial, setTrial] = useState<{ base: string; content: EngineeringContent }>();
  const versionTrigger = useRef<HTMLButtonElement>(null);
  const isTrial = selection === 'trial' && !!trial;
  const saved = recordView(detail.data, isTrial ? trial.base : selection);
  const view = isTrial && saved && trial.content.metric ? { ...saved, content: trial.content, rule: trial.content.metric, version: undefined } : saved;
  const selectedId = isTrial ? 'trial' : view?.version?.id ?? 'draft';
  const status = isTrial ? '仅本次预演' : versionState(view?.version);
  return <div><div className="mc-path"><button type="button" onClick={back}><ArrowLeft />指标目录</button><ChevronRight /><span>指标详情</span></div>
    {detail.isError ? <ErrorState title="当前指标不可读取" message={detail.error.message} onRetry={() => void detail.refetch()} /> : !detail.data ? <LoadingState label="正在读取指标定义…" /> : !view ? <p>此定义尚无可查看的取值规则。</p> : <>
      <header className="mc-page-head mc-detail-head"><div><div className="mc-title-line"><h1>{view.content.name || view.definition.code}</h1><span className="mc-version" data-usable={!!view.version?.usable}>{view.version ? 'V' + view.version.number : '草稿'} · {status}</span></div>
        <p className="mc-lede">{description(view.content).intro || '此版本尚未填写定义说明。'}</p><div className="mc-detail-meta"><code className="mc-code">{view.definition.code}</code>{description(view.content).facilities && <span>适用设施：{description(view.content).facilities}</span>}</div></div>
        <Button ref={versionTrigger} variant="outline" onClick={() => setOpen(true)}><History />版本与管理</Button></header>
      <DetailBody key={selectedId} view={view} variant={variant} />
      <ResponsivePanel open={open} onOpenChange={setOpen} title="版本与管理" description={`${view.content.name} · ${detail.data.versions.length} 个发布版本 · 原型不提交变更`} returnFocus={versionTrigger}>
        <div className="mc-panel mc-management"><h3>查看版本</h3><div className="mc-version-list">{[...(trial ? [{ id: 'trial', label: '预演草稿', note: '仅本次查看，未保存' }] : []), ...(detail.data.draft ? [{ id: 'draft', label: '已有草稿', note: '更新于 ' + stamp(detail.data.draft.updated_at) }] : []), ...detail.data.versions.map(version => ({ id: version.id, label: `V${version.number} · ${versionState(version)}`, note: '发布于 ' + stamp(version.published_at) }))].map(option => <button type="button" key={option.id} aria-pressed={option.id === selectedId} onClick={() => setSelection(option.id)}><FileClock /><span><strong>{option.label}</strong><small>{option.note}</small></span>{option.id === selectedId && <Check />}</button>)}</div>
        <div><h3>版本发布说明</h3><p className="mc-note">{view.content.release_notes || '尚未填写发布说明。'}</p></div>
        {editable && view.version && !detail.data.draft && !trial && <Button variant="outline" onClick={() => { setTrial({ base: view.version!.id, content: { ...view.content } }); setSelection('trial'); }}><PencilLine />从此版本试建草稿</Button>}
        {isTrial && <label className="mc-field">试改草稿名称<input value={trial.content.name} onChange={event => setTrial({ ...trial, content: { ...trial.content, name: event.target.value } })} /><small>仅内存预演，不保存、不发布。</small></label>}
        {can(Permissions.PlatformAuditRead) && <details className="mc-disclosure" onToggle={event => setAudit(event.currentTarget.open)}><summary>最近操作记录（最多 5 条）</summary>{audit && open && <AuditSnippet id={id} />}</details>}
        <Button onClick={() => setOpen(false)}>查看所选版本<ArrowRight /></Button>
      </div></ResponsivePanel></>}
  </div>;
}
