import { createFileRoute } from '@tanstack/react-router';
import { ProjectResourceAccessPage } from '../../../../../../features/resource-access/pages';
export const Route=createFileRoute('/_authenticated/t/$tenantId/resource-access/$projectId/')({component:function Page(){const {projectId}=Route.useParams();return <ProjectResourceAccessPage project={projectId}/>;}});
