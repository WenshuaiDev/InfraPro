import {createFileRoute} from '@tanstack/react-router';
import {TenantDevices} from '../../../../../features/devices/context';
import {DevicesPage} from '../../../../../features/devices/list-page';
import {deviceListSearch} from '../../../../../features/devices/search';
export const Route=createFileRoute('/_authenticated/t/$tenantId/devices/')({validateSearch:deviceListSearch,component:function Page(){const search=Route.useSearch();const navigate=Route.useNavigate();return <TenantDevices><DevicesPage search={search} onSearch={search=>void navigate({search,replace:true})}/></TenantDevices>;}});
