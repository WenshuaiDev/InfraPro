import {createFileRoute} from '@tanstack/react-router';
import {TenantDevices} from '../../../../../../features/devices/context';
import {DeviceAccessPage} from '../../../../../../features/device-access/page';
export const Route=createFileRoute('/_authenticated/t/$tenantId/projects/$projectId_/device-access')({component:function Page(){const {projectId}=Route.useParams();return <TenantDevices><DeviceAccessPage project={projectId}/></TenantDevices>;}});
