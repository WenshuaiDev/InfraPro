import { createFileRoute } from '@tanstack/react-router';
import { MonitoringBatchPage } from '../../../../../../../../features/monitoring/batch-page';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/monitoring/imports/$batchId')({ component: function Page() { const { batchId } = Route.useParams(); return <MonitoringBatchPage id={batchId} search={Route.useSearch()} />; } });
