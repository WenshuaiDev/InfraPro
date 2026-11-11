import { createContext, useContext, useEffect, useMemo, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import { Permissions as P, type ResourceDetail, type ResourceScope } from '@infrapro/api-client';
import { useTenant } from '../tenancy/context';
import { useProject } from '../projects/context';
import { projectContextKey } from '../projects/scope';

type Range = { scope: ResourceScope; prefix: readonly unknown[]; available: boolean; canRead: boolean; canManage: boolean; canAdopt: boolean; canHistory: boolean; canCreateFacility: boolean; canGovern:boolean; accessRevision?: string };
const RangeContext = createContext<Range | null>(null);
export function TenantResources({children}: {children: ReactNode}) {
 const tenant = useTenant(); const {account} = useIdentity();
 const value: Range = {scope:{kind:'tenant',tenantId:tenant.tenant.id},prefix:['tenant',account?.id,tenant.tenant.id,tenant.membership_id,'resources'],available:tenant.available,canRead:tenant.permissions.includes(P.TenantEngineeringResourceRead),canManage:tenant.permissions.includes(P.TenantEngineeringResourceManage),canAdopt:tenant.permissions.includes(P.TenantEngineeringCatalogRead),canHistory:tenant.permissions.includes(P.TenantEngineeringResourceRead),canGovern:[P.TenantProjectRead,P.TenantEngineeringResourceRead,P.TenantEngineeringResourceAuthorizationRead].every(p=>tenant.permissions.includes(p)),canCreateFacility:tenant.permissions.includes(P.TenantEngineeringResourceManage)&&tenant.permissions.includes(P.TenantEngineeringCatalogRead)};
 return <ResourceBoundary value={value}>{children}</ResourceBoundary>;
}
export function ProjectResources({children}: {children: ReactNode}) {
 const project = useProject();
 const value: Range = {scope:{kind:'project',tenantId:project.project.tenant_id,projectId:project.project.id},prefix:[...project.key,'resources',project.resource_access_revision],accessRevision:project.resource_access_revision,available:project.available&&project.project.status==='active',canRead:project.can(P.ProjectEngineeringResourceRead),canManage:[P.ProjectEngineeringResourceUpdate,P.ProjectEngineeringFacilityCreate,P.ProjectEngineeringComponentCreate,P.ProjectEngineeringPointCreate,P.ProjectEngineeringChannelCreate,P.ProjectEngineeringResourceLifecycle].some(project.can),canAdopt:project.can(P.ProjectEngineeringCatalogRead),canHistory:project.can(P.ProjectEngineeringResourceHistoryRead),canGovern:false,canCreateFacility:project.can_create_facility};
 return <ResourceBoundary value={value}><div key={project.resource_access_revision}>{children}</div></ResourceBoundary>;
}
function ResourceBoundary({value,children}: {value: Range;children: ReactNode}) {
 const {account,client}=useIdentity();const cache=useQueryClient();const tenant=value.scope.tenantId;const project=value.scope.kind==='project'?value.scope.projectId:undefined;
 useEffect(()=>client.onResourceRestriction((tid,pid)=>{if(tid!==tenant||pid!==project)return;void cache.invalidateQueries({queryKey:project?projectContextKey(account?.id,tenant,project):['tenant',account?.id,tenant,'context']});}),[client,cache,account?.id,tenant,project]);
 // Clear only this resource scope on revision changes. Project members/roles remain mounted.
 const stamp=JSON.stringify(value.prefix);
 useEffect(()=>{const prefix:unknown[]=JSON.parse(stamp);return ()=>{const predicate=(q:{queryKey:readonly unknown[]})=>prefix.every((v,i)=>q.queryKey[i]===v);void cache.cancelQueries({predicate});cache.removeQueries({predicate});};},[cache,stamp]);
 useEffect(()=>()=>client.invalidateResourceScope(tenant,project),[client,tenant,project]);
 return <RangeContext value={value}>{children}</RangeContext>;
}
// eslint-disable-next-line react-refresh/only-export-components
export function useResources() {
 const identity=useIdentity();const range=useContext(RangeContext);if(!range)throw new Error('Explicit resource scope required');
 const {scope}=range;const base=scope.kind==='project'?`/t/${scope.tenantId}/p/${scope.projectId}/resources/facilities`:`/t/${scope.tenantId}/resources/facilities`;
 const paths=useMemo(()=>({list:base,facility:(id:string)=>`${base}/${encodeURIComponent(id)}`}),[base]);
 const writeMeta=(detail?:ResourceDetail)=>scope.kind==='project'?{expected_access_revision:range.accessRevision,...(detail?.link_revision?{expected_link_revision:detail.link_revision}:{})}:{};
 const verifyScope=()=>scope.kind==='project'?identity.client.projectContext(scope.tenantId,scope.projectId):identity.client.tenantContext(scope.tenantId);
 return {...identity,...range,tenantId:scope.tenantId,paths,writeMeta,verifyScope};
}
// eslint-disable-next-line react-refresh/only-export-components
export function refreshResources(cache: import('@tanstack/react-query').QueryClient, prefix: readonly unknown[], facility?: string) {
 return cache.invalidateQueries({ predicate: (query) => prefix.every((v,i) => query.queryKey[i] === v) && (query.queryKey[prefix.length] === 'facilities' || (query.queryKey[prefix.length] === 'detail' ? query.queryKey[prefix.length+2] === facility : query.queryKey[prefix.length+1] === facility)) });
}
