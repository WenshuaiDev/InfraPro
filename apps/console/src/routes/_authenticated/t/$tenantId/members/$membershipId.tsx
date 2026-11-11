import { createFileRoute } from '@tanstack/react-router';
import { TenantMemberPage } from '../../../../../features/tenancy/members-page';
export const Route = createFileRoute('/_authenticated/t/$tenantId/members/$membershipId')({ component: function Page() { const { membershipId } = Route.useParams(); return <TenantMemberPage key={membershipId} id={membershipId} />; } });
