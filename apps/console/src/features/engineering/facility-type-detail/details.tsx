import { ChevronDown } from 'lucide-react';
import type { EngineeringField, EngineeringVersion } from '@infrapro/api-client';
import { displayValue, fieldGroups, fieldShape } from './facts';

export function FieldDetails({ fields, units }: { fields: EngineeringField[]; units: EngineeringVersion[] }) {
  if (!fields.length) return <p className="ft-muted">这里沿用系统的通用资料，没有额外字段。</p>;
  return <div className="space-y-5">
    {fieldGroups(fields).map(([group, items]) => <section key={group}>
      <h3 className="ft-group-title">{group}</h3>
      <div className="divide-y border-y">
        {items.map((field, i) => {
          const unit = units.find((item) => item.id === field.unit_version);
          const details = [
            field.options?.length && ['可选值', field.options.map((o) => o.name).join('、')],
            field.unit_version && ['单位', unit ? unit.content.name + (unit.content.unit?.symbol ? ' (' + unit.content.unit.symbol + ')' : '') : '已引用的单位版本'],
            field.minimum != null && ['最小值', field.minimum], field.maximum != null && ['最大值', field.maximum],
            field.min_length != null && ['最少字数', field.min_length], field.max_length != null && ['最多字数', field.max_length],
            field.min_items != null && ['最少选项', field.min_items], field.max_items != null && ['最多选项', field.max_items],
            field.precision != null && ['显示小数位数', field.precision],
            field.default !== undefined && ['默认值', displayValue(field, field.default)],
            field.key && ['字段编号', field.key],
          ].filter((entry): entry is [string, string | number] => Array.isArray(entry));
          return <details key={field.key || i} className="ft-disclosure">
            <summary><strong>{field.name || '未命名字段'}</strong><span className="ft-trailing">{fieldShape(field)} · {field.required ? '必填' : '选填'}</span><ChevronDown aria-hidden="true" /></summary>
            <div className="space-y-2 pb-4 text-sm">
              {field.description && <p>{field.description}</p>}
              <dl className="ft-facts">{details.map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>
            </div>
          </details>;
        })}
      </div>
    </section>)}
  </div>;
}
