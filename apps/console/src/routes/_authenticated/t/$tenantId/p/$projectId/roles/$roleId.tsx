import { createFileRoute } from '@tanstack/react-router';
import { ProjectRolePage } from '../../../../../../../features/projects/roles-page';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/roles/$roleId')({ component: function Page() { const { roleId } = Route.useParams(); return <ProjectRolePage role={roleId} />; } });
