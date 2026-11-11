import { ProjectResources } from '../../../../../../../../features/resources/context';
import { createFileRoute } from '@tanstack/react-router';
import { FacilitiesPage } from '../../../../../../../../features/resources/list-page';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/resources/facilities/')({ component: () => <ProjectResources><FacilitiesPage/></ProjectResources> });
