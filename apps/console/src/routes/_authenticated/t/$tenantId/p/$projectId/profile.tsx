import { createFileRoute } from '@tanstack/react-router';
import { ProjectProfilePage } from '../../../../../../features/projects/pages';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/profile')({ component: function Page() { return <ProjectProfilePage />; } });
