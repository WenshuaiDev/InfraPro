import { createFileRoute } from '@tanstack/react-router';
import { ProjectGovernancePage } from '../../../../../features/projects/lists';
import { readGovernanceSearch } from '../../../../../features/projects/governance-search';
export const Route = createFileRoute('/_authenticated/t/$tenantId/projects/')({
  validateSearch: readGovernanceSearch,
  component: function Page() {
    const search = Route.useSearch(), navigate = Route.useNavigate();
    return <ProjectGovernancePage search={search} onSearch={next => void navigate({ search: next, replace: true })} />;
  },
});
