import { createFileRoute } from '@tanstack/react-router';
import { TenantRolePage } from '../../../../../features/tenancy/roles-page';
export const Route = createFileRoute('/_authenticated/t/$tenantId/roles/$roleId')({ component: function Page() { const { roleId } = Route.useParams(); return <TenantRolePage key={roleId} id={roleId} />; } });
