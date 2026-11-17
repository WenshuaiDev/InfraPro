import { z } from '../../config/validation';
export const deviceListSearch=z.object({q:z.string().optional(),manufacturer:z.string().uuid().optional().catch(undefined),product:z.string().uuid().optional().catch(undefined),device:z.string().uuid().optional().catch(undefined),status:z.enum(['enabled','disabled']).optional().catch(undefined),installation:z.enum(['recorded','unrecorded']).optional().catch(undefined),source:z.enum(['all','public','private']).optional().catch(undefined),tab:z.enum(['devices','outputs']).optional().catch(undefined)});
export const deviceDetailSearch=z.object({tab:z.enum(['details','outputs','installations','changes','parsing']).optional().catch(undefined),output:z.string().uuid().optional().catch(undefined)});
export const tenantDeviceSearch = deviceListSearch.omit({ tab: true, device: true }).extend({
  ...deviceDetailSearch.shape,
  category:z.string().uuid().optional().catch(undefined), category_scope:z.enum(['exact','descendants']).optional().catch(undefined),category_group:z.enum(['sensor','non_sensor','unclassified','unavailable']).optional().catch(undefined),
  installation: z.enum(['recorded', 'unrecorded', 'location', 'resource']).optional().catch(undefined),
  facility: z.string().uuid().optional().catch(undefined),
});
export type TenantDeviceSearch = z.infer<typeof tenantDeviceSearch>;
