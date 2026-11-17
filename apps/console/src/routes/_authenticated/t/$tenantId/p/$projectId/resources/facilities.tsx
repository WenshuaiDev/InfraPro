import { createFileRoute, useParams } from '@tanstack/react-router';
import { ProjectResources } from '../../../../../../../features/resources/context';
import { FacilityWorkspace } from '../../../../../../../features/resources/workspace';
import { facilityWorkspaceSearch, type FacilityWorkspaceSearch } from '../../../../../../../features/resources/workspace-search';

export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/resources/facilities')({
  validateSearch: (raw): FacilityWorkspaceSearch => ({ ...facilityWorkspaceSearch(raw), deleted: undefined }),
  component: function Page() {
    const { facilityId } = useParams({ strict: false });
    const search = Route.useSearch();
    const navigate = Route.useNavigate();
    return <ProjectResources><FacilityWorkspace facilityId={facilityId} search={search} navigate={(id, next, replace) => {
      if (id) void navigate({ to: '/t/$tenantId/p/$projectId/resources/facilities/$facilityId', params: { facilityId: id }, search: next, resetScroll: false, replace });
      else void navigate({ to: '/t/$tenantId/p/$projectId/resources/facilities', search: next, resetScroll: false, replace });
    }} /></ProjectResources>;
  },
});
