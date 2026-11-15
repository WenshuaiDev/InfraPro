import { createFileRoute } from '@tanstack/react-router';
import { MyTenantInvitations } from '../../../../features/notifications/invitations';
export const Route = createFileRoute('/_authenticated/notifications/invitations/')({ component: MyTenantInvitations });
