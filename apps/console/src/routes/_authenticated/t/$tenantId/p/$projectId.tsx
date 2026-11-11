import { createFileRoute, Outlet } from '@tanstack/react-router';
import { ProjectBoundary } from '../../../../../features/projects/context';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId')({ component: function Page() { const {tenantId,projectId}=Route.useParams(); return <ProjectBoundary key={tenantId+':'+projectId} tenant={tenantId} project={projectId}><Outlet /></ProjectBoundary>; } });
