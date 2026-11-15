import { createFileRoute } from '@tanstack/react-router';
import { TenantMonitoring } from '../../../../../features/monitoring/context';
import { MonitoringImportsPage } from '../../../../../features/monitoring/imports-page';
import { importSearch } from '../../../../../features/monitoring/import-search';
export const Route = createFileRoute('/_authenticated/t/$tenantId/monitoring/imports')({ validateSearch: importSearch, component: function Page() { return <TenantMonitoring><MonitoringImportsPage search={Route.useSearch()} /></TenantMonitoring>; } });
