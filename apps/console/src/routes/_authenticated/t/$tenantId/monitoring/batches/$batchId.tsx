import {createFileRoute} from '@tanstack/react-router';
import {TenantMonitoring} from '../../../../../../features/monitoring/context';
import {MonitoringBatchPage} from '../../../../../../features/monitoring/batch-page';
export const Route=createFileRoute('/_authenticated/t/$tenantId/monitoring/batches/$batchId')({component:function Page(){const {batchId}=Route.useParams();return <TenantMonitoring><MonitoringBatchPage id={batchId}/></TenantMonitoring>;}});
