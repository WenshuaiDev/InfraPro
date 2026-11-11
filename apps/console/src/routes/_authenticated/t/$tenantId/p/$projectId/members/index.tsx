import { createFileRoute } from '@tanstack/react-router';
import { ProjectMembersPage } from '../../../../../../../features/projects/pages';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/members/')({ component: function Page() { return <ProjectMembersPage />; } });
