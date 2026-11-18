import { useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import { Permissions } from '@infrapro/api-client';
import { Eye, FlaskConical } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { DesignPrototypeSwitcher } from '@infrapro/ui/patterns/design-prototype-switcher';
import { ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { Directory } from './directory';
import { MetricDetail } from './detail';
import { shapes, variants, type Variant } from './model';
import './prototype.css';

type Props = { variant: Variant; definition?: string; select: (id?: string) => void; change: (key: Variant) => void };
const choices = variants.map(({ key, name }) => ({ key, name }));

function Categories() {
  const { client, account } = useIdentity();
  const query = useQuery({ queryKey: ['platform', account?.id, 'engineering', 'categories'], queryFn: ({ signal }) => client.engineeringCategories(undefined, signal) });
  if (query.isError) return <ErrorState message={query.error.message} onRetry={() => void query.refetch()} />;
  if (!query.data) return <LoadingState label="正在读取单位类别…" />;
  return <div className="mc-panel"><p className="mc-note">这里是平台的单位类别，不是设施导航分组。原型只读，不修改类别。</p><ul className="mc-history">{query.data.map(category => <li key={category.key}><strong>{category.name}</strong><code className="mc-code">{category.key}</code>{category.description && <p>{category.description}</p>}</li>)}</ul></div>;
}
function DraftSketch() {
  const [name, setName] = useState(''), [code, setCode] = useState(''), [shape, setShape] = useState<keyof typeof shapes>('decimal'), [preview, setPreview] = useState(false);
  return <form className="mc-panel mc-management" onSubmit={event => { event.preventDefault(); setPreview(true); }}>
    <label className="mc-field">指标名称<input required maxLength={100} value={name} placeholder="例如：隧道入口照度" onChange={event => { setName(event.target.value); setPreview(false); }} /></label>
    <label className="mc-field">稳定编号<input required pattern="[a-z][a-z0-9_]*" maxLength={80} value={code} placeholder="例如：metric_portal_illuminance" onChange={event => { setCode(event.target.value); setPreview(false); }} /></label>
    <label className="mc-field">取值形态<select value={shape} onChange={event => { setShape(event.target.value as keyof typeof shapes); setPreview(false); }}>{Object.entries(shapes).map(([key, name]) => <option key={key} value={key}>{name}</option>)}</select></label>
    <Button type="submit"><Eye />预览草稿卡片</Button>{preview && <div role="status" className="mc-draft-preview"><p className="mc-overline">未保存的草稿预演</p><h3>{name}</h3><code className="mc-code">{code}</code><p>{shapes[shape]} · {shape === 'enum' ? '待配置枚举选项' : '待配置单位和取值范围'}</p></div>}
    <p className="mc-note">仅演示录入布局，关闭后丢弃，不会新增目录记录。</p></form>;
}
function Workspace({ variant, definition, select, change, reset, editable }: Props & { reset: () => void; editable: boolean }) {
  const [panel, setPanel] = useState<'create' | 'categories' | null>(null);
  const trigger = useRef<HTMLElement | null>(null);
  return <section className="mc-prototype" data-variant={variant} aria-label="指标目录设计原型">
    <Directory active={!definition} variant={variant} select={select} editable={editable} manage={next => { trigger.current = document.activeElement as HTMLElement; setPanel(next); }} />
    {definition && <MetricDetail key={definition} id={definition} back={() => select()} variant={variant} editable={editable} />}
    <p className="mc-prototype-note"><FlaskConical />设计原型 · {variant} {choices.find(item => item.key === variant)?.name} · 不写入业务数据</p>
    <DesignPrototypeSwitcher current={variant} variants={choices} change={key => change(key as Variant)} reset={reset} />
    <ResponsivePanel open={!!panel} onOpenChange={open => { if (!open) setPanel(null); }} title={panel === 'create' ? '新建指标 · 试填' : '单位类别'} description={panel === 'create' ? '先确定名称、编号和记录形态。' : '数值指标通过固定单位版本引用这些类别。'} returnFocus={trigger}>
      {panel === 'create' ? <DraftSketch /> : panel === 'categories' ? <Categories /> : null}</ResponsivePanel>
  </section>;
}
export default function MetricCatalogPrototype(props: Props) {
  const { account, can, authorizationEpoch } = useIdentity();
  const [epoch, setEpoch] = useState(0);
  if (!can(Permissions.EngineeringMetricRead)) return <ErrorState title="没有工程目录读取权限" message="请联系当前范围的管理员配置相应角色。" />;
  return <Workspace key={`${account?.id}:${authorizationEpoch}:${epoch}`} {...props} editable={can(Permissions.EngineeringMetricEdit)} reset={() => { setEpoch(value => value + 1); if (props.definition) props.select(); }} />;
}
