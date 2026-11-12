import {createFileRoute} from '@tanstack/react-router';
import {ProjectMonitoring} from '../../../../../../../../features/monitoring/context';
import {MonitoringImportsPage} from '../../../../../../../../features/monitoring/imports-page';
export const Route=createFileRoute('/_authenticated/t/$tenantId/p/$projectId/monitoring/imports/')({component:function Page(){return <ProjectMonitoring><MonitoringImportsPage/></ProjectMonitoring>;}});
