import { createFileRoute } from '@tanstack/react-router';
import { ProjectRolesPage } from '../../../../../../../features/projects/roles-page';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/roles/')({ component: function Page() { return <ProjectRolesPage />; } });
