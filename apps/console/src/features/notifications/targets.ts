import type { NotificationTarget } from '@infrapro/api-client';
import { safeReturn } from '../../config/navigation';
const uuid = /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/i;
// Only domain keys, never a server-supplied URL or nested redirect.
export function notificationDestination(target: NotificationTarget): { to: string; search: Record<string, string> } | undefined {
 if (target.status !== 'available' || !target.kind) return;
 if (target.id && !uuid.test(target.id) || target.tenant_id && !uuid.test(target.tenant_id) || target.project_id && (!target.tenant_id || !uuid.test(target.project_id))) return;
 const base = target.tenant_id ? `/t/${target.tenant_id}${target.project_id ? `/p/${target.project_id}` : ''}` : '';
 let to: string; let search: Record<string, string> = {};
 switch (target.kind) {
  case 'audit_export': if(!target.id||!target.generation||!/^[1-9][0-9]*$/.test(target.generation))return;to=base?`${base}/audit`:'/access/audit';search={tab:'exports',export_id:target.id,execution:target.generation};break;
  case 'task': if (!target.id || !target.generation || !/^[1-9][0-9]*$/.test(target.generation)) return; to = `/tasks/${target.id}`; search = { execution: target.generation }; break;
  case 'security': to = '/security'; break;
  case 'signup_invitation': if (!target.id) return; to = '/invitations'; search = { invitation: target.id }; break;
  case 'tenant_invitation': if (!target.id) return; to = `/notifications/invitations/${target.id}`; break;
  case 'workspaces': to = base ? `${base}/profile` : '/'; break;
  case 'project_access': case 'spatial_access': if (!target.project_id) return; to = `${base}/profile`; break;
  case 'file_job': if (!base || !target.id || !target.generation) return; to = `${base}/files`; search = {tab:'uploads',job:target.id,generation:target.generation}; break;
  case 'monitoring_import': if (!base || !target.id) return; to = `${base}/monitoring/imports/${target.id}`; break;
  case 'monitoring_job': case 'derived_job': if (!base || !target.id) return; to = `${base}/monitoring/jobs`; search = { job: target.id, ...(target.generation ? { execution: target.generation } : {}) }; break;
  case 'derived_channel': if (!base || !target.id) return; to = `${base}/monitoring/derived/${target.id}`; break;
  case 'spatial_version': if (!base || !target.id) return; to = `${base}/spatial`; search = { version: target.id, ...(target.project_id ? {} : { mode: 'manage' }) }; break;
  case 'display': if (!target.project_id || !target.id) return; to = `${base}/displays`; search = { instance: target.id }; break;
  default: return;
 }
 if (safeReturn(to) !== to) return;
 return { to, search };
}
