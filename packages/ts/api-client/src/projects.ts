import type { components } from './schema';
type Schema<K extends keyof components['schemas']> = components['schemas'][K];
export type ProjectSummary = Schema<'ProjectSummary'>;
export type ProjectProfile = Schema<'ProjectProfile'>;
export type ProjectContext = Schema<'ProjectContext'>;
export type ProjectPerson = Schema<'ProjectPerson'>;
export type ProjectMember = Schema<'ProjectMember'>;
export type ProjectAuthorization = Schema<'ProjectAuthorization'>;
export type ProjectPreview = Schema<'ProjectPreview'>;
export type ProjectResult = Schema<'ProjectResult'>;
export type ProjectCreateInput = Schema<'ProjectCreateInput'>;
export type ProjectUpdateInput = Schema<'ProjectUpdateInput'>;
export type ProjectRoleInput = Schema<'ProjectRoleInput'>;
export type ProjectAction = 'archive' | 'restore' | 'owner' | 'administrator-transfer' | 'administrator-recovery' | 'member.suspend' | 'member.resume' | 'member.remove' | 'member.roles' | 'role.permissions' | 'role.status' | 'role.delete';
export type ProjectChange = Schema<'ProjectChangeInput'> & Partial<Pick<Schema<'ProjectCandidateInput'>, 'tenant_membership_id'> & Pick<Schema<'ProjectRolesInput'>, 'role_ids' | 'role_versions'> & Pick<Schema<'ProjectPermissionsInput'>, 'permission_codes'> & Pick<Schema<'ProjectStatusInput'>, 'status'>>;
export type ProjectFilter = { q?: string; status?: string; cursor?: string; limit?: number; owner_tenant_membership_id?: string; administrator_unavailable?: boolean; event_type?: string; from?: string; to?: string; object_id?: string; actor_id?: string };
type Request = <T>(path: string, method?: string, body?: unknown, notify?: boolean, signal?: AbortSignal) => Promise<T>;
const t = (id: string) => `/tenants/${encodeURIComponent(id)}`;
const p = (tenant: string, project: string) => `${t(tenant)}/projects/${encodeURIComponent(project)}`;
const g = (tenant: string, project?: string) => `${t(tenant)}/project-governance${project ? `/${encodeURIComponent(project)}` : ''}`;
const query = (filter: ProjectFilter = {}) => new URLSearchParams(Object.entries(filter).filter(([, v]) => v !== undefined && v !== '').map(([k, v]) => [k, String(v)])).toString();
const actionPath = (tenant: string, project: string, action: ProjectAction, target?: string) => action === 'administrator-recovery' ? `${g(tenant, project)}/${action}` : action.includes('.') ? `${p(tenant, project)}/${action.startsWith('member.') ? 'members' : 'roles'}/${encodeURIComponent(target ?? '')}/${action.split('.')[1]}` : `${p(tenant, project)}/${action}`;
export function projectMethods(request: Request) {
  return {
    myProjects: (filter?: ProjectFilter, signal?: AbortSignal) => request<Schema<'Projects'>>(`/me/projects?${query(filter)}`, 'GET', undefined, true, signal),
    projectGovernance: (tenant: string, filter?: ProjectFilter, signal?: AbortSignal) => request<Schema<'Projects'>>(`${g(tenant)}?${query(filter)}`, 'GET', undefined, true, signal),
    projectSummary: (tenant: string, project: string, signal?: AbortSignal) => request<ProjectSummary>(g(tenant, project), 'GET', undefined, true, signal),
    createProject: (tenant: string, input: ProjectCreateInput) => request<ProjectResult>(`${t(tenant)}/projects`, 'POST', input),
    projectContext: (tenant: string, project: string, signal?: AbortSignal) => request<ProjectContext>(`${p(tenant, project)}/context`, 'GET', undefined, true, signal),
    projectProfile: (tenant: string, project: string, signal?: AbortSignal) => request<ProjectProfile>(`${p(tenant, project)}/profile`, 'GET', undefined, true, signal),
    updateProject: (tenant: string, project: string, input: ProjectUpdateInput) => request<ProjectResult>(`${p(tenant, project)}/profile`, 'PATCH', input),
    projectCandidates: (tenant: string, project: string, kind: 'create' | 'recovery' | 'member' | 'owner' | 'administrator', q: string, signal?: AbortSignal) => request<Schema<'ProjectCandidates'>>(`${kind === 'create' ? `${t(tenant)}/project-candidates` : kind === 'recovery' ? `${g(tenant, project)}/administrator-candidates` : `${p(tenant, project)}/${kind}-candidates`}?${query({ q })}`, 'GET', undefined, true, signal),
    projectMembers: (tenant: string, project: string, filter?: ProjectFilter, signal?: AbortSignal, role?: string) => request<Schema<'ProjectMembers'>>(`${p(tenant, project)}${role ? `/roles/${encodeURIComponent(role)}` : ''}/members?${query(filter)}`, 'GET', undefined, true, signal),
    projectMember: (tenant: string, project: string, id: string, signal?: AbortSignal) => request<ProjectMember>(`${p(tenant, project)}/members/${encodeURIComponent(id)}`, 'GET', undefined, true, signal),
    addProjectMember: (tenant: string, project: string, input: Schema<'ProjectAddMemberInput'>) => request<ProjectResult>(`${p(tenant, project)}/members`, 'POST', input),
    projectAuthorization: (tenant: string, project: string, id: string, signal?: AbortSignal) => request<ProjectAuthorization>(`${p(tenant, project)}/members/${encodeURIComponent(id)}/authorization`, 'GET', undefined, true, signal),
    projectRoles: (tenant: string, project: string, filter?: ProjectFilter, signal?: AbortSignal) => request<Schema<'ProjectRoles'>>(`${p(tenant, project)}/roles?${query(filter)}`, 'GET', undefined, true, signal),
    projectRole: (tenant: string, project: string, id: string, signal?: AbortSignal) => request<Schema<'Role'>>(`${p(tenant, project)}/roles/${encodeURIComponent(id)}`, 'GET', undefined, true, signal),
    createProjectRole: (tenant: string, project: string, input: ProjectRoleInput, source?: string) => request<ProjectResult>(`${p(tenant, project)}/roles${source ? `/${encodeURIComponent(source)}/copy` : ''}`, 'POST', input),
    updateProjectRole: (tenant: string, project: string, id: string, input: Schema<'ProjectRoleUpdateInput'>) => request<ProjectResult>(`${p(tenant, project)}/roles/${encodeURIComponent(id)}`, 'PATCH', input),
    projectPermissions: (tenant: string, project: string, signal?: AbortSignal) => request<Schema<'Permission'>[]>(`${p(tenant, project)}/permissions`, 'GET', undefined, true, signal),
    previewProjectChange: (tenant: string, project: string, action: ProjectAction, input: Partial<ProjectChange>, target?: string, signal?: AbortSignal) => request<ProjectPreview>(`${actionPath(tenant, project, action, target)}/preview`, 'POST', input, true, signal),
    changeProject: (tenant: string, project: string, action: ProjectAction, input: ProjectChange, target?: string) => request<ProjectResult>(actionPath(tenant, project, action, target), action === 'member.roles' || action === 'role.permissions' ? 'PUT' : 'POST', input),
    projectAudit: (tenant: string, project: string, filter?: ProjectFilter, signal?: AbortSignal) => request<Schema<'ProjectAuditEvents'>>(`${p(tenant, project)}/audit?${query(filter)}`, 'GET', undefined, true, signal),
    projectRolePresets: (filter?: ProjectFilter, signal?: AbortSignal) => request<Schema<'ProjectRoles'>>(`/access/project-role-presets?${query(filter)}`, 'GET', undefined, true, signal),
    previewTenantTransfer: (tenant: string, signal?: AbortSignal) => request<Schema<'TenantProjectTransferPreview'>>(`/platform/tenants/${encodeURIComponent(tenant)}/administrator-transfer/preview`, 'POST', {}, true, signal),
  };
}
