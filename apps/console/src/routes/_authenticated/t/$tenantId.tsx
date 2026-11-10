import { createFileRoute, Outlet } from '@tanstack/react-router';
import { TenantBoundary } from '../../../features/tenancy/context';
export const Route = createFileRoute('/_authenticated/t/$tenantId')({ component: function Page() { const { tenantId } = Route.useParams(); return <TenantBoundary key={tenantId} id={tenantId}><Outlet /></TenantBoundary>; } });
