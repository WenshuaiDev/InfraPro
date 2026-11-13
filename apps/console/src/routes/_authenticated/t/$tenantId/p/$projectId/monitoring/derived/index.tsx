import {createFileRoute} from '@tanstack/react-router';
import {ProjectMonitoring} from '../../../../../../../../features/monitoring/context';
import {DerivedChannelsPage} from '../../../../../../../../features/derived-monitoring/channels-page';
import {derivedSearch} from '../../../../../../../../features/derived-monitoring/search';
export const Route=createFileRoute('/_authenticated/t/$tenantId/p/$projectId/monitoring/derived/')({validateSearch:derivedSearch,component:function Page(){const search=Route.useSearch();return <ProjectMonitoring><DerivedChannelsPage {...search}/></ProjectMonitoring>;}});
