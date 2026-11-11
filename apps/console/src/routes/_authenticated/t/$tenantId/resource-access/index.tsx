import { createFileRoute } from '@tanstack/react-router';
import { ResourceAccessProjects } from '../../../../../features/resource-access/pages';
export const Route=createFileRoute('/_authenticated/t/$tenantId/resource-access/')({validateSearch:(s:Record<string,unknown>)=>({facility:typeof s.facility==='string'&&/^[0-9a-f-]{36}$/i.test(s.facility)?s.facility:undefined}),component:function Page(){const {facility}=Route.useSearch();return <ResourceAccessProjects facility={facility}/>;}});
