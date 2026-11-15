import { createFileRoute, Outlet } from '@tanstack/react-router';
import { ProjectRolesPage } from '../../../../../../features/projects/roles-page';
import { projectRoleSearch } from '../../../../../../features/projects/role-search';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/roles')({ validateSearch: projectRoleSearch, component: function ProjectRolesLayout() { return <><ProjectRolesPage /><Outlet /></>; } });
