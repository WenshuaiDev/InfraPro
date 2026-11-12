import {createFileRoute} from '@tanstack/react-router';
import {TenantMonitoring} from '../../../../../features/monitoring/context';
import {MonitoringDataPage} from '../../../../../features/monitoring/data-page';
import {monitoringSearch} from '../../../../../features/monitoring/search';
export const Route=createFileRoute('/_authenticated/t/$tenantId/monitoring/')({validateSearch:monitoringSearch,component:function Page(){const search=Route.useSearch();return <TenantMonitoring><MonitoringDataPage key={`${search.point}:${search.channel}`} initialPoint={search.point} initialChannel={search.channel}/></TenantMonitoring>;}});
