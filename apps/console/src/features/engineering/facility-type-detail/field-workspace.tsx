import { useState } from 'react';
import { ArrowLeft, ChevronDown, ChevronRight, ListChecks, Plus, Trash2 } from 'lucide-react';
import type { EngineeringField, EngineeringVersion } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { CheckControl, NumberControl, OptionsEditor, TextControl, ValueInput } from '../controls';
import { emptyField, shapeLabels } from '../model';
import { VersionPicker } from '../reference-picker';
import { fieldGroups, fieldShape, fieldSummary } from './facts';

export function FieldWorkspace({ fields, change, units, remember }: {
  fields: EngineeringField[]; change: (fields: EngineeringField[]) => void; units: EngineeringVersion[]; remember: (v: EngineeringVersion) => void;
}) {
  const [index, select] = useState<number | null>(null);
  const current = index === null ? undefined : fields[index];
  const update = (patch: Partial<EngineeringField>) => change(fields.map((f, i) => i === index ? { ...f, ...patch } : f));
  if (!current) return <div className="ft-field-list">
    <div className="ft-inline-head"><p className="ft-muted">{fieldSummary(fields)}</p>
      <Button variant="outline" onClick={() => { change([...fields, { ...emptyField(), order: fields.length + 1 }]); select(fields.length); }}><Plus />添加字段</Button></div>
    {fieldGroups(fields).map(([group, list]) => <section key={group}><h4 className="ft-small-title">{group}</h4>
      {list.map((field) => <button key={fields.indexOf(field)} type="button" className="ft-edit-row" onClick={() => select(fields.indexOf(field))}>
        <span>{field.name || '未命名字段'}</span><small>{fieldShape(field)} · {field.required ? '必填' : '选填'}</small><ChevronRight /></button>)}
    </section>)}
  </div>;
  const f = current;
  const numeric = f.shape === 'integer' || f.shape === 'decimal';
  return <div className="ft-edit-form">
    <div className="ft-inline-head"><button type="button" className="ft-text-action" onClick={() => select(null)}><ArrowLeft />字段列表</button>
      <Button size="sm" variant="ghost" onClick={() => { change(fields.filter((_, i) => i !== index)); select(null); }}><Trash2 />移除字段</Button></div>
    <div className="ft-form-grid">
      <TextControl label="字段名称" value={f.name} onChange={(name) => update({ name })} />
      <TextControl label="字段编号" value={f.key} onChange={(key) => update({ key })} hint="小写字母开头，可用数字和下划线。" />
      <label className="ft-label">填写方式<select className="ft-select" value={f.shape} onChange={(e) => {
        const shape = e.target.value as EngineeringField['shape'];
        const next = { ...emptyField(), key: f.key, name: f.name, description: f.description, group: f.group, order: f.order, required: f.required, shape };
        change(fields.map((item, i) => i === index ? next : item));
      }}>{Object.entries(shapeLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label>
      <CheckControl label="建档时必填" value={f.required} onChange={(required) => update({ required })} />
    </div>
    {(f.shape === 'enum' || f.shape === 'multi_enum') && <OptionsEditor value={f.options ?? []} onChange={(options) => update({ options })} />}
    <details className="ft-options"><summary><ListChecks />填写规则与显示设置<ChevronDown /></summary><div className="ft-edit-form">
      <TextControl label="填写说明" value={f.description} onChange={(description) => update({ description })} />
      <div className="ft-form-grid"><TextControl label="资料分组" value={f.group} onChange={(group) => update({ group })} />
        <NumberControl label="排列顺序" value={f.order} onChange={(order) => update({ order: order ?? 0 })} />
        {f.shape === 'text' && <><NumberControl label="最少字数" value={f.min_length} onChange={(min_length) => update({ min_length })} />
          <NumberControl label="最多字数" value={f.max_length} onChange={(max_length) => update({ max_length })} />
          <CheckControl label="多行填写" value={f.multiline} onChange={(multiline) => update({ multiline })} /></>}
        {numeric && <><NumberControl label="最小数值" value={f.minimum} onChange={(minimum) => update({ minimum })} />
          <NumberControl label="最大数值" value={f.maximum} onChange={(maximum) => update({ maximum })} />
          <NumberControl label="小数位数" value={f.precision} onChange={(precision) => update({ precision })} /></>}
        {f.shape === 'multi_enum' && <><NumberControl label="最少选项数" value={f.min_items} onChange={(min_items) => update({ min_items })} />
          <NumberControl label="最多选项数" value={f.max_items} onChange={(max_items) => update({ max_items })} /></>}
      </div>
      {numeric && <VersionPicker kind="units" label="计量单位" value={f.unit_version ?? ''} known={units}
        onChange={(id, v) => { if (v) remember(v); update({ unit_version: id || undefined }); }} />}
      <ValueInput field={{ ...f, name: '默认值', required: false }} value={f.default} onChange={(value) => update({ default: value })} />
      {f.default != null && <Button variant="ghost" onClick={() => update({ default: undefined })}>清除默认值</Button>}
    </div></details>
  </div>;
}
