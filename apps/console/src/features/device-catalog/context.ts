import { useIdentity } from '@infrapro/api-client/react';
import { Permissions as P, type DeviceCatalogScope, type DeviceCatalogWriteScope } from '@infrapro/api-client';
import { useOptionalTenant } from '../tenancy/context';
export function useDeviceCatalog(source:'public'|'private'='private'){
 const identity=useIdentity();const tenant=useOptionalTenant();const scope:DeviceCatalogScope=tenant?{kind:'tenant',tenantId:tenant.tenant.id,source}:{kind:'platform'};
 const writeScope:DeviceCatalogWriteScope=tenant?{kind:'tenant',tenantId:tenant.tenant.id,source:'private'}:{kind:'platform'};const can=tenant?(p:Parameters<typeof identity.can>[0])=>tenant.permissions.includes(p):identity.can;
 return {...identity,tenant,scope,writeScope,base:tenant?`/t/${tenant.tenant.id}/device-products`:'/platform/device-products',prefix:tenant?['tenant',identity.account?.id,tenant.tenant.id,tenant.membership_id,'device-catalog',source,tenant.permissions.join(',')]:['platform',identity.account?.id,'device-catalog'],available:identity.phase==='authenticated'&&tenant?.available!==false,read:can(tenant?(source==='public'?P.TenantDeviceCatalogRead:P.TenantDeviceDefinitionRead):P.DeviceDefinitionRead),edit:can(tenant?P.TenantDeviceProductEdit:P.DeviceProductEdit),manufacturerManage:can(tenant?P.TenantDeviceManufacturerManage:P.DeviceManufacturerManage),publish:can(tenant?P.TenantDeviceProductPublish:P.DeviceProductPublish),state:can(tenant?P.TenantDeviceProductState:P.DeviceProductState),owned:!tenant||source==='private'};
}
