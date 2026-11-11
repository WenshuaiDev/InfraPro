import { useIdentity } from '@infrapro/api-client/react';
import { Permissions as P } from '@infrapro/api-client';
import { useTenant } from '../tenancy/context';
export function useResources() {
 const identity = useIdentity(); const tenant = useTenant();
 return { ...identity, tenant, tenantId: tenant.tenant.id, prefix: ['tenant', identity.account?.id, tenant.tenant.id, tenant.membership_id, 'resources'], canRead: tenant.permissions.includes(P.TenantEngineeringResourceRead), canManage: tenant.available && tenant.permissions.includes(P.TenantEngineeringResourceManage) && tenant.permissions.includes(P.TenantEngineeringResourceRead), canAdopt: tenant.available && tenant.permissions.includes(P.TenantEngineeringCatalogRead) };
}
// Refresh only this facility and the facility list; unrelated open resources keep
// their cache and forms. Descendant actions can change when an ancestor retires.
export function refreshResources(cache: import('@tanstack/react-query').QueryClient, prefix: unknown[], facility?: string) {
 return cache.invalidateQueries({ predicate: (query) => prefix.every((v,i) => query.queryKey[i] === v) && (query.queryKey[prefix.length] === 'facilities' || (query.queryKey[prefix.length] === 'detail' ? query.queryKey[prefix.length+2] === facility : query.queryKey[prefix.length+1] === facility)) });
}
