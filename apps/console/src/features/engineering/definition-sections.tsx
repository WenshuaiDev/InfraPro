import type { ReactNode } from 'react';
import type { EngineeringContent, EngineeringField, EngineeringVersion } from '@infrapro/api-client';
import { DirectoryTable } from '@infrapro/ui/patterns/directory-table';
import { Section } from './controls';
import { shapeLabels } from './model';

export function DefinitionInfo({ values }: { values: { label: string; value?: ReactNode }[] }) {
  return (
    <dl className="grid gap-x-8 gap-y-5 sm:grid-cols-2">
      {values.map(({ label, value }) => (
        <div key={label} className="min-w-0 space-y-1">
          <dt className="text-sm text-muted-foreground">{label}</dt>
          <dd className="break-words whitespace-pre-wrap text-sm">
            {value === undefined || value === null || value === '' ? '—' : value}
          </dd>
        </div>
      ))}
    </dl>
  );
}

function Fields({ fields, known }: { fields: EngineeringField[]; known: EngineeringVersion[] }) {
  return (
    <DirectoryTable
      label="属性字段"
      items={fields}
      rowId={(field) => field.key}
      empty="未配置属性字段"
      columns={[
        {
          id: 'name',
          label: '字段',
          cell: (field) => (
            <div>
              <p className="font-medium">{field.name}</p>
              <p className="break-all text-xs text-muted-foreground">{field.key}</p>
              {field.description && <p className="mt-1 text-xs text-muted-foreground">{field.description}</p>}
            </div>
          ),
        },
        { id: 'shape', label: '类型', className: 'w-24', cell: (field) => shapeLabels[field.shape] },
        { id: 'required', label: '必填', className: 'w-16', cell: (field) => (field.required ? '是' : '否') },
        {
          id: 'rules',
          label: '规则',
          secondary: true,
          cell: (field) => (
            <span className="text-sm">
              {[
                field.group && `分组：${field.group}`,
                field.minimum !== undefined && `最小 ${field.minimum}`,
                field.maximum !== undefined && `最大 ${field.maximum}`,
                field.options?.map((option) => option.name).join('、'),
                field.unit_version && known.find((v) => v.id === field.unit_version)?.content.unit?.symbol,
              ]
                .filter(Boolean)
                .join('；') || '—'}
            </span>
          ),
        },
      ]}
    />
  );
}

export function DefinitionSnapshot({
  content,
  section,
  known,
}: {
  content: EngineeringContent;
  section: string;
  known: EngineeringVersion[];
}) {
  const ref = (id: string) => {
    const v = known.find((v) => v.id === id);
    return v ? `${v.content.name} · V${v.number}` : '未指定';
  };
  const component = (key: string) =>
    key === 'root' ? '设施本体' : (content.components?.find((c) => c.key === key)?.name ?? key);
  if (section === 'fields') return <Fields fields={content.fields ?? []} known={known} />;
  if (section === 'components')
    return (
      <div className="space-y-5">
        <Section title="构件类别">
          {content.components?.length ? (
            content.components.map((c) => (
              <details key={c.key} className="rounded-lg border p-4">
                <summary className="cursor-pointer text-sm font-medium">
                  {c.name}
                  <span className="ml-3 font-normal text-muted-foreground">
                    {c.fields.length} 个字段 · {c.key}
                  </span>
                </summary>
                <div className="mt-4 space-y-3">
                  {c.description && <p className="text-sm text-muted-foreground">{c.description}</p>}
                  <Fields fields={c.fields} known={known} />
                </div>
              </details>
            ))
          ) : (
            <p className="text-sm text-muted-foreground">未配置构件类别。</p>
          )}
        </Section>
        <DirectoryTable
          label="组成规则"
          items={content.relations ?? []}
          rowId={(r) => `${r.parent}:${r.child}`}
          empty="未配置组成规则"
          columns={[
            { id: 'parent', label: '所属对象', cell: (r) => component(r.parent) },
            { id: 'child', label: '构件类别', cell: (r) => component(r.child) },
            { id: 'count', label: '数量范围', cell: (r) => `${r.min_count} – ${r.max_count ?? '不限'}` },
          ]}
        />
      </div>
    );
  if (section === 'rules' && content.metric) {
    const metric = content.metric;
    return (
      <Section title="取值规则">
        <DefinitionInfo
          values={[
            { label: '数据形态', value: shapeLabels[metric.shape] },
            ...(metric.shape === 'enum'
              ? [{ label: '取值范围', value: '按枚举选项' }]
              : [
                  { label: '单位类别', value: metric.category },
                  { label: '取值范围', value: `${metric.minimum ?? '不限'} ～ ${metric.maximum ?? '不限'}` },
                  { label: '标准单位', value: metric.standard_unit ? ref(metric.standard_unit) : '无' },
                  { label: '允许单位', value: metric.allowed_units?.map(ref).join('、') },
                ]),
          ]}
        />
        {metric.shape === 'enum' && (
          <DirectoryTable
            label="枚举选项"
            items={metric.options ?? []}
            rowId={(v) => v.key}
            columns={[
              { id: 'name', label: '选项', cell: (v) => v.name },
              { id: 'key', label: '保存值', cell: (v) => v.key },
            ]}
          />
        )}
      </Section>
    );
  }
  if (section === 'scope' && content.template)
    return (
      <Section title="适用范围">
        <DefinitionInfo
          values={[
            { label: '监测目的', value: content.template.purpose },
            { label: '适用设施类型', value: content.template.type_versions.map(ref).join('、') },
          ]}
        />
      </Section>
    );
  if (section === 'items' && content.template) {
    const template = content.template;
    return (
      <DirectoryTable
        label="监测项"
        items={template.items}
        rowId={(item) => item.key}
        empty="未配置监测项"
        columns={[
          {
            id: 'name',
            label: '监测项',
            cell: (item) => (
              <div className="space-y-1">
                <p className="font-medium">{item.name}</p>
                <p className="text-xs text-muted-foreground">{item.description || item.key}</p>
                <p className="text-xs text-muted-foreground lg:hidden">
                  {item.bindings.map((b) => ref(b.metric_version)).join('、')}
                </p>
                <details className="pt-1 text-sm">
                  <summary className="cursor-pointer text-muted-foreground">查看配置</summary>
                  <div className="mt-3 space-y-3">
                    <DefinitionInfo
                      values={[
                        { label: '监测分组', value: template.groups.find((g) => g.key === item.group)?.name },
                        { label: '顺序', value: String(item.order) },
                        {
                          label: '适用对象',
                          value: item.targets
                            .map((t) => {
                              const type = known.find((v) => v.id === t.type_version);
                              return `${ref(t.type_version)} / ${t.component === 'root' ? '设施本体' : (type?.content.components?.find((c) => c.key === t.component)?.name ?? t.component)}`;
                            })
                            .join('；'),
                        },
                      ]}
                    />
                    {item.bindings.map((binding, i) => (
                      <p key={i}>
                        {ref(binding.metric_version)}
                        {binding.unit_version && ` · ${ref(binding.unit_version)}`} · {binding.min_count} –{' '}
                        {binding.max_count ?? '不限'} 个通道
                      </p>
                    ))}
                  </div>
                </details>
              </div>
            ),
          },
          {
            id: 'metric',
            label: '监测指标',
            secondary: true,
            cell: (item) => item.bindings.map((b) => ref(b.metric_version)).join('、'),
          },
          { id: 'enabled', label: '默认选用', className: 'w-24', cell: (item) => (item.enabled ? '是' : '否') },
        ]}
      />
    );
  }
  return null;
}
