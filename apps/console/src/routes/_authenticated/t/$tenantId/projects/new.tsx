import { createFileRoute } from '@tanstack/react-router';
import { ProjectCreatePage } from '../../../../../features/projects/profile-editor';
export const Route = createFileRoute('/_authenticated/t/$tenantId/projects/new')({ component: function Page() { return <ProjectCreatePage />; } });
