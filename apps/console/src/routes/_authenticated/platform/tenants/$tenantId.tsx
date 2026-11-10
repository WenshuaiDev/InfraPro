import { createFileRoute } from '@tanstack/react-router';
import { PlatformTenantPage } from '../../../../features/tenancy/pages';
export const Route = createFileRoute('/_authenticated/platform/tenants/$tenantId')({ component: function Page() { const { tenantId } = Route.useParams(); return <PlatformTenantPage key={tenantId} id={tenantId} />; } });
