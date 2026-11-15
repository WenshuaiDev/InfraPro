import { createFileRoute } from '@tanstack/react-router';
import { TenantFiles } from '../../../../features/files/context';
import { FilesWorkspace } from '../../../../features/files/workspace';
import { filesSearch } from '../../../../features/files/state';
export const Route=createFileRoute('/_authenticated/t/$tenantId/files')({validateSearch:filesSearch,component:function Page(){const search=Route.useSearch(),navigate=Route.useNavigate();return <TenantFiles><FilesWorkspace search={search} onSearch={search=>void navigate({search,resetScroll:false})}/></TenantFiles>;}});
