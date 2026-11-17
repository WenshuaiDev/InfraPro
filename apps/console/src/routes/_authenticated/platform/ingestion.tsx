import {createFileRoute} from '@tanstack/react-router';
import {z} from '../../../config/validation';
import {IngestionPage} from '../../../features/ingestion/page';
export const Route=createFileRoute('/_authenticated/platform/ingestion')({validateSearch:z.object({source:z.string().uuid().optional().catch(undefined),terminal:z.string().uuid().optional().catch(undefined),message:z.string().uuid().optional().catch(undefined),device:z.string().uuid().optional().catch(undefined),recovery:z.string().uuid().optional().catch(undefined),generation:z.preprocess(v=>typeof v==='number'&&Number.isSafeInteger(v)?String(v):v,z.string().regex(/^[1-9][0-9]*$/).optional()),tab:z.string().optional().catch(undefined)}),component:function Page(){return <IngestionPage {...Route.useSearch()}/>;}});
