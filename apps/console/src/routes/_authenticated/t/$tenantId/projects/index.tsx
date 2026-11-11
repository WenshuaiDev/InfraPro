import { createFileRoute } from '@tanstack/react-router';
import { ProjectGovernancePage } from '../../../../../features/projects/lists';
export const Route = createFileRoute('/_authenticated/t/$tenantId/projects/')({ component: function Page() { return <ProjectGovernancePage />; } });
