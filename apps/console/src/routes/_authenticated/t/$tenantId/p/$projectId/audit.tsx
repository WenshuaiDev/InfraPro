import { auditSearch } from '../../../../../../features/audit/search';
import { createFileRoute } from '@tanstack/react-router';
import { ProjectAuditPage } from '../../../../../../features/projects/pages';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/audit')({ validateSearch: auditSearch, component: function Page() { return <ProjectAuditPage />; } });
