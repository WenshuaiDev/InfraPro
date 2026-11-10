import { createFileRoute } from '@tanstack/react-router';
import { TenantProfilePage } from '../../../../features/tenancy/pages';
export const Route = createFileRoute('/_authenticated/t/$tenantId/profile')({ component: TenantProfilePage });
