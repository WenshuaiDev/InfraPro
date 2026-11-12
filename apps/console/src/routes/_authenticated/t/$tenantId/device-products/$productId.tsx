import {createFileRoute} from '@tanstack/react-router';
import {z} from '../../../../../config/validation';
import {DeviceProductPage} from '../../../../../features/device-catalog/detail';
export const Route=createFileRoute('/_authenticated/t/$tenantId/device-products/$productId')({validateSearch:z.object({source:z.enum(['public','private']).optional().catch(undefined),q:z.string().optional(),manufacturer:z.string().uuid().optional().catch(undefined),status:z.enum(['enabled','disabled','draft']).optional().catch(undefined)}),component:function Page(){const search=Route.useSearch();const {productId}=Route.useParams();return <DeviceProductPage id={productId} source={search.source}/>;}});
