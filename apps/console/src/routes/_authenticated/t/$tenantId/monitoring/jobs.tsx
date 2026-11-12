import {createFileRoute} from '@tanstack/react-router';
import {TenantMonitoring} from '../../../../../features/monitoring/context';
import {MonitoringJobsPage} from '../../../../../features/monitoring/jobs-page';
export const Route=createFileRoute('/_authenticated/t/$tenantId/monitoring/jobs')({component:function Page(){return <TenantMonitoring><MonitoringJobsPage/></TenantMonitoring>;}});
