import {createFileRoute} from '@tanstack/react-router';
import {z} from '../../../../../config/validation';
import {DeviceCatalogPage} from '../../../../../features/device-catalog/page';
export const Route=createFileRoute('/_authenticated/t/$tenantId/device-products/')({validateSearch:z.object({source:z.enum(['public','private']).optional().catch(undefined),q:z.string().optional(),manufacturer:z.string().uuid().optional().catch(undefined),status:z.enum(['enabled','disabled','draft']).optional().catch(undefined)}),component:function Page(){const search=Route.useSearch();const navigate=Route.useNavigate();return <DeviceCatalogPage search={search} onSearch={search=>void navigate({search,replace:true})}/>;}});
