import {createFileRoute} from '@tanstack/react-router';
import {ProjectMonitoring} from '../../../../../../../../features/monitoring/context';
import {MonitoringBatchPage} from '../../../../../../../../features/monitoring/batch-page';
export const Route=createFileRoute('/_authenticated/t/$tenantId/p/$projectId/monitoring/imports/$batchId')({component:function Page(){const {batchId}=Route.useParams();return <ProjectMonitoring><MonitoringBatchPage id={batchId}/></ProjectMonitoring>;}});
