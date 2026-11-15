import type { AuditTarget } from '@infrapro/api-client';
import { auditPath } from './search';
const uuid=/^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/i;
export function auditDestination(target:AuditTarget):{to:string;search:Record<string,string>}|undefined {
 if(!target.available||!target.id||!uuid.test(target.id))return;
 const s=target.scope;if(s.kind!=='platform'&&(!uuid.test(s.tenant_id)||s.kind==='project'&&!uuid.test(s.project_id)))return;
 const base=s.kind==='platform'?'':`/t/${s.tenant_id}${s.kind==='project'?`/p/${s.project_id}`:''}`;
 const search:Record<string,string>={};let to:string;
 switch(target.kind){
 case 'task':if(!target.generation||!/^[1-9][0-9]*$/.test(target.generation))return;to=`/tasks/${target.id}`;search.execution=target.generation;break;
 case 'audit_export':to=auditPath(s);search.export_id=target.id;search.execution=target.generation??'';search.tab='exports';break;
 case 'account':if(s.kind!=='platform')return;to='/access/accounts';search.account=target.id;break;
 case 'tenant_role':case 'project_role':case 'role':to=s.kind==='platform'?'/access/roles':`${base}/roles/${target.id}`;if(s.kind==='platform')search.role=target.id;break;
 case 'tenant':to=s.kind==='platform'?`/platform/tenants/${target.id}`:`${base}/profile`;break;
 case 'project':to=s.kind==='tenant'?`${base}/projects/${target.id}`:`${base}/profile`;break;
 case 'tenant_member':case 'project_member':to=`${base}/members/${target.id}`;break;
 case 'tenant_invitation':to=`${base}/invitations/${target.id}`;break;
 case 'file_upload':to=`${base}/files`;search.upload=target.id;search.tab='uploads';break;
 case 'file':case 'file_attachment':case 'attachment':to=`${base}/files`;search.attachment=target.id;search.revision=target.revision??'';break;
 case 'monitoring_import':case 'monitoring_batch':to=`${base}/monitoring/imports/${target.id}`;break;
 case 'monitoring_job':to=`${base}/monitoring/jobs`;search.job=target.id;break;
 case 'monitoring_export':to=`${base}/monitoring/jobs`;search.export_id=target.id;break;
 case 'derived_channel':to=`${base}/monitoring/derived/${target.id}`;break;
 case 'derived_run':to=`${base}/monitoring/derived-runs`;search.id=target.id;break;
 case 'analysis_export':to=`${base}/monitoring/derived-runs`;search.export_id=target.id;break;
 case 'derived_evidence_export':to=`${base}/monitoring/derived-runs`;search.evidence_export_id=target.id;break;
 case 'spatial_version':case 'spatial_source':to=`${base}/spatial`;search.version=target.id;search.mode='manage';break;
 case 'spatial_asset':to=`${base}/spatial`;search.asset=target.id;search.mode='manage';break;
 case 'spatial_access':if(s.kind!=='tenant')return;to=`${base}/projects/${target.id}/spatial-access`;break;
 case 'display_package':if(!target.parameters?.instance||!uuid.test(target.parameters.instance))return;to=`${base}/displays`;search.instance=target.parameters.instance;search.package=target.id;break;
 case 'display_instance':case 'display':to=`${base}/displays`;search.instance=target.id;break;
 case 'device_output':case 'device_installation':{const device=target.parameters?.device;if(!device||!uuid.test(device))return;to=`${base}/devices/${device}`;search.tab=target.kind==='device_output'?'outputs':'installations';if(target.kind==='device_output')search.output=target.id;break;}
 case 'device':to=`${base}/devices/${target.id}`;break;
 case 'project_device_grant_set':if(s.kind!=='tenant')return;to=`${base}/projects/${target.id}/device-access`;break;
 case 'project_resource_policy':case 'project_resource_grant_set':case 'project_facility_link':{const project=target.parameters?.project;if(s.kind!=='tenant'||!project||!uuid.test(project))return;to=`${base}/resource-access/${project}`;const facility=target.parameters?.facility;if(facility&&uuid.test(facility))to+=`/facilities/${facility}`;break;}
 case 'monitoring_source':{const p=target.parameters,facility=p?.facility,point=p?.point,channel=p?.channel;if(!facility||!point||!channel||!uuid.test(facility)||!uuid.test(point)||!uuid.test(channel))return;to=`${base}/resources/facilities/${facility}`;search.point=point;search.channel=channel;search.view='source';break;}
 case 'engineering_facility':case 'engineering_component':case 'engineering_point':case 'engineering_channel':case 'facility':case 'component':case 'point':case 'channel':{
 const kind=target.kind.replace('engineering_',''),facility=target.parameters?.facility??(kind==='facility'?target.id:'');if(!uuid.test(facility))return;to=`${base}/resources/facilities/${facility}`;
 if(kind!=='facility')search[kind]=target.id;if(kind==='channel'&&target.parameters?.point&&uuid.test(target.parameters.point))search.point=target.parameters.point;break;
 }
 case 'engineering_definition':{const kind=target.parameters?.kind;if(!kind||!['units','metrics','type-definitions','monitoring-templates'].includes(kind))return;to=s.kind==='platform'?`/platform/engineering/${kind}`:`${base}/engineering/${kind==='monitoring-templates'?'templates':`catalog/${kind}`}`;search.definition=target.id;break;}
 case 'device_product':to=s.kind==='platform'?`/platform/device-products/${target.id}`:`${base}/device-products/${target.id}`;break;
 case 'device_manufacturer':to=s.kind==='platform'?'/platform/device-products':`${base}/device-products`;search.manufacturer=target.id;break;
 case 'analysis_artifact':to=`${base}/monitoring/analysis/${target.id}`;break;
 default:return;
 }
 return {to,search};
}
