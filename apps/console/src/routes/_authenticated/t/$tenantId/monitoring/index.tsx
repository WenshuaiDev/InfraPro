import { createFileRoute } from '@tanstack/react-router';
import { TenantMonitoring } from '../../../../../features/monitoring/context';
import { MonitoringDataPage } from '../../../../../features/monitoring/data-page';
import { monitoringSearch } from '../../../../../features/monitoring/search';
export const Route = createFileRoute('/_authenticated/t/$tenantId/monitoring/')({
  validateSearch: monitoringSearch,
  component: function Page() {
    const search = Route.useSearch(), navigate = Route.useNavigate();
    return <TenantMonitoring><MonitoringDataPage initialPoint={search.point} initialChannel={search.channel} onSelect={(point, channel) => void navigate({ search: { point, channel }, resetScroll: false })} /></TenantMonitoring>;
  },
});
