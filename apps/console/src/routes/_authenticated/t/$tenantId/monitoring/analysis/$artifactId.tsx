import {createFileRoute} from '@tanstack/react-router';
import {TenantMonitoring} from '../../../../../../features/monitoring/context';
import {AnalysisPage} from '../../../../../../features/derived-monitoring/analysis-page';
import {derivedSearch} from '../../../../../../features/derived-monitoring/search';
export const Route=createFileRoute('/_authenticated/t/$tenantId/monitoring/analysis/$artifactId')({validateSearch:derivedSearch,component:function Page(){const search=Route.useSearch();return <TenantMonitoring><AnalysisPage {...search} id={Route.useParams().artifactId} view="artifacts"/></TenantMonitoring>;}});
