import { createFileRoute } from '@tanstack/react-router';
import { TenantSpatial } from '../../../../features/spatial/context';
import { SpatialWorkspace } from '../../../../features/spatial/workspace';
import { spatialSearch } from '../../../../features/spatial/state';
export const Route=createFileRoute('/_authenticated/t/$tenantId/spatial')({validateSearch:spatialSearch,component:function Page(){const search=Route.useSearch(),navigate=Route.useNavigate();return <TenantSpatial><SpatialWorkspace search={search} facility={search.facility} onSearch={search=>void navigate({search,resetScroll:false})}/></TenantSpatial>;}});
