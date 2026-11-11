import { createFileRoute } from '@tanstack/react-router';
import { TenantInvitationPage } from '../../../../../features/tenancy/invitations-page';
export const Route = createFileRoute('/_authenticated/t/$tenantId/invitations/$invitationId')({ component: function Page() { const { invitationId } = Route.useParams(); return <TenantInvitationPage key={invitationId} id={invitationId} />; } });
