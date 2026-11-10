import { createFileRoute } from '@tanstack/react-router';
import { TenantAuditPage } from '../../../../features/tenancy/pages';
export const Route = createFileRoute('/_authenticated/t/$tenantId/audit')({ component: TenantAuditPage });
