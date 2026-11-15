import { createFileRoute } from '@tanstack/react-router';
import { ProjectSpatial } from '../../../../../../features/spatial/context';
import { SpatialWorkspace } from '../../../../../../features/spatial/workspace';
import { spatialSearch } from '../../../../../../features/spatial/state';
export const Route=createFileRoute('/_authenticated/t/$tenantId/p/$projectId/spatial')({validateSearch:spatialSearch,component:function Page(){const search=Route.useSearch(),navigate=Route.useNavigate();return <ProjectSpatial><SpatialWorkspace search={search} facility={search.facility} onSearch={search=>void navigate({search,resetScroll:false})}/></ProjectSpatial>;}});
