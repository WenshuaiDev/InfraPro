import {createFileRoute} from '@tanstack/react-router';
import {TenantDevices} from '../../../../../features/devices/context';
import {DevicePage} from '../../../../../features/devices/detail-page';
import {deviceDetailSearch} from '../../../../../features/devices/search';
export const Route=createFileRoute('/_authenticated/t/$tenantId/devices/$deviceId')({validateSearch:deviceDetailSearch,component:function Page(){const search=Route.useSearch();const navigate=Route.useNavigate();const {deviceId}=Route.useParams();return <TenantDevices><DevicePage id={deviceId} search={search} onSearch={search=>void navigate({search,replace:true})}/></TenantDevices>;}});
