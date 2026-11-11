import { createFileRoute } from '@tanstack/react-router';
import { ProjectMemberPage } from '../../../../../../../features/projects/pages';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/members/$membershipId')({ component: function Page() { const { membershipId } = Route.useParams(); return <ProjectMemberPage member={membershipId} />; } });
