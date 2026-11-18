import {createFileRoute} from '@tanstack/react-router';
import {TenantMonitoring} from '../../../../../features/monitoring/context';
import {DerivedRunsPage} from '../../../../../features/derived-monitoring/runs-page';
import {derivedSearch} from '../../../../../features/derived-monitoring/search';
export const Route=createFileRoute('/_authenticated/t/$tenantId/monitoring/derived-runs')({validateSearch:derivedSearch,component:function Page(){const search=Route.useSearch();return <TenantMonitoring><DerivedRunsPage {...search}/></TenantMonitoring>;}});
