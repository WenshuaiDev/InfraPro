import {createFileRoute} from '@tanstack/react-router';
import {ProjectMonitoring} from '../../../../../../../features/monitoring/context';
import {MonitoringJobsPage} from '../../../../../../../features/monitoring/jobs-page';
export const Route=createFileRoute('/_authenticated/t/$tenantId/p/$projectId/monitoring/jobs')({component:function Page(){return <ProjectMonitoring><MonitoringJobsPage/></ProjectMonitoring>;}});
