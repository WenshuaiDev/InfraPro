import { createFileRoute } from '@tanstack/react-router';
import { ProjectFiles } from '../../../../../../features/files/context';
import { FilesWorkspace } from '../../../../../../features/files/workspace';
import { filesSearch } from '../../../../../../features/files/state';
export const Route=createFileRoute('/_authenticated/t/$tenantId/p/$projectId/files')({validateSearch:filesSearch,component:function Page(){const search=Route.useSearch(),navigate=Route.useNavigate();return <ProjectFiles><FilesWorkspace search={search} onSearch={search=>void navigate({search,resetScroll:false})}/></ProjectFiles>;}});
