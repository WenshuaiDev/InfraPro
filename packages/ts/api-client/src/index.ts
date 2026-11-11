import { resourceMethods } from './resources';
export type * from './resources';
import { engineeringMethods, type EngineeringIssue } from './engineering';
export type * from './engineering';
import { projectMethods } from './projects';
export type * from './projects';
import type { components } from './schema';
export { Permissions, type PermissionCode } from './permissions.generated';
export type Role = components['schemas']['Role'];
export type Permission = components['schemas']['Permission'];
export type Authorization = components['schemas']['Authorization'];
export type AuthContext = components['schemas']['AuthContext'];
export type ManagedAccount = components['schemas']['ManagedAccount'];
export type AccessAuditEvent = components['schemas']['AccessAuditEvent'];
export type TenantRoleDefinition = Schema<'TenantRoleDefinition'>;
export type TenantPermission = Schema<'TenantPermission'>;
export type TenantAuthorization = Schema<'TenantAuthorization'>;
export type TenantAuthorizationImpact = Schema<'TenantAuthorizationImpact'>;
export type TenantRoleCreateInput = Schema<'TenantRoleCreateInput'>;
export type TenantRoleChangeInput = Schema<'TenantRolePermissionsInput'> | Schema<'TenantRoleStatusInput'> | Schema<'TenantRoleArchiveInput'>;
export type TenantRolePreviewInput = Schema<'TenantRolePermissionsPreviewInput'> | Schema<'TenantRoleStatusPreviewInput'> | Schema<'TenantRoleArchivePreviewInput'>;
export type TenantAvailability = Schema<'TenantAvailability'>;
export type TenantStatusPreview = Schema<'TenantStatusPreview'>;
export type Tenant = components['schemas']['Tenant'];
export type TenantProfile = components['schemas']['TenantProfile'];
export type TenantMember = components['schemas']['TenantMember'];
export type TenantInvitation = components['schemas']['TenantInvitation'];
export type TenantInvitationCreated = components['schemas']['TenantInvitationCreated'];
export type TenantInvitationInspection = components['schemas']['TenantInvitationInspection'];
export type TenantContext = components['schemas']['TenantContext'];
export type TenantAccount = components['schemas']['TenantAccount'];
export type TenantAdministratorCandidate = components['schemas']['TenantAdministratorCandidate'];
export type MyTenant = components['schemas']['MyTenant'];
export type TenantAuditEvent = components['schemas']['TenantAuditEvent'];
export type TenantResult = components['schemas']['TenantMutationResult'];
export type TenantFilter = { cursor?: string; q?: string; limit?: number; administrator_unavailable?: boolean; event_type?: string; from?: string; to?: string; status?: string; role?: string; object_type?: string; object_id?: string; tenant_id?: string };
export type AccessFilter = { category?: string; outcome?: string; source?: string; tenant_id?: string; event_type?: string; cursor?: string; q?: string; status?: string; role_id?: string; object_id?: string; object_type?: string; from?: string; to?: string; limit?: number };


export class ApiError extends Error {
  constructor(message: string, public readonly status?: number, public readonly code?: string, public readonly fields: Record<string, string> = {}, public readonly issues: EngineeringIssue[] = [], public readonly dependencies: Record<string, number> = {}) {
    super(message);
    this.name = 'ApiError';
  }
}

export function createApiClient({ baseUrl = '/api/v1', timeoutMs = 5000 } = {}) {
  const base = baseUrl.replace(/\/+$/, '');
  return accountMethods(base, timeoutMs);
}

export type ApiClient = ReturnType<typeof createApiClient>;

export type Account = components['schemas']['Account'];
export type AccountHelp = components['schemas']['AccountHelp'];
export type Session = components['schemas']['Session'];
export type Invitation = components['schemas']['Invitation'];
export type InvitationFilter = { cursor?: string; q?: string; status?: Invitation['status'] | '' };
export type InvitationCounts = components['schemas']['InvitationCounts'];
export type SecurityEvent = components['schemas']['SecurityEvent'];
export type LinkResult = components['schemas']['Link'];

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];
function accountMethods(base: string, timeoutMs: number) {
  let csrfToken = '';
  let identityEpoch = 0;
  const projectEpochs = new Map<string, number>();
  const projectMemberships = new Map<string, string>();
  const projectListeners = new Set<(tenant: string, project: string, code: string) => void>();
  const invalidateProjectScope = (tenant: string, project: string) => { const key = `${tenant}:${project}`; projectEpochs.set(key, (projectEpochs.get(key) ?? 0) + 1); };
  const resourceEpochs = new Map<string, number>();
  const resourceRevisions = new Map<string, string>();
  const resourceListeners = new Set<(tenant: string, project: string | undefined, code: string) => void>();
  const invalidateResourceScope = (tenant: string, project?: string) => { const key = `${tenant}:${project ?? ''}`; resourceEpochs.set(key, (resourceEpochs.get(key) ?? 0) + 1); };
  const tenantEpochs = new Map<string, number>();
  const tenantMemberships = new Map<string, string>();
  const tenantListeners = new Set<(id: string, code: string) => void>();
  const invalidateTenantScope = (id: string) => tenantEpochs.set(id, (tenantEpochs.get(id) ?? 0) + 1);
  let csrfPending: Promise<void> | undefined;
  const listeners = new Set<() => void>();
  const authorizationListeners = new Set<(revision?: string) => void>();
  async function request<T>(path: string, method = 'GET', body?: unknown, notifyUnauthorized = true, signal?: AbortSignal): Promise<T> {
    const epoch = identityEpoch;
    const tenant = /^\/tenants\/([^/?]+)/.exec(path)?.[1];
    const project = path.includes('/resource-access') ? undefined : /^\/tenants\/[^/]+\/projects\/([^/?]+)/.exec(path)?.[1];
    const resourceRequest = /\/engineering\/(resources|catalog)(?:\/|$)/.test(path);
    const resourceKey = `${tenant}:${project ?? ''}`; const resourceEpoch = resourceEpochs.get(resourceKey) ?? 0;
    const projectKey = `${tenant}:${project}`; const projectEpoch = projectEpochs.get(projectKey) ?? 0;
    const tenantEpoch = tenant ? tenantEpochs.get(tenant) ?? 0 : 0;
    const current = () => { if (epoch !== identityEpoch || (resourceRequest && resourceEpoch !== (resourceEpochs.get(resourceKey) ?? 0)) || (project && projectEpoch !== (projectEpochs.get(projectKey) ?? 0)) || (tenant && tenantEpoch !== (tenantEpochs.get(tenant) ?? 0))) throw new DOMException('Identity or authorization changed', 'AbortError'); };
    if (method !== 'GET' && !csrfToken) await csrf();
    current();
    let response: Response;
    try {
      response = await fetch(`${base}${path}`, {
        method, credentials: 'same-origin', cache: 'no-store', signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(timeoutMs)]) : AbortSignal.timeout(timeoutMs),
        headers: { Accept: 'application/json', ...(method === 'GET' ? {} : { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }) },
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      });
    } catch { signal?.throwIfAborted(); current(); throw new ApiError(method === 'GET' ? '无法连接服务，请稍后重试。' : '结果尚未确认，请查验对应记录后再决定是否重试。'); }
    const revision = response.headers.get('X-Authorization-Revision');
    if (response.status === 204) {
      current(); signal?.throwIfAborted();
      if (revision) authorizationListeners.forEach((listener) => listener(revision));
      return undefined as T;
    }
    let data: unknown;
    try { data = await response.json(); } catch { throw new ApiError('服务返回了无效数据，请检查连接。', response.status); }
    current(); signal?.throwIfAborted();
    if (revision && path !== '/auth/me') authorizationListeners.forEach((listener) => listener(revision));
    if (!response.ok) {
      const error = data && typeof data === 'object' ? data as Partial<Schema<'Error'>> & { dependencies?: Record<string, number> } : {};
      if (notifyUnauthorized && error.code === 'unauthenticated') listeners.forEach((listener) => listener());
      if (tenant && resourceRequest && ['permission_denied', 'resource_not_found', 'resource_access_changed', 'project_archived'].includes(error.code ?? '')) {
        resourceListeners.forEach((listener) => listener(tenant, project, error.code!));
      } else if (project && tenant && ['project_not_found', 'permission_denied', 'project_archived'].includes(error.code ?? '')) {
        invalidateProjectScope(tenant, project); projectListeners.forEach((listener) => listener(tenant, project, error.code!));
      } else if (tenant && ['tenant_disabled', 'tenant_not_found', 'permission_denied'].includes(error.code ?? '')) {
        invalidateTenantScope(tenant); tenantListeners.forEach((listener) => listener(tenant, error.code!));
      } else if (error.code === 'permission_denied') authorizationListeners.forEach((listener) => listener());
      if (error.code === 'csrf_invalid') { csrfToken = ''; await csrf().catch(() => undefined); }
      const retry = response.headers.get('Retry-After');
      throw new ApiError((typeof error.message === 'string' ? error.message : '请求失败，请稍后重试。') + (response.status === 429 && retry ? ` 请在 ${retry} 秒后重试。` : ''), response.status, error.code, error.fields, error.issues ?? [], error.dependencies ?? {});
    }
    if (project && tenant && path.endsWith('/context')) {
      const member = (data as import('./projects').ProjectContext).authorization.membership_id; const previous = projectMemberships.get(projectKey);
      if (previous && previous !== member) invalidateProjectScope(tenant, project);
      projectMemberships.set(projectKey, member);
      const resourceRevision = (data as import('./projects').ProjectContext).resource_access_revision;
      const previousRevision = resourceRevisions.get(resourceKey);
      if (previousRevision && resourceRevision !== previousRevision) invalidateResourceScope(tenant, project);
      resourceRevisions.set(resourceKey, resourceRevision);
    }
    if (project && tenant && resourceRequest && data && typeof data === 'object' && 'access_revision' in data) {
      const observed = String(data.access_revision ?? '');
      const known = resourceRevisions.get(resourceKey);
      if (known && observed && observed !== known) resourceListeners.forEach(listener => listener(tenant, project, 'resource_access_changed'));
    }
    if (tenant && /^\/tenants\/[^/]+\/context$/.test(path)) {
      const member = (data as TenantContext).membership_id;
      const previous = tenantMemberships.get(tenant);
      // Advance before resolving the context, so its new child queries capture the
      // new epoch. An effect after rendering would accidentally cancel those reads.
      if (previous && previous !== member) invalidateTenantScope(tenant);
      tenantMemberships.set(tenant, member);
      tenantListeners.forEach((listener) => listener(tenant, 'active'));
    }
    return data as T;
  }
  function csrf() {
    if (!csrfPending) csrfPending = request<Schema<'CSRF'>>('/auth/csrf').then((data) => { csrfToken = data.token; }).finally(() => { csrfPending = undefined; });
    return csrfPending;
  }
  const query = (filter: AccessFilter | TenantFilter = {}) => new URLSearchParams(Object.entries(filter).filter(([, v]) => v !== undefined && v !== '').map(([k, v]) => [k, String(v)])).toString();
  const idPath = (kind: string, id: string) => `/access/${kind}/${encodeURIComponent(id)}`;
  const tenantPath = (id: string) => `/tenants/${encodeURIComponent(id)}`;
  return {
    ...projectMethods(request),
    invalidateResourceScope,
    onResourceRestriction(listener: (tenant: string, project: string | undefined, code: string) => void) { resourceListeners.add(listener); return () => { resourceListeners.delete(listener); }; },
    ...engineeringMethods(request),
    ...resourceMethods(request),
    invalidateProjectScope,
    onProjectRestriction(listener: (tenant: string, project: string, code: string) => void) { projectListeners.add(listener); return () => { projectListeners.delete(listener); }; },
    csrf,
    invalidateTenantScope,
    onTenantRestriction(listener: (id: string, code: string) => void) { tenantListeners.add(listener); return () => { tenantListeners.delete(listener); }; },
    tenantAvailability: (id: string, signal?: AbortSignal) => request<TenantAvailability>(`/account/tenants/${encodeURIComponent(id)}/availability`, 'GET', undefined, true, signal),
    previewTenantStatus: (id: string, action: 'disable' | 'enable', signal?: AbortSignal) => request<TenantStatusPreview>(`/platform/tenants/${encodeURIComponent(id)}/${action}/preview`, 'POST', {}, true, signal),
    changeTenantStatus: (id: string, action: 'disable' | 'enable', input: Schema<'TenantDisableInput'> | Schema<'TenantEnableInput'>) => request<Schema<'TenantStatusResult'>>(`/platform/tenants/${encodeURIComponent(id)}/${action}`, 'POST', input),
    tenantRolePresets: (filter?: AccessFilter, signal?: AbortSignal) => request<Schema<'TenantRoleDefinitions'>>(`/access/tenant-role-presets?${query(filter)}`, 'GET', undefined, true, signal),
    tenantPermissions: (id: string, signal?: AbortSignal) => request<TenantPermission[]>(`${tenantPath(id)}/permissions`, 'GET', undefined, true, signal),
    tenantRoles: (id: string, filter?: AccessFilter, signal?: AbortSignal) => request<Schema<'TenantRoleDefinitions'>>(`${tenantPath(id)}/roles?${query(filter)}`, 'GET', undefined, true, signal),
    tenantRole: (id: string, role: string, signal?: AbortSignal) => request<TenantRoleDefinition>(`${tenantPath(id)}/roles/${encodeURIComponent(role)}`, 'GET', undefined, true, signal),
    tenantRoleMembers: (id: string, role: string, filter?: AccessFilter, signal?: AbortSignal) => request<Schema<'TenantRoleMembers'>>(`${tenantPath(id)}/roles/${encodeURIComponent(role)}/members?${query(filter)}`, 'GET', undefined, true, signal),
    createTenantRole: (id: string, input: TenantRoleCreateInput, source?: string) => request<Schema<'TenantRoleResult'>>(`${tenantPath(id)}/roles${source ? `/${encodeURIComponent(source)}/copy` : ''}`, 'POST', input),
    updateTenantRole: (id: string, role: string, input: Schema<'TenantRoleUpdateInput'>) => request<Schema<'TenantAuthorizationResult'>>(`${tenantPath(id)}/roles/${encodeURIComponent(role)}`, 'PATCH', input),
    previewTenantRole: (id: string, role: string, action: 'permissions' | 'status' | 'archive', input: TenantRolePreviewInput, filter?: AccessFilter) => request<TenantAuthorizationImpact>(`${tenantPath(id)}/roles/${encodeURIComponent(role)}/${action}/preview?${query(filter)}`, 'POST', input),
    changeTenantRole: (id: string, role: string, action: 'permissions' | 'status' | 'archive', input: TenantRoleChangeInput) => request<Schema<'TenantAuthorizationResult'>>(`${tenantPath(id)}/roles/${encodeURIComponent(role)}/${action}`, action === 'archive' ? 'POST' : 'PUT', input),
    tenantMemberAuthorization: (id: string, member: string, signal?: AbortSignal) => request<TenantAuthorization>(`${tenantPath(id)}/members/${encodeURIComponent(member)}/authorization`, 'GET', undefined, true, signal),
    previewTenantMemberRoles: (id: string, member: string, input: Schema<'TenantMemberRolesPreviewInput'>, filter?: AccessFilter) => request<TenantAuthorizationImpact>(`${tenantPath(id)}/members/${encodeURIComponent(member)}/roles/preview?${query(filter)}`, 'POST', input),
    assignTenantMemberRoles: (id: string, member: string, input: Schema<'TenantMemberRolesInput'>) => request<Schema<'TenantAuthorizationResult'>>(`${tenantPath(id)}/members/${encodeURIComponent(member)}/roles`, 'PUT', input),
    tenantMembers: (id: string, filter?: TenantFilter, signal?: AbortSignal) => request<Schema<'TenantMembers'>>(`${tenantPath(id)}/members?${query(filter)}`, 'GET', undefined, true, signal),
    tenantMember: (id: string, member: string, signal?: AbortSignal) => request<TenantMember>(`${tenantPath(id)}/members/${encodeURIComponent(member)}`, 'GET', undefined, true, signal),
    changeTenantMember: (id: string, member: string, action: 'suspend' | 'resume' | 'remove', input: Schema<'TenantMemberMutationInput'>) => request<Schema<'TenantMembershipResult'>>(`${tenantPath(id)}/members/${encodeURIComponent(member)}/${action}`, 'POST', input),
    resolveTenantMember: (id: string, username: string) => request<TenantAccount>(`${tenantPath(id)}/member-candidates/resolve`, 'POST', { username }),
    tenantInvitations: (id: string, filter?: TenantFilter, signal?: AbortSignal) => request<Schema<'TenantInvitations'>>(`${tenantPath(id)}/member-invitations?${query(filter)}`, 'GET', undefined, true, signal),
    tenantInvitation: (id: string, invitation: string, signal?: AbortSignal) => request<TenantInvitation>(`${tenantPath(id)}/member-invitations/${encodeURIComponent(invitation)}`, 'GET', undefined, true, signal),
    createTenantInvitation: (id: string, input: Schema<'CreateTenantInvitationInput'>) => request<TenantInvitationCreated>(`${tenantPath(id)}/member-invitations`, 'POST', input),
    revokeTenantInvitation: (id: string, invitation: string, input: Schema<'TenantMemberMutationInput'>) => request<TenantInvitation>(`${tenantPath(id)}/member-invitations/${encodeURIComponent(invitation)}/revoke`, 'POST', input),
    regenerateTenantInvitation: (id: string, invitation: string, input: Schema<'RegenerateTenantInvitationInput'>) => request<TenantInvitationCreated>(`${tenantPath(id)}/member-invitations/${encodeURIComponent(invitation)}/regenerate`, 'POST', input),
    inspectTenantInvitation: (token: string, signal?: AbortSignal) => request<TenantInvitationInspection>('/account/tenant-invitations/inspect', 'POST', { token }, true, signal),
    acceptTenantInvitation: (input: Schema<'AcceptTenantInvitationInput'>) => request<Schema<'TenantAcceptanceResult'>>('/account/tenant-invitations/accept', 'POST', input),
    tenants: (filter?: TenantFilter, signal?: AbortSignal) => request<Schema<'Tenants'>>(`/platform/tenants?${query(filter)}`, 'GET', undefined, true, signal),
    myTenants: (filter?: TenantFilter, signal?: AbortSignal) => request<Schema<'MyTenants'>>(`/account/tenants?${query(filter)}`, 'GET', undefined, true, signal),
    accountTenants: (id: string, filter?: TenantFilter, signal?: AbortSignal) => request<Schema<'MyTenants'>>(`${idPath('accounts', id)}/tenant-memberships?${query(filter)}`, 'GET', undefined, true, signal),
    tenantCandidates: (filter: TenantFilter, signal?: AbortSignal) => request<Schema<'TenantCandidates'>>(`/platform/tenant-admin-candidates?${query(filter)}`, 'GET', undefined, true, signal),
    tenant: <T extends boolean>(id: string, platform: T, signal?: AbortSignal) => request<T extends true ? Tenant : TenantProfile>(platform ? `/platform/tenants/${encodeURIComponent(id)}` : `/tenants/${encodeURIComponent(id)}/profile`, 'GET', undefined, true, signal),
    tenantContext: (id: string, signal?: AbortSignal) => request<Schema<'TenantContext'>>(`/tenants/${encodeURIComponent(id)}/context`, 'GET', undefined, true, signal),
    createTenant: (input: Schema<'CreateTenantInput'>) => request<Schema<'TenantMutationResult'>>('/platform/tenants', 'POST', input),
    updateTenant: (id: string, platform: boolean, input: Schema<'UpdateTenantInput'>) => request<Schema<'TenantMutationResult'>>(platform ? `/platform/tenants/${encodeURIComponent(id)}` : `/tenants/${encodeURIComponent(id)}/profile`, 'PATCH', input),
    transferTenant: (id: string, input: Schema<'TransferTenantInput'>) => request<Schema<'TenantMutationResult'>>(`/platform/tenants/${encodeURIComponent(id)}/administrator-transfer`, 'POST', input),
    tenantAudit: (id: string, filter?: TenantFilter, signal?: AbortSignal) => request<Schema<'TenantAuditEvents'>>(`/tenants/${encodeURIComponent(id)}/audit-events?${query(filter)}`, 'GET', undefined, true, signal),
    invalidateIdentity() { identityEpoch++; },
    onAuthorizationChange(listener: (revision?: string) => void) { authorizationListeners.add(listener); return () => { authorizationListeners.delete(listener); }; },
    permissions: (signal?: AbortSignal) => request<Schema<'Permissions'>>('/access/permissions', 'GET', undefined, true, signal),
    profile: (signal?: AbortSignal) => request<Schema<'Profile'>>('/account/profile', 'GET', undefined, true, signal),
    roles: (filter?: AccessFilter, signal?: AbortSignal) => request<Schema<'Roles'>>(`/access/roles?${query(filter)}`, 'GET', undefined, true, signal),
    role: (id: string, signal?: AbortSignal) => request<Schema<'Role'>>(idPath('roles', id), 'GET', undefined, true, signal),
    createRole: (input: Schema<'CreateRoleInput'>) => request<Schema<'Role'>>('/access/roles', 'POST', input),
    updateRole: (id: string, input: Schema<'UpdateRoleInput'>) => request<Schema<'Role'>>(idPath('roles', id), 'PATCH', input),
    setRolePermissions: (id: string, input: Schema<'RolePermissionsInput'>) => request<Schema<'Role'>>(`${idPath('roles', id)}/permissions`, 'PUT', input),
    setRoleStatus: (id: string, input: Schema<'RoleStatusInput'>) => request<Schema<'Role'>>(`${idPath('roles', id)}/status`, 'PUT', input),
    archiveRole: (id: string, input: Schema<'AccessMutation'>) => request<void>(`${idPath('roles', id)}/archive`, 'POST', input),
    accounts: (filter?: AccessFilter, signal?: AbortSignal) => request<Schema<'ManagedAccounts'>>(`/access/accounts?${query(filter)}`, 'GET', undefined, true, signal),
    account: (id: string, signal?: AbortSignal) => request<Schema<'ManagedAccount'>>(idPath('accounts', id), 'GET', undefined, true, signal),
    authorization: (id: string, signal?: AbortSignal) => request<Schema<'Authorization'>>(`${idPath('accounts', id)}/authorization`, 'GET', undefined, true, signal),
    assignRoles: (id: string, input: Schema<'AccountRolesInput'>) => request<Schema<'ManagedAccount'>>(`${idPath('accounts', id)}/roles`, 'PUT', input),
    setAccountStatus: (id: string, enabled: boolean, input: Schema<'AccessMutation'>) => request<Schema<'ManagedAccount'>>(`${idPath('accounts', id)}/${enabled ? 'enable' : 'disable'}`, 'POST', input),
    accessAudit: (filter?: AccessFilter, signal?: AbortSignal) => request<Schema<'AccessAuditEvents'>>(`/access/audit-events?${query(filter)}`, 'GET', undefined, true, signal),
    onUnauthorized(listener: () => void) { listeners.add(listener); return () => { listeners.delete(listener); }; },
    accountHelp: () => request<AccountHelp>('/public/account-help'),
    me: () => request<Schema<'AuthContext'>>('/auth/me', 'GET', undefined, false),
    login: (input: Schema<'LoginInput'>) => request<Account>('/auth/login', 'POST', input),
    logout: () => request<void>('/auth/logout', 'POST'),
    logoutAll: () => request<void>('/auth/logout-all', 'POST'),
    activity: () => request<void>('/auth/activity', 'POST'),
    reauthenticate: (password: string) => request<void>('/auth/reauthenticate', 'POST', { password } satisfies Schema<'PasswordInput'>),
    updateProfile: (display_name: string) => request<Account>('/account/profile', 'PATCH', { display_name } satisfies Schema<'ProfileInput'>),
    changePassword: (input: Schema<'ChangePasswordInput'>) => request<void>('/account/password', 'POST', input),
    sessions: (signal?: AbortSignal) => request<Schema<'Sessions'>>('/account/sessions', 'GET', undefined, true, signal),
    revokeSession: (id: string) => request<void>(`/account/sessions/${encodeURIComponent(id)}`, 'DELETE'),
    securityEvents: (cursor = '', signal?: AbortSignal) => request<Schema<'SecurityEvents'>>(`/account/security-events?cursor=${encodeURIComponent(cursor)}`, 'GET', undefined, true, signal),
    invitations: (filter: InvitationFilter = {}, signal?: AbortSignal) => request<Schema<'Invitations'>>(`/invitations?${new URLSearchParams({ cursor: filter.cursor ?? '', q: filter.q ?? '', status: filter.status ?? '' })}`, 'GET', undefined, true, signal),
    invitation: (id: string, signal?: AbortSignal) => request<Invitation>(`/invitations/${encodeURIComponent(id)}`, 'GET', undefined, true, signal),
    createInvitation: (note: string) => request<LinkResult>('/invitations', 'POST', { note } satisfies Schema<'InvitationInput'>),
    revokeInvitation: (id: string) => request<void>(`/invitations/${encodeURIComponent(id)}/revoke`, 'POST'),
    regenerateInvitation: (id: string) => request<LinkResult>(`/invitations/${encodeURIComponent(id)}/regenerate`, 'POST'),
    inspectInvitation: (token: string) => request<Schema<'Inspection'>>('/public/invitations/inspect', 'POST', { token } satisfies Schema<'TokenInput'>),
    acceptInvitation: (input: Schema<'AcceptInvitationInput'>) => request<void>('/public/invitations/accept', 'POST', input),
    inspectReset: (token: string) => request<Schema<'Inspection'>>('/public/password-resets/inspect', 'POST', { token } satisfies Schema<'TokenInput'>),
    completeReset: (input: Schema<'ResetPasswordInput'>) => request<void>('/public/password-resets/complete', 'POST', input),
  };
}
