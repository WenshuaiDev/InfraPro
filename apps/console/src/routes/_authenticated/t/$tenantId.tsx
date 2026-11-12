import { createFileRoute, Outlet, useChildMatches } from '@tanstack/react-router';
import { TenantBoundary } from '../../../features/tenancy/context';
export const Route = createFileRoute('/_authenticated/t/$tenantId')({ component: function Page() {
  const { tenantId } = Route.useParams();
  const project = useChildMatches({ select: (matches) => matches.some((match) => match.routeId === '/_authenticated/t/$tenantId/p/$projectId') });
  // ProjectBoundary validates its own membership and ancestor lifecycle. Tenant
  // business entry is independent and must not gate an authorized project.
  if (project) return <Outlet />;
  return <TenantBoundary key={tenantId} id={tenantId}><Outlet /></TenantBoundary>;
} });
