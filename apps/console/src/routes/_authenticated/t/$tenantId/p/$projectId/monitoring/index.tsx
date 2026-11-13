import { createFileRoute } from '@tanstack/react-router';
import { ProjectMonitoring } from '../../../../../../../features/monitoring/context';
import { MonitoringDataPage } from '../../../../../../../features/monitoring/data-page';
import { monitoringSearch } from '../../../../../../../features/monitoring/search';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/monitoring/')({
  validateSearch: monitoringSearch,
  component: function Page() {
    const search = Route.useSearch(), navigate = Route.useNavigate();
    return <ProjectMonitoring><MonitoringDataPage initialPoint={search.point} initialChannel={search.channel} onSelect={(point, channel) => void navigate({ search: { point, channel }, resetScroll: false })} /></ProjectMonitoring>;
  },
});
