import {createFileRoute} from '@tanstack/react-router';
import {ProjectMonitoring} from '../../../../../../../../features/monitoring/context';
import {DerivedWorkspacePage} from '../../../../../../../../features/derived-monitoring/workspace-page';
import {derivedSearch} from '../../../../../../../../features/derived-monitoring/search';
export const Route=createFileRoute('/_authenticated/t/$tenantId/p/$projectId/monitoring/derived/$channelId')({validateSearch:derivedSearch,component:function Page(){const search=Route.useSearch();return <ProjectMonitoring><DerivedWorkspacePage {...search} id={Route.useParams().channelId} tab="channels"/></ProjectMonitoring>;}});
