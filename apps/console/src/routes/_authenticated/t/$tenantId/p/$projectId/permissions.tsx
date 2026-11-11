import { createFileRoute } from '@tanstack/react-router';
import { ProjectPermissionsPage } from '../../../../../../features/projects/pages';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/permissions')({ component: function Page() { return <ProjectPermissionsPage />; } });
