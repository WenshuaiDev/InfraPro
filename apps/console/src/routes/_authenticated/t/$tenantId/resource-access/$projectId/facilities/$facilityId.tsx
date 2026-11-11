import { createFileRoute } from '@tanstack/react-router';
import { FacilityResourceAccessPage } from '../../../../../../../features/resource-access/editor';
export const Route=createFileRoute('/_authenticated/t/$tenantId/resource-access/$projectId/facilities/$facilityId')({component:function Page(){const {projectId,facilityId}=Route.useParams();return <FacilityResourceAccessPage project={projectId} facility={facilityId}/>;}});
