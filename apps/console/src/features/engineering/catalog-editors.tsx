import { useQuery } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import type { EngineeringContent } from '@infrapro/api-client';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { useOptionalTenant } from '../tenancy/context';
import { Section, SelectControl, TextControl } from './controls';

function CategorySelect({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const { client, account } = useIdentity();
  const tenant = useOptionalTenant();
  const query = useQuery({
    queryKey: tenant
      ? ['tenant', account?.id, tenant.tenant.id, tenant.membership_id, 'engineering', 'categories']
      : ['platform', account?.id, 'engineering', 'categories'],
    queryFn: ({ signal }) => client.engineeringCategories(tenant?.tenant.id, signal),
  });
  return (
    <>
      <SelectControl
        label="单位类别"
        value={value}
        onChange={onChange}
        options={(query.data ?? []).map((v) => ({ value: v.key, label: `${v.name} · ${v.key}` }))}
      />
      {query.error && <FormNotice error>{query.error.message}</FormNotice>}
    </>
  );
}
export function UnitDefinitionEditor({ value, onChange }: { value: EngineeringContent; onChange: (v: EngineeringContent) => void }) {
  if (value.unit) {
    const unit = value.unit;
    return (
      <Section title="单位定义">
        <div className="grid gap-4 sm:grid-cols-2">
          <TextControl
            label="单位符号"
            value={unit.symbol}
            onChange={(symbol) => onChange({ ...value, unit: { ...unit, symbol } })}
          />
          <CategorySelect
            value={unit.category}
            onChange={(category) => onChange({ ...value, unit: { ...unit, category } })}
          />
        </div>
        <p className="text-sm text-muted-foreground">仅定义适用关系，本轮不进行数值单位换算。</p>
      </Section>
    );
  }
  return null;
}
