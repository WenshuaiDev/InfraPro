import {createFileRoute} from '@tanstack/react-router';
import {ProjectDevices} from '../../../../../../../features/devices/context';
import {ProjectOutputPage} from '../../../../../../../features/devices/detail-page';
export const Route=createFileRoute('/_authenticated/t/$tenantId/p/$projectId/device-outputs/$outputId')({component:function Page(){const {outputId}=Route.useParams();return <ProjectDevices><ProjectOutputPage id={outputId}/></ProjectDevices>;}});
