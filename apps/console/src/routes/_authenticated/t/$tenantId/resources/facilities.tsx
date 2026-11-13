import { createFileRoute, useParams } from '@tanstack/react-router';
import { TenantResources } from '../../../../../features/resources/context';
import { FacilityWorkspace } from '../../../../../features/resources/workspace';
import { facilityWorkspaceSearch } from '../../../../../features/resources/workspace-search';

export const Route = createFileRoute('/_authenticated/t/$tenantId/resources/facilities')({
  validateSearch: facilityWorkspaceSearch,
  component: function Page() {
    const { facilityId } = useParams({ strict: false });
    const search = Route.useSearch();
    const navigate = Route.useNavigate();
    return <TenantResources><FacilityWorkspace facilityId={facilityId} search={search} navigate={(id, next, replace) => {
      if (id) void navigate({ to: '/t/$tenantId/resources/facilities/$facilityId', params: { facilityId: id }, search: next, resetScroll: false, replace });
      else void navigate({ to: '/t/$tenantId/resources/facilities', search: next, resetScroll: false, replace });
    }} /></TenantResources>;
  },
});
