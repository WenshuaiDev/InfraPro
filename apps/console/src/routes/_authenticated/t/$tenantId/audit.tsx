import { auditSearch } from '../../../../features/audit/search';
import { createFileRoute } from '@tanstack/react-router';
import { TenantAuditPage } from '../../../../features/tenancy/audit-page';
export const Route = createFileRoute('/_authenticated/t/$tenantId/audit')({ validateSearch: auditSearch, component: TenantAuditPage });
