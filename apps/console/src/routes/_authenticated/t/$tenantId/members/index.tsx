import { createFileRoute } from '@tanstack/react-router';
import { TenantMembersPage } from '../../../../../features/tenancy/members-page';
export const Route = createFileRoute('/_authenticated/t/$tenantId/members/')({ component: TenantMembersPage });
