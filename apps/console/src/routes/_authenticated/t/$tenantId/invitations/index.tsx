import { createFileRoute } from '@tanstack/react-router';
import { TenantInvitationsPage } from '../../../../../features/tenancy/invitations-page';
export const Route = createFileRoute('/_authenticated/t/$tenantId/invitations/')({ component: TenantInvitationsPage });
