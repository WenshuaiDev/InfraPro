import { createFileRoute } from '@tanstack/react-router';
import { MyTenantInvitation } from '../../../../features/notifications/invitations';
export const Route = createFileRoute('/_authenticated/notifications/invitations/$invitationId')({ component: function Page() { return <MyTenantInvitation key={Route.useParams().invitationId} id={Route.useParams().invitationId} />; } });
