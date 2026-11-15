import { createFileRoute } from '@tanstack/react-router';
import { ProjectRolePage } from '../../../../../../../features/projects/role-detail';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/roles/$roleId')({ component: function Page() { const { roleId } = Route.useParams(); return <ProjectRolePage key={roleId} role={roleId} />; } });
