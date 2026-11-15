import { createFileRoute } from '@tanstack/react-router';
import { ProjectMonitoring } from '../../../../../../../features/monitoring/context';
import { MonitoringImportsPage } from '../../../../../../../features/monitoring/imports-page';
import { importSearch } from '../../../../../../../features/monitoring/import-search';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/monitoring/imports')({ validateSearch: importSearch, component: function Page() { return <ProjectMonitoring><MonitoringImportsPage search={Route.useSearch()} /></ProjectMonitoring>; } });
