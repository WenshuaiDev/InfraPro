import { createFileRoute } from '@tanstack/react-router';
import { Button } from '@infrapro/ui/components/button';
import { EmptyState } from '@infrapro/ui/patterns/feedback';
import { useIdentity } from '@infrapro/api-client/react';
import { EngineeringPage, type FacilityPanel } from '../../../../features/engineering/page';
import { unitCatalogSearch, type UnitCatalogSearch } from '../../../../features/engineering/unit-catalog/search';

export const Route = createFileRoute('/_authenticated/platform/engineering/$kind')({
  validateSearch: (search: Record<string, unknown>): UnitCatalogSearch & { definition?: string; metricPage?: number; metricSize?: number; variant?: 'A' | 'B' | 'C'; panel?: FacilityPanel } => {
    const size = Number(search.metricSize), page = Number(search.metricPage);
    const metricSize = [10, 20, 50, 100].includes(size) ? size : undefined;
    return {
      ...unitCatalogSearch(search),
      metricSize,
      metricPage: Number.isInteger(page) && page >= 1 && (page - 1) * (metricSize ?? 20) <= 100000 ? page : undefined,
      definition: typeof search.definition === 'string' && /^[0-9a-f-]{36}$/i.test(search.definition) ? search.definition : undefined,
      panel: (search.panel === 'structure' || search.panel === 'versions') ? search.panel : undefined,
      variant: import.meta.env.DEV && (search.variant === 'A' || search.variant === 'B' || search.variant === 'C') ? search.variant : undefined,
    };
  },
  component: function Page() {
    const { help } = useIdentity();
    const { kind } = Route.useParams();
    const { definition, variant, panel } = Route.useSearch();
    const navigate = Route.useNavigate();
    if (import.meta.env.DEV && kind === 'units' && variant) return <EmptyState title="单位目录原型已归档">
      <Button variant="outline" onClick={() => void navigate({ search: {}, replace: true })}>打开正式单位目录</Button>
    </EmptyState>;
    if (import.meta.env.DEV && kind === 'type-definitions' && import.meta.env.VITE_FACILITY_TYPE_PROTOTYPE === 'B') {
      const base = help?.console_url;
      const target = base ? new URL('platform/engineering/type-definitions', base.endsWith('/') ? base : base + '/') : undefined;
      if (definition) target?.searchParams.set('definition', definition);
      if (panel) target?.searchParams.set('panel', panel);
      return <EmptyState title="方案 B 已用于设施类型详情">
        {target && <Button asChild><a href={target.href}>打开设施类型详情</a></Button>}
      </EmptyState>;
    }
    return <EngineeringPage
      kind={kind}
      selected={definition}
      onSelect={(id) => void navigate({ search: previous => ({ ...previous, definition: id, panel: undefined }) })}
      initialPanel={panel}
    />;
  },
});
