import {createFileRoute} from '@tanstack/react-router';
import {TenantMonitoring} from '../../../../../features/monitoring/context';
import {DerivedDefinitionsPage} from '../../../../../features/derived-monitoring/definitions-page';
import {derivedSearch} from '../../../../../features/derived-monitoring/search';
export const Route=createFileRoute('/_authenticated/t/$tenantId/monitoring/derived-definitions')({validateSearch:derivedSearch,component:function Page(){const search=Route.useSearch();return <TenantMonitoring><DerivedDefinitionsPage id={search.id}/></TenantMonitoring>;}});
