import { useState } from 'react';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { useQuery } from '@tanstack/react-query';
import { Permissions, type EngineeringContent, type EngineeringVersion } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { ArrowRight } from 'lucide-react';
import { outcomes } from '../../audit/presentation';
import { TextControl } from '../controls';
import { shapeLabels } from '../model';
import { stamp } from './facts';
import { type Maintenance } from './maintenance';

const labels: Record<string, string> = {
  name: '名称', description: '说明', release_notes: '发布说明', key: '编号', order: '排列顺序',
  fields: '建档字段', components: '构件', relations: '归属规则', structure: '监测位置', monitoring_section: '监测断面', borehole: '孔位',
  parent: '上级对象', child: '下级构件', supported: '允许建档', min_count: '最少数量', max_count: '最多数量',
  metamodel_version: '建模格式', shape: '填写方式', required: '必填', options: '可选项', group: '资料分组', unit_version: '计量单位',
  minimum: '最小数值', maximum: '最大数值', min_length: '最少字数', max_length: '最多字数', min_items: '最少选项', max_items: '最多选项',
  metric: '指标规则', facility_topics: '设施索引', allowed_units: '允许单位', standard_unit: '标准单位', category: '单位类别',
  precision: '小数位数', default: '默认值', multiline: '多行填写',
};
function objectName(value: unknown, content: EngineeringContent): string {
  if (value == null) return '无';
  if (typeof value === 'boolean') return value ? '是' : '否';
  if (typeof value !== 'object') return String(value) || '空白';
  if (Array.isArray(value)) return value.map((v) => objectName(v, content)).join('、') || '无';
  if ('name' in value && typeof value.name === 'string') return value.name;
  if ('parent' in value && 'child' in value) {
    const name = (key: unknown) => key === 'root' ? '设施本体' : content.components?.find((c) => c.key === key)?.name ?? String(key);
    return name(value.parent) + ' → ' + name(value.child);
  }
  return JSON.stringify(value);
}
function differenceName(path: string, before: EngineeringContent, after: EngineeringContent) {
  let a: unknown = before, b: unknown = after;
  const names: string[] = [];
  for (const segment of path.split('/').filter(Boolean)) {
    const child = (item: unknown) => item && typeof item === 'object' ? (item as Record<string, unknown>)[segment] : undefined;
    a = child(a); b = child(b);
    names.push(/^\d+$/.test(segment) ? objectName(b ?? a, b ? after : before) : labels[segment] ?? segment);
  }
  return names.join(' / ');
}

function differenceValue(path: string, value: unknown, content: EngineeringContent, references: EngineeringVersion[]) {
  const key = path.split('/').at(-1);
  if (key === 'allowed_units' && Array.isArray(value)) return value.map(id => {
    const unit = references.find(v => v.id === id);
    return unit ? unit.content.name + ' V' + unit.number : String(id);
  }).join('、') || '无';
  if (typeof value === 'string') {
    if (key === 'parent' || key === 'child') {
      return value === 'root' ? '设施本体' : content.components?.find((part) => part.key === value)?.name ?? value;
    }
    if (key === 'shape' && Object.hasOwn(shapeLabels, value)) return shapeLabels[value as keyof typeof shapeLabels];
    if (key === 'unit_version' || key === 'standard_unit' || path.includes('/allowed_units/')) {
      const unit = references.find((v) => v.id === value);
      if (unit) return unit.content.name + ' V' + unit.number;
    }
  }
  if (value && typeof value === 'object' && 'supported' in value) {
    return value.supported ? '可按需建立' : '不适用';
  }
  return objectName(value, content);
}

export function VersionCompare({ model }: { model: Maintenance }) {
  const { definition, draft, version, releases, selected } = model;
  const { client, account } = useIdentity();
  const options = [...releases.map((r) => ({ id: r.id, label: 'V' + r.number, content: r.content })),
    ...(definition.draft ? [{ id: 'draft', label: '已保存草稿', content: definition.draft.content }] : [])];
  const choices = options.filter((r) => r.id !== selected);
  const [choice, setChoice] = useState(version && draft ? 'draft' : draft?.base_version);
  const from = choices.find((r) => r.id === choice) ?? choices[0], to = options.find((r) => r.id === selected);
  const q = useQuery({
    queryKey: ['engineering-version-diff', model.scope.kind, account?.id, definition.id, definition.revision, from?.id, to?.id],
    queryFn: ({ signal }) => client.engineeringDiff(model.scope, definition.id, from!.id === 'draft' ? '' : from!.id, to!.id === 'draft' ? '' : to!.id, definition.revision, signal),
    enabled: !!from && !!to, retry: false,
  });
  if (!from || !to) return <p className="ft-muted">有两份已保存的内容时可以查看差异。</p>;
  return <div className="ft-edit-form"><label className="ft-label">对照版本<select className="ft-select" value={from.id} onChange={(e) => setChoice(e.target.value)}>
    {choices.map((o) => <option key={o.id} value={o.id}>{o.label}</option>)}</select></label>
    {model.dirty && <p className="ft-hint">当前对比使用已保存的内容，草稿还有待保存的修改。</p>}
    {q.isError ? <div role="alert"><p>{q.error.message}</p><Button variant="outline" onClick={() => void q.refetch()}>重试</Button></div> :
      q.isPending ? <p role="status">正在比较版本…</p> : <><p className="ft-hint">从 {from.label} 到 {to.label}，{q.data.length ? '共 ' + q.data.length + ' 项变更。' : '定义内容一致。'}</p>
        <ul className="ft-changes">{q.data.map((c) => {
          const before = differenceValue(c.path, c.before, from.content, model.known);
          const after = differenceValue(c.path, c.after, to.content, model.known);
          const sameLabel = before === after && typeof c.before === 'string' && typeof c.after === 'string';
          return <li key={c.path}><strong>{differenceName(c.path, from.content, to.content)}</strong>
            <div><span>{sameLabel ? before + ' · ' + c.before : before}</span><ArrowRight /><span>{sameLabel ? after + ' · ' + c.after : after}</span></div></li>;
        })}</ul></>}
  </div>;
}

export function OperationRecords({ model }: { model: Maintenance }) {
  const identity = useIdentity();
  const id = model.definition.id;
  const allowed = identity.can(Permissions.PlatformAuditRead);
  const q = usePagedQuery({
    queryKey: ['engineering-type-events', identity.account?.id, identity.authorization_revision, identity.authorizationEpoch, id, model.definition.revision, model.version?.state_revision],
    initialPageSize: 10, enabled: allowed, retry: false,
    queryFn: ({ cursor, limit, signal }) => identity.client.auditEvents({ kind: 'platform' }, { object_type: 'engineering_definition', object_id: id, cursor: cursor || undefined, limit }, signal),
  });
  if (!allowed) return null;
  return <div>{q.error ? <div role="alert"><p>暂时无法读取记录。</p><Button variant="outline" onClick={() => void q.refetch()}>重试</Button></div> :
    q.isPending ? <p role="status">正在读取记录…</p> : q.data?.items.length ? <ol className="ft-log">{q.data.items.map((e) => <li key={e.id}><strong>{e.action_label}</strong>
      <small>{stamp(e.occurred_at, true)}</small><p>{e.actor.name} · {outcomes[e.outcome]}{e.reason && ' · ' + e.reason}</p></li>)}</ol> : <p className="ft-muted">暂无可见记录。</p>}
    {q.data && <ListPagination label="平台操作记录" {...q.pagination} />}</div>;
}

export function Availability({ model, done }: { model: Maintenance; done: () => void }) {
  const { client, account, authorizationEpoch } = useIdentity();
  const { definition, version, known } = model;
  const [reason, change] = useState('');
  const q = useQuery({
    queryKey: ['engineering-version-impact', model.scope.kind, account?.id, authorizationEpoch, definition.id, definition.revision, version?.id, version?.state_revision],
    queryFn: ({ signal }) => client.engineeringImpact(model.scope, definition.id, version!.id, signal),
    enabled: !!version, retry: false,
  });
  if (!version) return null;
  const enable = !version.enabled;
  const blocked = enable && version.dependencies.some((id) => !known.find((v) => v.id === id)?.usable);
  const consumer = model.scope.kind === 'metrics' ? '通道与模板' : '设施';
  return <div className="ft-edit-form"><p>{enable ? '恢复后，新建' + consumer + '可以选用这个版本。' : '停用后，新建' + consumer + '将无法选用这个版本。已有引用保留原版本。'}</p>
    <p className="ft-hint" role="status">{q.isError ? '引用情况读取失败。' : q.isPending ? '正在读取引用情况…' :
      '可见的定义引用 ' + q.data.public.length + ' 个。' + (q.data.private_references ? '另有受限引用。' : '') + (q.data.resource_references ? '已有租户资源引用。' : '')}</p>
    {q.isError && <Button variant="outline" onClick={() => void q.refetch()}>重读引用情况</Button>}
    {blocked && <p className="ft-hint">需要先恢复所引用的单位版本。</p>}
    <TextControl label="变更原因" value={reason} onChange={change} />
    <Button disabled={!reason.trim() || blocked || !q.isSuccess} onClick={async () => {
      if (await model.write({ action: 'state', version_id: version.id, revision: version.state_revision, enabled: enable }, reason)) done();
    }}>{enable ? '确认恢复' : '确认停用'}</Button></div>;
}
