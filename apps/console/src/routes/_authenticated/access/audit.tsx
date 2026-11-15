import { auditSearch } from '../../../features/audit/search';
import { createFileRoute } from '@tanstack/react-router';
import { AuditPage } from '../../../features/access/audit-page';
export const Route = createFileRoute('/_authenticated/access/audit')({ validateSearch: auditSearch, component: AuditPage });
