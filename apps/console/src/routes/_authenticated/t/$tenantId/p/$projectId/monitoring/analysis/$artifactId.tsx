import {createFileRoute} from '@tanstack/react-router';
import {ProjectMonitoring} from '../../../../../../../../features/monitoring/context';
import {AnalysisPage} from '../../../../../../../../features/derived-monitoring/analysis-page';
import {derivedSearch} from '../../../../../../../../features/derived-monitoring/search';
export const Route=createFileRoute('/_authenticated/t/$tenantId/p/$projectId/monitoring/analysis/$artifactId')({validateSearch:derivedSearch,component:function Page(){const search=Route.useSearch();return <ProjectMonitoring><AnalysisPage {...search} id={Route.useParams().artifactId} view="artifacts"/></ProjectMonitoring>;}});
