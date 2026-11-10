import type { components } from './schema';
export { Permissions, type PermissionCode } from './permissions.generated';
export type Role = components['schemas']['Role'];
export type Permission = components['schemas']['Permission'];
export type Authorization = components['schemas']['Authorization'];
export type AuthContext = components['schemas']['AuthContext'];
export type ManagedAccount = components['schemas']['ManagedAccount'];
export type AccessAuditEvent = components['schemas']['AccessAuditEvent'];
export type Tenant = components['schemas']['Tenant'];
export type TenantContext = components['schemas']['TenantContext'];
export type TenantAccount = components['schemas']['TenantAccount'];
export type MyTenant = components['schemas']['MyTenant'];
export type TenantAuditEvent = components['schemas']['TenantAuditEvent'];
export type TenantResult = components['schemas']['TenantMutationResult'];
export type TenantFilter = { cursor?: string; q?: string; limit?: number; administrator_unavailable?: boolean; event_type?: string; from?: string; to?: string };
export type AccessFilter = { tenant_id?: string; event_type?: string; cursor?: string; q?: string; status?: string; role_id?: string; object_id?: string; object_type?: string; from?: string; to?: string; limit?: number };


export class ApiError extends Error {
  constructor(message: string, public readonly status?: number, public readonly code?: string, public readonly fields: Record<string, string> = {}) {
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
export type SecurityEvent = components['schemas']['SecurityEvent'];
export type LinkResult = components['schemas']['Link'];

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];
function accountMethods(base: string, timeoutMs: number) {
  let csrfToken = '';
  let identityEpoch = 0;
  let csrfPending: Promise<void> | undefined;
  const listeners = new Set<() => void>();
  const authorizationListeners = new Set<(revision?: string) => void>();
  async function request<T>(path: string, method = 'GET', body?: unknown, notifyUnauthorized = true, signal?: AbortSignal): Promise<T> {
    const epoch = identityEpoch;
    const current = () => { if (epoch !== identityEpoch) throw new DOMException('Identity or authorization changed', 'AbortError'); };
    if (method !== 'GET' && !csrfToken) await csrf();
    current();
    let response: Response;
    try {
      response = await fetch(`${base}${path}`, {
        method, credentials: 'same-origin', cache: 'no-store', signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(timeoutMs)]) : AbortSignal.timeout(timeoutMs),
        headers: { Accept: 'application/json', ...(method === 'GET' ? {} : { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }) },
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      });
    } catch { signal?.throwIfAborted(); throw new ApiError('无法连接服务，请稍后重试。'); }
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
      const error = data && typeof data === 'object' ? data as Partial<Schema<'Error'>> : {};
      if (notifyUnauthorized && error.code === 'unauthenticated') listeners.forEach((listener) => listener());
      if (error.code === 'permission_denied' || error.code === 'tenant_not_found') authorizationListeners.forEach((listener) => listener());
      if (error.code === 'csrf_invalid') { csrfToken = ''; await csrf().catch(() => undefined); }
      throw new ApiError(typeof error.message === 'string' ? error.message : '请求失败，请稍后重试。', response.status, error.code, error.fields);
    }
    return data as T;
  }
  function csrf() {
    if (!csrfPending) csrfPending = request<Schema<'CSRF'>>('/auth/csrf').then((data) => { csrfToken = data.token; }).finally(() => { csrfPending = undefined; });
    return csrfPending;
  }
  const query = (filter: AccessFilter | TenantFilter = {}) => new URLSearchParams(Object.entries(filter).filter(([, v]) => v !== undefined && v !== '').map(([k, v]) => [k, String(v)])).toString();
  const idPath = (kind: string, id: string) => `/access/${kind}/${encodeURIComponent(id)}`;
  return {
    csrf,
    tenants: (filter?: TenantFilter, signal?: AbortSignal) => request<Schema<'Tenants'>>(`/platform/tenants?${query(filter)}`, 'GET', undefined, true, signal),
    myTenants: (filter?: TenantFilter, signal?: AbortSignal) => request<Schema<'MyTenants'>>(`/account/tenants?${query(filter)}`, 'GET', undefined, true, signal),
    accountTenants: (id: string, filter?: TenantFilter, signal?: AbortSignal) => request<Schema<'MyTenants'>>(`${idPath('accounts', id)}/tenant-memberships?${query(filter)}`, 'GET', undefined, true, signal),
    tenantCandidates: (filter: TenantFilter, signal?: AbortSignal) => request<Schema<'TenantCandidates'>>(`/platform/tenant-admin-candidates?${query(filter)}`, 'GET', undefined, true, signal),
    tenant: (id: string, platform: boolean, signal?: AbortSignal) => request<Schema<'Tenant'>>(platform ? `/platform/tenants/${encodeURIComponent(id)}` : `/tenants/${encodeURIComponent(id)}/profile`, 'GET', undefined, true, signal),
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
    invitations: (cursor = '', signal?: AbortSignal) => request<Schema<'Invitations'>>(`/invitations?cursor=${encodeURIComponent(cursor)}`, 'GET', undefined, true, signal),
    createInvitation: (note: string) => request<LinkResult>('/invitations', 'POST', { note } satisfies Schema<'InvitationInput'>),
    revokeInvitation: (id: string) => request<void>(`/invitations/${encodeURIComponent(id)}/revoke`, 'POST'),
    regenerateInvitation: (id: string) => request<LinkResult>(`/invitations/${encodeURIComponent(id)}/regenerate`, 'POST'),
    inspectInvitation: (token: string) => request<Schema<'Inspection'>>('/public/invitations/inspect', 'POST', { token } satisfies Schema<'TokenInput'>),
    acceptInvitation: (input: Schema<'AcceptInvitationInput'>) => request<void>('/public/invitations/accept', 'POST', input),
    inspectReset: (token: string) => request<Schema<'Inspection'>>('/public/password-resets/inspect', 'POST', { token } satisfies Schema<'TokenInput'>),
    completeReset: (input: Schema<'ResetPasswordInput'>) => request<void>('/public/password-resets/complete', 'POST', input),
  };
}
