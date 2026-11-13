import {createFileRoute} from '@tanstack/react-router';
import {TenantMonitoring} from '../../../../../../features/monitoring/context';
import {DerivedChannelsPage} from '../../../../../../features/derived-monitoring/channels-page';
import {derivedSearch} from '../../../../../../features/derived-monitoring/search';
export const Route=createFileRoute('/_authenticated/t/$tenantId/monitoring/derived/$channelId')({validateSearch:derivedSearch,component:function Page(){const search=Route.useSearch();return <TenantMonitoring><DerivedChannelsPage {...search} id={Route.useParams().channelId}/></TenantMonitoring>;}});
