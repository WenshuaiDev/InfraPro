import { ProjectResources } from '../../../../../../../../features/resources/context';
import { createFileRoute } from '@tanstack/react-router';
import { FacilityPage, type Selection } from '../../../../../../../../features/resources/facility-page';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/resources/facilities/$facilityId')({
  validateSearch: (search: Record<string, unknown>): Selection => ({
    component:
      typeof search.component === 'string' && /^[0-9a-f-]{36}$/i.test(search.component) ? search.component : undefined,
    point: typeof search.point === 'string' && /^[0-9a-f-]{36}$/i.test(search.point) ? search.point : undefined,
    channel: typeof search.channel === 'string' && /^[0-9a-f-]{36}$/i.test(search.channel) ? search.channel : undefined,
    tab:
      typeof search.tab === 'string' &&
      ['details', 'components', 'points', 'channels', 'usage', 'changes'].includes(search.tab)
        ? (search.tab as Selection['tab'])
        : undefined,
  }),
  component: function Page() {
    const { facilityId } = Route.useParams();
    const selection = Route.useSearch();
    const navigate = Route.useNavigate();
    return (
      <ProjectResources>
        <FacilityPage facilityId={facilityId} selection={selection} select={(search) => void navigate({ search })} />
      </ProjectResources>
    );
  },
});
