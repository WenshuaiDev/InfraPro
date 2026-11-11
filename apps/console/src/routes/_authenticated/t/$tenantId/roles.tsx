import { createFileRoute, Outlet } from '@tanstack/react-router';
import { TenantRolesPage } from '../../../../features/tenancy/roles-page';

export const Route = createFileRoute('/_authenticated/t/$tenantId/roles')({ component: function TenantRolesLayout() { return <><TenantRolesPage /><Outlet /></>; } });
