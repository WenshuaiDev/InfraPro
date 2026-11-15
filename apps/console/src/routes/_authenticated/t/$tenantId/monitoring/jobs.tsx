import {createFileRoute} from '@tanstack/react-router';
import {TenantMonitoring} from '../../../../../features/monitoring/context';
import {MonitoringJobsPage} from '../../../../../features/monitoring/jobs-page';
export const Route=createFileRoute('/_authenticated/t/$tenantId/monitoring/jobs')({validateSearch:(s:Record<string,unknown>)=>({job:typeof s.job==='string'&&/^[0-9a-f-]{36}$/i.test(s.job)?s.job:undefined}),component:function Page(){return <TenantMonitoring><MonitoringJobsPage key={Route.useSearch().job} jobID={Route.useSearch().job}/></TenantMonitoring>;}});
