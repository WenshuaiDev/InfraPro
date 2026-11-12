import {createFileRoute} from '@tanstack/react-router';
import {TenantMonitoring} from '../../../../../../features/monitoring/context';
import {MonitoringImportsPage} from '../../../../../../features/monitoring/imports-page';
export const Route=createFileRoute('/_authenticated/t/$tenantId/monitoring/imports/')({component:function Page(){return <TenantMonitoring><MonitoringImportsPage/></TenantMonitoring>;}});
