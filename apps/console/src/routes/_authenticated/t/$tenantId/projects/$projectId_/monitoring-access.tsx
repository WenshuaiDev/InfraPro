import {createFileRoute} from '@tanstack/react-router';
import {TenantMonitoring} from '../../../../../../features/monitoring/context';
import {MonitoringAccessPage} from '../../../../../../features/monitoring/access-page';
export const Route=createFileRoute('/_authenticated/t/$tenantId/projects/$projectId_/monitoring-access')({component:function Page(){const {projectId}=Route.useParams();return <TenantMonitoring><MonitoringAccessPage project={projectId}/></TenantMonitoring>;}});
