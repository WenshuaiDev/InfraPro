import {createFileRoute} from '@tanstack/react-router';
import {z} from '../../../../config/validation';
import {ParsingPage} from '../../../../features/parsing/page';
export const Route=createFileRoute('/_authenticated/t/$tenantId/parsing')({validateSearch:z.object({run:z.string().uuid().optional().catch(undefined),generation:z.string().regex(/^[1-9][0-9]*$/).optional().catch(undefined)}),component:function Page(){const search=Route.useSearch();return <ParsingPage {...search}/>;}});
