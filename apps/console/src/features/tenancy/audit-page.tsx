import { AuditWorkspace } from '../audit/workspace';
import { useTenant } from './context';
export function TenantAuditPage(){const ctx=useTenant();return <AuditWorkspace scope={{kind:'tenant',tenant_id:ctx.tenant.id}}/>;}
