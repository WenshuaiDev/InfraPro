import { createFileRoute } from '@tanstack/react-router';
import { ProjectGovernanceDetail } from '../../../../../features/projects/lists';
export const Route = createFileRoute('/_authenticated/t/$tenantId/projects/$projectId')({ component: function Page() { const { projectId } = Route.useParams(); return <ProjectGovernanceDetail project={projectId} />; } });
