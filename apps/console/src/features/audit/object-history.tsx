import type { ComponentProps, ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { useIdentity } from '@infrapro/api-client/react';
import { Permissions as P,type AuditScope,type AuditFilter } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { useOptionalTenant } from '../tenancy/context';
import { useOptionalProject } from '../projects/context';
import { auditPath } from './search';
// Finite application vocabulary, never a database table or arbitrary URL.
export type AuditObjectType='account'|'role'|'tenant'|'tenant_member'|'tenant_role'|'tenant_invitation'|'project'|'project_member'|'project_role'|'engineering_definition'|'facility'|'component'|'monitoring_section'|'borehole'|'point'|'channel'|'manufacturer'|'product'|'product_version'|'device'|'device_output'|'device_access'|'resource_access'|'monitoring_batch'|'monitoring_job'|'derived_channel'|'derived_run'|'analysis_artifact'|'spatial_asset'|'spatial_version'|'display_instance'|'display_package'|'file_attachment'|'file_upload'|'file_revision';
export function ObjectHistory({objectType,objectId,scope:given,filter={},label='操作历史',variant='outline',className=''}:{objectType?:AuditObjectType;objectId?:string;scope?:AuditScope;filter?:AuditFilter;label?:ReactNode;variant?:ComponentProps<typeof Button>['variant'];className?:string}) {
 const t=useOptionalTenant(),p=useOptionalProject(),identity=useIdentity();
 const scope=given??(p?{kind:'project' as const,tenant_id:p.project.tenant_id,project_id:p.project.id}:t?{kind:'tenant' as const,tenant_id:t.tenant.id}:{kind:'platform' as const});
 const local=scope.kind==='platform'||scope.kind==='project'&&p?.project.id===scope.project_id||scope.kind==='tenant'&&t?.tenant.id===scope.tenant_id;
 const remote=useQuery({queryKey:['audit-history-capabilities',identity.account?.id,scope,identity.authorization_revision,identity.authorizationEpoch],queryFn:({signal})=>identity.client.auditCapabilities(scope,signal),enabled:!!given&&!local&&identity.phase==='authenticated',retry:false,staleTime:0});
 const localAllowed=scope.kind==='platform'?identity.can(P.PlatformAuditRead):scope.kind==='project'?p?.project.id===scope.project_id&&p.can(P.ProjectAuditRead):t?.tenant.id===scope.tenant_id&&t.permissions.includes(P.TenantAuditRead);
 const allowed=local?localAllowed:!remote.error&&remote.data?.read;
 const types:Partial<Record<AuditObjectType,string>>={facility:"engineering_facility",component:"engineering_component",monitoring_section:"engineering_monitoring_section",borehole:"engineering_borehole",point:"engineering_point",channel:"engineering_channel",manufacturer:"device_manufacturer",product:"device_product",product_version:"device_product",display_instance:"display",display_package:"display",file_attachment:"file",file_upload:"file",file_revision:"file",device_access:"project_device_grant_set",resource_access:"project_resource_policy"};
 if(!allowed||!objectId&&!filter.task_id)return null;
 const objectFilter=objectId?(objectType==='resource_access'?{reference_id:objectId,q:'resource_access.'}:objectType==='device'||objectType==='spatial_asset'||objectType==='spatial_version'||objectType==='channel'||objectType==='monitoring_job'?{reference_id:objectId}:{object_type:objectType?types[objectType]??objectType:undefined,object_id:objectId}):{};
 return <Button className={`min-h-11 ${className}`} variant={variant} asChild><Link to={auditPath(scope)} search={{...filter,...objectFilter}}>{label}</Link></Button>;
}
