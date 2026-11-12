import { createContext, useContext, useEffect, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Permissions as P, type DeviceScope, type PermissionCode } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { useTenant } from '../tenancy/context';
import { useProject } from '../projects/context';
import { projectContextKey } from '../projects/scope';

type Range = { scope: DeviceScope; prefix: readonly unknown[]; available: boolean; can: (p:PermissionCode)=>boolean; canRead: boolean; canHistory:boolean; canManage:boolean; canConfigure:boolean; canInstall:boolean; canState:boolean; canSource:boolean;canSourceHistory:boolean; resourceRevision?:string; deviceRevision?:string };
const Context = createContext<Range|null>(null);
export function TenantDevices({children}:{children:ReactNode}) {
 const t=useTenant();const {account}=useIdentity();const can=(p:PermissionCode)=>t.permissions.includes(p);
 const range:Range={scope:{kind:'tenant',tenantId:t.tenant.id},prefix:['tenant',account?.id,t.tenant.id,t.membership_id,'devices',t.permissions.join(',')],available:t.available,can,canRead:can(P.TenantDeviceRead),canHistory:can(P.TenantDeviceHistoryRead),canManage:can(P.TenantDeviceManage),canConfigure:can(P.TenantDeviceOutputManage),canInstall:can(P.TenantDeviceInstall),canState:can(P.TenantDeviceState),canSource:can(P.TenantMonitoringSourceManage)&&can(P.TenantEngineeringResourceRead),canSourceHistory:can(P.TenantEngineeringResourceRead)};
 return <DeviceBoundary value={range}>{children}</DeviceBoundary>;
}
export function ProjectDevices({children}:{children:ReactNode}) {
 const p=useProject();const range:Range={scope:{kind:'project',tenantId:p.project.tenant_id,projectId:p.project.id},prefix:[...p.key,'devices',p.resource_access_revision,p.device_access_revision],resourceRevision:p.resource_access_revision,deviceRevision:p.device_access_revision,available:p.available&&p.project.status==='active',can:p.can,canRead:p.can(P.ProjectDeviceRead),canHistory:p.can(P.ProjectDeviceHistoryRead),canManage:false,canConfigure:false,canInstall:false,canState:false,canSource:p.can(P.ProjectMonitoringSourceManage)&&p.can(P.ProjectDeviceOutputUse)&&p.can(P.ProjectEngineeringResourceRead),canSourceHistory:p.can(P.ProjectEngineeringResourceRead)&&p.can(P.ProjectEngineeringResourceHistoryRead)};
 return <DeviceBoundary value={range}>{children}</DeviceBoundary>;
}
function DeviceBoundary({value,children}:{value:Range;children:ReactNode}){
 const {client,account}=useIdentity();const cache=useQueryClient();const tenant=value.scope.tenantId,project=value.scope.kind==='project'?value.scope.projectId:undefined;
 const stamp=JSON.stringify(value.prefix);
 useEffect(()=>client.onDeviceRestriction((t,p)=>{if(t!==tenant||p!==project)return;const prefix:unknown[]=JSON.parse(stamp);void cache.cancelQueries({queryKey:prefix});cache.removeQueries({queryKey:prefix});void cache.invalidateQueries({queryKey:project?projectContextKey(account?.id,tenant,project):['tenant',account?.id,tenant,'context']});}),[account?.id,cache,client,tenant,project,stamp]);
 useEffect(()=>{const prefix:unknown[]=JSON.parse(stamp);return ()=>{void cache.cancelQueries({queryKey:prefix});cache.removeQueries({queryKey:prefix});};},[cache,stamp]);
 useEffect(()=>()=>client.invalidateDeviceScope(tenant,project),[client,tenant,project]);
 return <Context value={value}><div key={stamp}>{children}</div></Context>;
}
// eslint-disable-next-line react-refresh/only-export-components
export function useDevices(){const range=useContext(Context);const identity=useIdentity();if(!range)throw new Error('Explicit device scope required');const {scope}=range;const base=scope.kind==='project'?`/t/${scope.tenantId}/p/${scope.projectId}`:`/t/${scope.tenantId}`;return {...identity,...range,paths:{list:`${base}/devices`,device:(id:string)=>`${base}/devices/${encodeURIComponent(id)}`,output:(id:string)=>`${base}/device-outputs/${encodeURIComponent(id)}`}};}
