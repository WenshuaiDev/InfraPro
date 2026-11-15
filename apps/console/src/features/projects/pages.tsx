import { AuditWorkspace } from '../audit/workspace';
import { useProject } from './context';
export function ProjectAuditPage(){const ctx=useProject();return <AuditWorkspace scope={{kind:'project',tenant_id:ctx.project.tenant_id,project_id:ctx.project.id}}/>;}
