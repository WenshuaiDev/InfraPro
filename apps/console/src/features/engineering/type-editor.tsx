import type { EngineeringContent, EngineeringVersion } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { FieldsEditor } from './fields-editor';
import { NumberControl, Section, SelectControl, TextControl } from './controls';
export function TypeEditor({
  value,
  onChange,
  known,
  section,
  onReferences,
}: {
  value: EngineeringContent;
  onChange: (v: EngineeringContent) => void;
  known: EngineeringVersion[];
  section?: 'fields' | 'components';
  onReferences?: (versions: EngineeringVersion[]) => void;
}) {
  const components = value.components ?? [];
  const relations = value.relations ?? [];
  const options = [
    { value: 'root', label: '设施根对象' },
    ...components.filter((v) => v.key).map((v) => ({ value: v.key, label: `${v.name} · ${v.key}` })),
  ];
  return (
    <div className="space-y-5">
      {section !== 'components' && (
        <Section title="设施属性" id="/fields">
          <FieldsEditor
            onReferences={onReferences}
            value={value.fields ?? []}
            onChange={(fields) => onChange({ ...value, fields })}
            path="/fields"
            known={known}
          />
        </Section>
      )}
      {section !== 'fields' && (
        <>
          <Section
            title="构件类别"
            id="/components"
            action={
              <Button
                type="button"
                variant="outline"
                onClick={() =>
                  onChange({
                    ...value,
                    components: [...components, { key: '', name: '', description: '', fields: [] }],
                  })
                }
              >
                添加构件类别
              </Button>
            }
          >
            {components.map((v, i) => {
              const update = (patch: Partial<typeof v>) =>
                onChange({ ...value, components: components.map((c, j) => (i === j ? { ...c, ...patch } : c)) });
              return (
                <Section
                  key={i}
                  id={`/components/${i}`}
                  title={`构件类别 ${i + 1}`}
                  action={
                    <Button
                      type="button"
                      variant="ghost"
                      onClick={() => onChange({ ...value, components: components.filter((_, j) => j !== i) })}
                    >
                      删除类别 {i + 1}
                    </Button>
                  }
                >
                  <div className="grid gap-4 sm:grid-cols-2">
                    <TextControl
                      id={`/components/${i}/key`}
                      label="类别键"
                      value={v.key}
                      onChange={(key) => update({ key })}
                    />
                    <TextControl
                      id={`/components/${i}/name`}
                      label="类别名称"
                      value={v.name}
                      onChange={(name) => update({ name })}
                    />
                  </div>
                  <TextControl
                    label="类别说明"
                    value={v.description}
                    onChange={(description) => update({ description })}
                  />
                  <FieldsEditor
                    onReferences={onReferences}
                    value={v.fields ?? []}
                    onChange={(fields) => update({ fields })}
                    path={`/components/${i}/fields`}
                    known={known}
                  />
                </Section>
              );
            })}
          </Section>
          <Section
            title="组成关系"
            id="/relations"
            action={
              <Button
                type="button"
                variant="outline"
                onClick={() =>
                  onChange({
                    ...value,
                    relations: [...relations, { parent: 'root', child: '', min_count: 0, order: 0 }],
                  })
                }
              >
                添加组成关系
              </Button>
            }
          >
            <p className="text-sm text-muted-foreground">
              按每个父实例约束子构件数量；类别关系必须无环，所有类别均从根对象可达。
            </p>
            {relations.map((v, i) => {
              const update = (patch: Partial<typeof v>) =>
                onChange({ ...value, relations: relations.map((r, j) => (i === j ? { ...r, ...patch } : r)) });
              return (
                <div key={i} id={`/relations/${i}`} className="space-y-3 rounded-lg border p-3">
                  <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                    <SelectControl
                      label="父级类别"
                      value={v.parent}
                      options={options}
                      onChange={(parent) => update({ parent })}
                    />
                    <SelectControl
                      label="子级类别"
                      value={v.child}
                      options={options.slice(1)}
                      onChange={(child) => update({ child })}
                    />
                    <NumberControl
                      label="最少构件数"
                      value={v.min_count}
                      onChange={(min_count) => update({ min_count: min_count ?? 0 })}
                    />
                    <NumberControl
                      label="最多构件数（可空）"
                      value={v.max_count}
                      onChange={(max_count) => update({ max_count })}
                    />
                    <NumberControl
                      label="关系顺序"
                      value={v.order}
                      onChange={(order) => update({ order: order ?? 0 })}
                    />
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    onClick={() => onChange({ ...value, relations: relations.filter((_, j) => i !== j) })}
                  >
                    删除关系 {i + 1}
                  </Button>
                </div>
              );
            })}
          </Section>
        </>
      )}
    </div>
  );
}
