import { createFileRoute, Navigate } from '@tanstack/react-router';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/')({ component: function Page(){ const params=Route.useParams();return <Navigate to="/t/$tenantId/p/$projectId/profile" params={params} replace />; } });
