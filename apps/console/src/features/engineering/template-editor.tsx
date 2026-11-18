import { defaultMetricUnit, metricUnitOptions } from './metric-units';
import { useState } from 'react';
import { CategoryPicker } from '../classification/picker';
import { useOptionalTenant } from '../tenancy/context';
import type { EngineeringTemplate, EngineeringVersion } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { CheckControl, NumberControl, Section, SelectControl, TextControl } from './controls';
import { TargetEditor } from './structure-editor';
import { VersionPicker } from './reference-picker';
export function TemplateEditor({
  value,
  version,
  onUpgrade,
  onChange,
  known,
  section,
  onReferences,
}: {
  value: EngineeringTemplate;
  version: string;
  onUpgrade: () => void;
  onChange: (v: EngineeringTemplate) => void;
  known: EngineeringVersion[];
  section?: 'scope' | 'items';
  onReferences?: (versions: EngineeringVersion[]) => void;
}) {
  const tenant = useOptionalTenant();
  const [picked, setPicked] = useState<EngineeringVersion[]>([]);
  const definitions = [...picked, ...known];
  const types = value.type_versions ?? [];
  const groups = value.groups ?? [];
  const items = value.items ?? [];
  return (
    <div className="space-y-5">
      {version !== '2' && <Section title="扩展监测目标"><p className="text-sm text-muted-foreground">当前为根对象／构件映射。扩展草稿后可按类型映射断面、孔位和测点。</p><Button type="button" variant="outline" onClick={onUpgrade}>扩展目标映射</Button></Section>}
      {section !== 'items' && (
        <>
          <TextControl
            label="监测目的"
            value={value.purpose}
            onChange={(purpose) => onChange({ ...value, purpose })}
            multiline
          />
          <Section
            title="适用类型版本"
            id="/template/type_versions"
            action={
              <Button
                type="button"
                variant="outline"
                onClick={() => onChange({ ...value, type_versions: [...types, ''] })}
              >
                添加适用类型
              </Button>
            }
          >
            {types.map((id, i) => (
              <div className="space-y-2" key={i}>
                <VersionPicker
                  kind="type-definitions"
                  value={id}
                  label={`适用类型 ${i + 1}`}
                  known={definitions}
                  onChange={(id, v, dependencies = []) => {
                    if (v) onReferences?.([v, ...dependencies]);
                    if (v) setPicked([...picked.filter((p) => p.id !== v.id), v]);
                    onChange({
                      ...value,
                      type_versions: types.map((old, j) => (i === j ? id : old)),
                      items: items.map((item) => ({
                        ...item,
                        targets: item.targets.filter((t) => t.type_version !== types[i]),
                      })),
                    });
                  }}
                />
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() =>
                    onChange({
                      ...value,
                      type_versions: types.filter((_, j) => i !== j),
                      items: items.map((item) => ({
                        ...item,
                        targets: item.targets.filter((t) => t.type_version !== id),
                      })),
                    })
                  }
                >
                  删除适用类型 {i + 1}
                </Button>
              </div>
            ))}
          </Section>
        </>
      )}
      {section !== 'scope' && (
        <>
          <Section
            title="监测分组"
            action={
              <Button
                type="button"
                variant="outline"
                onClick={() =>
                  onChange({
                    ...value,
                    groups: [...groups, { key: '', name: '', description: '', order: groups.length }],
                  })
                }
              >
                添加监测分组
              </Button>
            }
          >
            {groups.map((g, i) => {
              const update = (patch: Partial<typeof g>) =>
                onChange({ ...value, groups: groups.map((v, j) => (i === j ? { ...v, ...patch } : v)) });
              return (
                <div key={i} className="space-y-3 rounded-lg border p-3">
                  <div className="grid gap-3 sm:grid-cols-3">
                    <TextControl label="分组键" value={g.key} onChange={(key) => update({ key })} />
                    <TextControl label="分组名称" value={g.name} onChange={(name) => update({ name })} />
                    <NumberControl
                      label="分组顺序"
                      value={g.order}
                      onChange={(order) => update({ order: order ?? 0 })}
                    />
                  </div>
                  <TextControl
                    label="分组说明"
                    value={g.description}
                    onChange={(description) => update({ description })}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    onClick={() => onChange({ ...value, groups: groups.filter((_, j) => j !== i) })}
                  >
                    删除分组 {i + 1}
                  </Button>
                </div>
              );
            })}
          </Section>
          <Section
            title="监测项"
            id="/template/items"
            action={
              <Button
                type="button"
                variant="outline"
                onClick={() =>
                  onChange({
                    ...value,
                    items: [
                      ...items,
                      {
                        key: '',
                        name: '',
                        description: '',
                        enabled: true,
                        group: '',
                        order: items.length,
                        targets: types.filter(Boolean).map((type_version) => ({ type_version, component: version === '2' ? '' : 'root', ...(version === '2' ? { applicability: 'applicable' as const, anchor_kind: 'facility' as const, point_scope: 'descendant_points' as const, quantity_basis: 'per_anchor' as const } : {}) })),
                        bindings: [
                          { metric_version: '', unit_version: '', resource_kind: 'observation_channel', min_count: 1 },
                        ],
                      },
                    ],
                  })
                }
              >
                添加监测项
              </Button>
            }
          >
            {items.map((item, i) => {
              const update = (patch: Partial<typeof item>) =>
                onChange({ ...value, items: items.map((v, j) => (i === j ? { ...v, ...patch } : v)) });
              return (
                <Section
                  key={i}
                  id={`/template/items/${i}`}
                  title={`监测项 ${i + 1} · ${item.name || '未命名'}`}
                  action={
                    <Button
                      type="button"
                      variant="ghost"
                      onClick={() => onChange({ ...value, items: items.filter((_, j) => i !== j) })}
                    >
                      删除监测项 {i + 1}
                    </Button>
                  }
                >
                  <div className="grid gap-3 sm:grid-cols-2">
                    <TextControl label="监测项键" value={item.key} onChange={(key) => update({ key })} />
                    <TextControl label="监测项名称" value={item.name} onChange={(name) => update({ name })} />
                    <SelectControl
                      label="所属分组"
                      value={item.group}
                      onChange={(group) => update({ group })}
                      options={groups
                        .filter((g) => g.key)
                        .map((g) => ({ value: g.key, label: `${g.name} · ${g.key}` }))}
                    />
                    <NumberControl
                      label="监测项顺序"
                      value={item.order}
                      onChange={(order) => update({ order: order ?? 0 })}
                    />
                  </div>
                  <TextControl
                    label="监测项说明"
                    value={item.description}
                    onChange={(description) => update({ description })}
                  />
                  <CheckControl label="默认选用" value={item.enabled} onChange={(enabled) => update({ enabled })} />
                  {version==='2'&&<Section title="建议设备分类"><CategoryPicker suggestion scope={tenant?{kind:'tenant',tenantId:tenant.tenant.id}:{kind:'platform'}} value={item.suggested_category?{mode:'classify',dictionary_release_id:item.suggested_category.dictionary_release_id,category_id:item.suggested_category.category_id}:{mode:'unclassified'}} onChange={v=>update({suggested_category:v.mode==='classify'&&v.category_id&&v.dictionary_release_id?{dictionary_release_id:v.dictionary_release_id,category_id:v.category_id}:undefined})}/><p className="text-sm text-muted-foreground">建议分类保留所选字典版本，实际设备通过独立分类流程维护。</p></Section>}
                  <Section title="逐版本目标映射">
                    {types.filter(Boolean).map((id) => {
                      const def = definitions.find((v) => v.id === id);
                      if (version === '2') return <TargetEditor key={id} typeVersion={id} definition={def} value={item.targets.find((t) => t.type_version === id)} onChange={(target) => update({ targets: [...item.targets.filter((t) => t.type_version !== id), target] })} />;
                      return (
                        <SelectControl
                          key={id}
                          label={`${def?.content.name ?? '适用类型'} V${def?.number ?? ''} 的目标`}
                          value={item.targets.find((t) => t.type_version === id)?.component ?? ''}
                          onChange={(component) =>
                            update({
                              targets: [
                                ...item.targets.filter((t) => t.type_version !== id),
                                { type_version: id, component },
                              ],
                            })
                          }
                          options={[
                            { value: 'root', label: '设施根对象' },
                            ...(def?.content.components ?? []).map((c) => ({
                              value: c.key,
                              label: `${c.name} · ${c.key}`,
                            })),
                          ]}
                        />
                      );
                    })}
                  </Section>
                  <Section
                    title="指标与待绑定资源"
                    action={
                      <Button
                        type="button"
                        variant="outline"
                        onClick={() =>
                          update({
                            bindings: [
                              ...item.bindings,
                              {
                                metric_version: '',
                                unit_version: '',
                                resource_kind: 'observation_channel',
                                min_count: 1,
                              },
                            ],
                          })
                        }
                      >
                        添加绑定要求
                      </Button>
                    }
                  >
                    {item.bindings.map((b, n) => {
                      const change = (patch: Partial<typeof b>) =>
                        update({ bindings: item.bindings.map((v, j) => (n === j ? { ...v, ...patch } : v)) });
                      const metric = definitions.find((v) => v.id === b.metric_version)?.content.metric;
                      return (
                        <div key={n} className="space-y-3 rounded-lg border p-3">
                          <VersionPicker
                            kind="metrics"
                            label={`绑定指标 ${n + 1}`}
                            value={b.metric_version}
                            known={definitions}
                            onChange={(metric_version, v, dependencies = []) => {
                              if (v) onReferences?.([v, ...dependencies]);
                              if (v)
                                setPicked([
                                  ...picked.filter((p) => p.id !== v.id && !dependencies.some((d) => d.id === p.id)),
                                  v,
                                  ...dependencies,
                                ]);
                              change({ metric_version, unit_version: defaultMetricUnit(v, dependencies) });
                            }}
                          />
                          {metric?.shape !== 'enum' && (
                            <SelectControl label={`绑定单位 ${n + 1}`} value={b.unit_version} disabled={!metric}
                              options={metricUnitOptions(metric, definitions, b.unit_version)}
                              onChange={unit_version => change({ unit_version })} />
                          )}
                          <p className="text-sm">资源要求：观测通道（待后续绑定）</p>
                          <div className="grid gap-3 sm:grid-cols-2">
                            <NumberControl
                              label="最少资源数"
                              value={b.min_count}
                              onChange={(min_count) => change({ min_count: min_count ?? 0 })}
                            />
                            <NumberControl
                              label="最多资源数（可空）"
                              value={b.max_count}
                              onChange={(max_count) => change({ max_count })}
                            />
                          </div>
                          <Button
                            type="button"
                            variant="ghost"
                            onClick={() => update({ bindings: item.bindings.filter((_, j) => n !== j) })}
                          >
                            删除绑定要求 {n + 1}
                          </Button>
                        </div>
                      );
                    })}
                  </Section>
                </Section>
              );
            })}
          </Section>
        </>
      )}
    </div>
  );
}
