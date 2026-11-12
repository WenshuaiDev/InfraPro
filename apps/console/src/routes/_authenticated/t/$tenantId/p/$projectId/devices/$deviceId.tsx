import {createFileRoute} from '@tanstack/react-router';
import {ProjectDevices} from '../../../../../../../features/devices/context';
import {DevicePage} from '../../../../../../../features/devices/detail-page';
import {deviceDetailSearch} from '../../../../../../../features/devices/search';
export const Route=createFileRoute('/_authenticated/t/$tenantId/p/$projectId/devices/$deviceId')({validateSearch:deviceDetailSearch,component:function Page(){const search=Route.useSearch();const navigate=Route.useNavigate();const {deviceId}=Route.useParams();return <ProjectDevices><DevicePage id={deviceId} search={search} onSearch={search=>void navigate({search,replace:true})}/></ProjectDevices>;}});
