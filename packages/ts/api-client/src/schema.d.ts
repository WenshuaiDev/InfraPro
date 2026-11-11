// Generated from contracts/openapi/api.yaml. Run pnpm contracts:generate.
export interface paths {
    "/healthz": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getLiveness"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/readyz": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getReadiness"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/public/account-help": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getAccountHelp"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/auth/csrf": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getCSRF"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/auth/login": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["login"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/auth/me": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["currentAuthorization"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/auth/reauthenticate": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["reauthenticate"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/auth/logout": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["logout"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/auth/logout-all": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["logoutAll"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/auth/activity": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["recordActivity"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/account/profile": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["readProfile"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch: operations["updateProfile"];
        trace?: never;
    };
    "/api/v1/account/password": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["changePassword"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/account/sessions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getSessions"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/account/sessions/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        delete: operations["revokeSession"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/account/security-events": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getSecurityEvents"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/invitations": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Owner-scoped cursor pagination. total counts all matches for q and status, before cursor. counts covers all invitations owned by this account regardless of q/status/cursor. Expired status is computed at read time. A cursor must be reused with the same filters. */
        get: operations["getInvitations"];
        put?: never;
        post: operations["createInvitation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/invitations/{id}/revoke": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["revokeInvitation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/invitations/{id}/regenerate": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["regenerateInvitation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/public/invitations/inspect": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["inspectInvitation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/public/invitations/accept": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["acceptInvitation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/public/password-resets/inspect": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["inspectReset"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/public/password-resets/complete": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["completeReset"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/access/permissions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["permissionCatalog"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/access/roles": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listRoles"];
        put?: never;
        post: operations["createRole"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/access/roles/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getRole"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch: operations["updateRole"];
        trace?: never;
    };
    "/api/v1/access/roles/{id}/permissions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put: operations["setRolePermissions"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/access/roles/{id}/status": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put: operations["setRoleStatus"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/access/roles/{id}/archive": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["archiveRole"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/access/accounts": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listAccounts"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/access/accounts/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getAccount"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/access/accounts/{id}/authorization": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["accountAuthorization"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/access/accounts/{id}/roles": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put: operations["assignRoles"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/access/accounts/{id}/disable": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["disableAccount"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/access/accounts/{id}/enable": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["enableAccount"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/access/audit-events": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["accessAudit"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/tenants": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listTenants"];
        put?: never;
        post: operations["createTenant"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/tenant-admin-candidates": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["tenantCandidates"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/account/tenants": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["myTenants"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/access/accounts/{id}/tenant-memberships": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["accountTenants"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/context": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["tenantContext"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/tenants/{tenant_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["platformTenant"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch: operations["updateplatformTenant"];
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/profile": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["tenantProfile"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch: operations["updatetenantProfile"];
        trace?: never;
    };
    "/api/v1/platform/tenants/{tenant_id}/administrator-transfer": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["transferTenant"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/audit-events": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["tenantAudit"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/members": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Live scoped authorization. All POST requests require Origin and CSRF. Management writes require recent password verification; recipient confirmation requires only an active matching account. No automatic write replay. */
        get: operations["tenantMembers"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/members/{membership_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Live scoped authorization. All POST requests require Origin and CSRF. Management writes require recent password verification; recipient confirmation requires only an active matching account. No automatic write replay. */
        get: operations["tenantMember"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/members/{membership_id}/suspend": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Live scoped authorization. All POST requests require Origin and CSRF. Management writes require recent password verification; recipient confirmation requires only an active matching account. No automatic write replay. */
        post: operations["suspendTenantMember"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/members/{membership_id}/resume": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Live scoped authorization. All POST requests require Origin and CSRF. Management writes require recent password verification; recipient confirmation requires only an active matching account. No automatic write replay. */
        post: operations["resumeTenantMember"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/members/{membership_id}/remove": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Live scoped authorization. All POST requests require Origin and CSRF. Management writes require recent password verification; recipient confirmation requires only an active matching account. No automatic write replay. */
        post: operations["removeTenantMember"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/member-candidates/resolve": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Live scoped authorization. All POST requests require Origin and CSRF. Management writes require recent password verification; recipient confirmation requires only an active matching account. No automatic write replay. */
        post: operations["resolveTenantMember"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/member-invitations": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Live scoped authorization. All POST requests require Origin and CSRF. Management writes require recent password verification; recipient confirmation requires only an active matching account. No automatic write replay. */
        get: operations["tenantInvitations"];
        put?: never;
        /** @description Live scoped authorization. All POST requests require Origin and CSRF. Management writes require recent password verification; recipient confirmation requires only an active matching account. No automatic write replay. */
        post: operations["createTenantInvitation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/member-invitations/{invitation_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Live scoped authorization. All POST requests require Origin and CSRF. Management writes require recent password verification; recipient confirmation requires only an active matching account. No automatic write replay. */
        get: operations["tenantInvitation"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/member-invitations/{invitation_id}/revoke": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Live scoped authorization. All POST requests require Origin and CSRF. Management writes require recent password verification; recipient confirmation requires only an active matching account. No automatic write replay. */
        post: operations["revokeTenantInvitation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/member-invitations/{invitation_id}/regenerate": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Live scoped authorization. All POST requests require Origin and CSRF. Management writes require recent password verification; recipient confirmation requires only an active matching account. No automatic write replay. */
        post: operations["regenerateTenantInvitation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/account/tenant-invitations/inspect": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Live scoped authorization. All POST requests require Origin and CSRF. Management writes require recent password verification; recipient confirmation requires only an active matching account. No automatic write replay. */
        post: operations["inspectTenantInvitation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/account/tenant-invitations/accept": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Live scoped authorization. All POST requests require Origin and CSRF. Management writes require recent password verification; recipient confirmation requires only an active matching account. No automatic write replay. */
        post: operations["acceptTenantInvitation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/permissions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        get: operations["tenantPermissions"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/roles": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        get: operations["tenantRoles"];
        put?: never;
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        post: operations["createTenantRole"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/roles/{role_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        get: operations["tenantRole"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        patch: operations["updateTenantRole"];
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/roles/{role_id}/members": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        get: operations["tenantRoleMembers"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/roles/{role_id}/copy": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        post: operations["copyTenantRole"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/roles/{role_id}/permissions/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        post: operations["previewTenantRolePermissions"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/roles/{role_id}/permissions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        put: operations["changeTenantRolePermissions"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/roles/{role_id}/status/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        post: operations["previewTenantRoleStatus"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/roles/{role_id}/status": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        put: operations["changeTenantRoleStatus"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/roles/{role_id}/archive/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        post: operations["previewTenantRoleArchive"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/roles/{role_id}/archive": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        post: operations["changeTenantRoleArchive"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/members/{membership_id}/authorization": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        get: operations["tenantMemberAuthorization"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/members/{membership_id}/roles/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        post: operations["previewTenantMemberRoles"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/members/{membership_id}/roles": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        put: operations["assignTenantMemberRoles"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/access/tenant-role-presets": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        get: operations["tenantRolePresets"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/access/tenant-role-presets/{role_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Live scoped authorization; writes require Origin, CSRF and recent password verification. Preview hashes are concurrency evidence, never authority. */
        get: operations["tenantRolePreset"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/tenants/{tenant_id}/disable/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Requires platform.tenant.read and platform.tenant.disable. Read-only impact; no successful lifecycle event. */
        post: operations["disableTenantPreview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/tenants/{tenant_id}/disable": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Requires platform.tenant.read and platform.tenant.disable. Recent password confirmation required. Atomically revalidates version and preview; no automatic retries. Errors: tenant_status_unchanged, version_conflict, preview_changed, tenant_restore_blocked. */
        post: operations["disableTenant"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/tenants/{tenant_id}/enable/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Requires platform.tenant.read and platform.tenant.enable. Read-only impact; no successful lifecycle event. */
        post: operations["enableTenantPreview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/tenants/{tenant_id}/enable": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Requires platform.tenant.read and platform.tenant.enable. Recent password confirmation required. Atomically revalidates version and preview; no automatic retries. Errors: tenant_status_unchanged, version_conflict, preview_changed, tenant_restore_blocked. */
        post: operations["enableTenant"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/account/tenants/{tenant_id}/availability": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Authenticated account and active membership; minimal public status only. Suspended/ended/unrelated memberships receive tenant_not_found. */
        get: operations["tenantAvailability"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/me/projects": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["MyProjects"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/project-governance": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectGovernanceList"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/project-governance/{project_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectGovernanceDetail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectCreate"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/project-candidates": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectCreateCandidates"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/project-governance/{project_id}/administrator-candidates": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectRecoveryCandidates"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/project-governance/{project_id}/administrator-recovery/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectRecoverAdminPreview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/project-governance/{project_id}/administrator-recovery": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectRecoverAdmin"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/context": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectContext"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/availability": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectAvailability"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/profile": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectProfile"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch: operations["ProjectUpdate"];
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/members": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectMembers"];
        put?: never;
        post: operations["ProjectMemberAdd"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/members/{membership_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectMember"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/members/{membership_id}/authorization": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectAuthorization"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/roles": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectRoles"];
        put?: never;
        post: operations["ProjectRoleCreate"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/roles/{role_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectRole"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch: operations["ProjectRoleUpdate"];
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/roles/{role_id}/members": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectRoleMembers"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/permissions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectPermissions"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/audit": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectAudit"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/member-candidates": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectMemberCandidates"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/owner-candidates": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectOwnerCandidates"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/administrator-candidates": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectAdministratorCandidates"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/members/{membership_id}/suspend/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectMemberSuspendPreview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/members/{membership_id}/suspend": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectMemberSuspend"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/members/{membership_id}/resume/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectMemberResumePreview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/members/{membership_id}/resume": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectMemberResume"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/members/{membership_id}/remove/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectMemberRemovePreview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/members/{membership_id}/remove": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectMemberRemove"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/owner/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectOwnerChangePreview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/owner": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectOwnerChange"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/administrator-transfer/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectTransferPreview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/administrator-transfer": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectTransfer"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/archive/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectArchivePreview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/archive": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectArchive"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/restore/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectRestorePreview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/restore": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectRestore"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/roles/{role_id}/permissions/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectRolePermissionsPreview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/roles/{role_id}/permissions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put: operations["ProjectRolePermissions"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/roles/{role_id}/status/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectRoleStatusPreview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/roles/{role_id}/status": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectRoleStatus"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/roles/{role_id}/delete/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectRoleDeletePreview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/roles/{role_id}/delete": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectRoleDelete"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/members/{membership_id}/roles/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectMemberRolesPreview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/members/{membership_id}/roles": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put: operations["ProjectMemberRoles"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/roles/{role_id}/copy": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["ProjectRoleCopy"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/access/project-role-presets": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectPresetList"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/access/project-role-presets/{role_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["ProjectPresetDetail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/tenants/{tenant_id}/administrator-transfer/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["TenantTransferPreview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/type-definitions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 list */
        get: operations["Engineering_type_definitions_list"];
        put?: never;
        /** 工程配置 create */
        post: operations["Engineering_type_definitions_create"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/type-definitions/{definition_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 detail */
        get: operations["Engineering_type_definitions_detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/type-definitions/{definition_id}/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 preview */
        post: operations["Engineering_type_definitions_preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/type-definitions/{definition_id}/diff": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 diff */
        get: operations["Engineering_type_definitions_diff"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/type-definitions/{definition_id}/versions/{version_id}/impact": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 impact */
        get: operations["Engineering_type_definitions_impact"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/type-definitions/{definition_id}/draft": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /** 工程配置 save */
        put: operations["Engineering_type_definitions_save"];
        /** 工程配置 new-draft */
        post: operations["Engineering_type_definitions_new_draft"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/type-definitions/{definition_id}/draft/delete": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 delete-draft */
        post: operations["Engineering_type_definitions_delete_draft"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/type-definitions/copy": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 copy */
        post: operations["Engineering_type_definitions_copy"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/type-definitions/{definition_id}/publish": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 publish */
        post: operations["Engineering_type_definitions_publish"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/type-definitions/{definition_id}/versions/{version_id}/state": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 state */
        post: operations["Engineering_type_definitions_state"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/catalog/type-definitions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 list */
        get: operations["TenantEngineeringCatalog_type_definitions_list"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/catalog/type-definitions/{definition_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 detail */
        get: operations["TenantEngineeringCatalog_type_definitions_detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/catalog/type-definitions/{definition_id}/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 preview */
        post: operations["TenantEngineeringCatalog_type_definitions_preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/catalog/type-definitions/{definition_id}/diff": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 diff */
        get: operations["TenantEngineeringCatalog_type_definitions_diff"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/metrics": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 list */
        get: operations["Engineering_metrics_list"];
        put?: never;
        /** 工程配置 create */
        post: operations["Engineering_metrics_create"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/metrics/{definition_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 detail */
        get: operations["Engineering_metrics_detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/metrics/{definition_id}/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 preview */
        post: operations["Engineering_metrics_preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/metrics/{definition_id}/diff": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 diff */
        get: operations["Engineering_metrics_diff"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/metrics/{definition_id}/versions/{version_id}/impact": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 impact */
        get: operations["Engineering_metrics_impact"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/metrics/{definition_id}/draft": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /** 工程配置 save */
        put: operations["Engineering_metrics_save"];
        /** 工程配置 new-draft */
        post: operations["Engineering_metrics_new_draft"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/metrics/{definition_id}/draft/delete": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 delete-draft */
        post: operations["Engineering_metrics_delete_draft"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/metrics/copy": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 copy */
        post: operations["Engineering_metrics_copy"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/metrics/{definition_id}/publish": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 publish */
        post: operations["Engineering_metrics_publish"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/metrics/{definition_id}/versions/{version_id}/state": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 state */
        post: operations["Engineering_metrics_state"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/catalog/metrics": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 list */
        get: operations["TenantEngineeringCatalog_metrics_list"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/catalog/metrics/{definition_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 detail */
        get: operations["TenantEngineeringCatalog_metrics_detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/catalog/metrics/{definition_id}/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 preview */
        post: operations["TenantEngineeringCatalog_metrics_preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/catalog/metrics/{definition_id}/diff": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 diff */
        get: operations["TenantEngineeringCatalog_metrics_diff"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/units": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 list */
        get: operations["Engineering_units_list"];
        put?: never;
        /** 工程配置 create */
        post: operations["Engineering_units_create"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/units/{definition_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 detail */
        get: operations["Engineering_units_detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/units/{definition_id}/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 preview */
        post: operations["Engineering_units_preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/units/{definition_id}/diff": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 diff */
        get: operations["Engineering_units_diff"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/units/{definition_id}/versions/{version_id}/impact": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 impact */
        get: operations["Engineering_units_impact"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/units/{definition_id}/draft": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /** 工程配置 save */
        put: operations["Engineering_units_save"];
        /** 工程配置 new-draft */
        post: operations["Engineering_units_new_draft"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/units/{definition_id}/draft/delete": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 delete-draft */
        post: operations["Engineering_units_delete_draft"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/units/copy": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 copy */
        post: operations["Engineering_units_copy"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/units/{definition_id}/publish": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 publish */
        post: operations["Engineering_units_publish"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/units/{definition_id}/versions/{version_id}/state": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 state */
        post: operations["Engineering_units_state"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/catalog/units": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 list */
        get: operations["TenantEngineeringCatalog_units_list"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/catalog/units/{definition_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 detail */
        get: operations["TenantEngineeringCatalog_units_detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/catalog/units/{definition_id}/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 preview */
        post: operations["TenantEngineeringCatalog_units_preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/catalog/units/{definition_id}/diff": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 diff */
        get: operations["TenantEngineeringCatalog_units_diff"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/monitoring-templates": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 list */
        get: operations["Engineering_monitoring_templates_list"];
        put?: never;
        /** 工程配置 create */
        post: operations["Engineering_monitoring_templates_create"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/monitoring-templates/{definition_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 detail */
        get: operations["Engineering_monitoring_templates_detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/monitoring-templates/{definition_id}/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 preview */
        post: operations["Engineering_monitoring_templates_preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/monitoring-templates/{definition_id}/diff": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 diff */
        get: operations["Engineering_monitoring_templates_diff"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/monitoring-templates/{definition_id}/versions/{version_id}/impact": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 impact */
        get: operations["Engineering_monitoring_templates_impact"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/monitoring-templates/{definition_id}/draft": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /** 工程配置 save */
        put: operations["Engineering_monitoring_templates_save"];
        /** 工程配置 new-draft */
        post: operations["Engineering_monitoring_templates_new_draft"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/monitoring-templates/{definition_id}/draft/delete": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 delete-draft */
        post: operations["Engineering_monitoring_templates_delete_draft"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/monitoring-templates/copy": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 copy */
        post: operations["Engineering_monitoring_templates_copy"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/monitoring-templates/{definition_id}/publish": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 publish */
        post: operations["Engineering_monitoring_templates_publish"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/monitoring-templates/{definition_id}/versions/{version_id}/state": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 state */
        post: operations["Engineering_monitoring_templates_state"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/monitoring-templates": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 list */
        get: operations["TenantEngineering_monitoring_templates_list"];
        put?: never;
        /** 工程配置 create */
        post: operations["TenantEngineering_monitoring_templates_create"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/monitoring-templates/{definition_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 detail */
        get: operations["TenantEngineering_monitoring_templates_detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/monitoring-templates/{definition_id}/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 preview */
        post: operations["TenantEngineering_monitoring_templates_preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/monitoring-templates/{definition_id}/diff": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 diff */
        get: operations["TenantEngineering_monitoring_templates_diff"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/monitoring-templates/{definition_id}/versions/{version_id}/impact": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 impact */
        get: operations["TenantEngineering_monitoring_templates_impact"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/monitoring-templates/{definition_id}/draft": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /** 工程配置 save */
        put: operations["TenantEngineering_monitoring_templates_save"];
        /** 工程配置 new-draft */
        post: operations["TenantEngineering_monitoring_templates_new_draft"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/monitoring-templates/{definition_id}/draft/delete": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 delete-draft */
        post: operations["TenantEngineering_monitoring_templates_delete_draft"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/monitoring-templates/copy": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 copy */
        post: operations["TenantEngineering_monitoring_templates_copy"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/monitoring-templates/{definition_id}/publish": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 publish */
        post: operations["TenantEngineering_monitoring_templates_publish"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/monitoring-templates/{definition_id}/versions/{version_id}/state": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 state */
        post: operations["TenantEngineering_monitoring_templates_state"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/catalog/monitoring-templates": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 list */
        get: operations["TenantEngineeringCatalog_monitoring_templates_list"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/catalog/monitoring-templates/{definition_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 detail */
        get: operations["TenantEngineeringCatalog_monitoring_templates_detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/catalog/monitoring-templates/{definition_id}/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 preview */
        post: operations["TenantEngineeringCatalog_monitoring_templates_preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/catalog/monitoring-templates/{definition_id}/diff": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 diff */
        get: operations["TenantEngineeringCatalog_monitoring_templates_diff"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/unit-categories": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 list */
        get: operations["EngineeringCategories_list"];
        put?: never;
        /** 工程配置 save */
        post: operations["EngineeringCategories_save"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/unit-categories/{category_key}/delete": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** 工程配置 delete */
        post: operations["EngineeringCategories_delete"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/unit-categories": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 工程配置 catalog */
        get: operations["EngineeringCategories_catalog"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/type-definitions/requests/{request_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 查询本人请求结果（复验当前原操作权限） */
        get: operations["Engineering_type_definitions_receipt"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/metrics/requests/{request_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 查询本人请求结果（复验当前原操作权限） */
        get: operations["Engineering_metrics_receipt"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/units/requests/{request_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 查询本人请求结果（复验当前原操作权限） */
        get: operations["Engineering_units_receipt"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/platform/engineering/monitoring-templates/requests/{request_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 查询本人请求结果（复验当前原操作权限） */
        get: operations["Engineering_monitoring_templates_receipt"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/monitoring-templates/requests/{request_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** 查询本人请求结果（复验当前原操作权限） */
        get: operations["TenantEngineering_monitoring_templates_receipt"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** facility list */
        get: operations["Resource:facility:list"];
        put?: never;
        /** facility create */
        post: operations["Resource:facility:create"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** facility detail */
        get: operations["Resource:facility:detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /** facility update */
        patch: operations["Resource:facility:update"];
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/retire": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** facility retire */
        post: operations["Resource:facility:retire"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/restore": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** facility restore */
        post: operations["Resource:facility:restore"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/delete": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** facility delete */
        post: operations["Resource:facility:delete"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/components": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** component list */
        get: operations["Resource:component:list"];
        put?: never;
        /** component create */
        post: operations["Resource:component:create"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/components/{component_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** component detail */
        get: operations["Resource:component:detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /** component update */
        patch: operations["Resource:component:update"];
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/components/{component_id}/retire": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** component retire */
        post: operations["Resource:component:retire"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/components/{component_id}/restore": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** component restore */
        post: operations["Resource:component:restore"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/components/{component_id}/delete": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** component delete */
        post: operations["Resource:component:delete"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/points": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** point list */
        get: operations["Resource:point:list"];
        put?: never;
        /** point create */
        post: operations["Resource:point:create"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/points/{point_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** point detail */
        get: operations["Resource:point:detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /** point update */
        patch: operations["Resource:point:update"];
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/retire": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** point retire */
        post: operations["Resource:point:retire"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/restore": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** point restore */
        post: operations["Resource:point:restore"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/delete": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** point delete */
        post: operations["Resource:point:delete"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/channels": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** channel list */
        get: operations["Resource:channel:list"];
        put?: never;
        /** channel create */
        post: operations["Resource:channel:create"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/channels/{channel_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** channel detail */
        get: operations["Resource:channel:detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /** channel update */
        patch: operations["Resource:channel:update"];
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/channels/{channel_id}/retire": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** channel retire */
        post: operations["Resource:channel:retire"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/channels/{channel_id}/restore": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** channel restore */
        post: operations["Resource:channel:restore"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/channels/{channel_id}/delete": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** channel delete */
        post: operations["Resource:channel:delete"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/changes": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** facility changes */
        get: operations["Resource:facility:changes"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/requests/{request_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** request detail */
        get: operations["Resource:request:detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/invitations/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Read one invitation owned by the current account. Requires platform.invitation.read. Unknown and foreign invitation IDs return 404. Never returns link credentials. */
        get: operations["getInvitation"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** facility list */
        get: operations["ProjectResource:facility:list"];
        put?: never;
        /** facility create */
        post: operations["ProjectResource:facility:create"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** facility detail */
        get: operations["ProjectResource:facility:detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /** facility update */
        patch: operations["ProjectResource:facility:update"];
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/mutation-preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Resource:facility:preview */
        post: operations["Resource:facility:preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}/mutation-preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** ProjectResource:facility:preview */
        post: operations["ProjectResource:facility:preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}/components": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** component list */
        get: operations["ProjectResource:component:list"];
        put?: never;
        /** component create */
        post: operations["ProjectResource:component:create"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}/components/{component_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** component detail */
        get: operations["ProjectResource:component:detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /** component update */
        patch: operations["ProjectResource:component:update"];
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/components/{component_id}/mutation-preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Resource:component:preview */
        post: operations["Resource:component:preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}/components/{component_id}/mutation-preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** ProjectResource:component:preview */
        post: operations["ProjectResource:component:preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}/components/{component_id}/retire": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** component retire */
        post: operations["ProjectResource:component:retire"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}/components/{component_id}/restore": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** component restore */
        post: operations["ProjectResource:component:restore"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}/points": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** point list */
        get: operations["ProjectResource:point:list"];
        put?: never;
        /** point create */
        post: operations["ProjectResource:point:create"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}/points/{point_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** point detail */
        get: operations["ProjectResource:point:detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /** point update */
        patch: operations["ProjectResource:point:update"];
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/mutation-preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Resource:point:preview */
        post: operations["Resource:point:preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/mutation-preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** ProjectResource:point:preview */
        post: operations["ProjectResource:point:preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/retire": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** point retire */
        post: operations["ProjectResource:point:retire"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/restore": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** point restore */
        post: operations["ProjectResource:point:restore"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/channels": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** channel list */
        get: operations["ProjectResource:channel:list"];
        put?: never;
        /** channel create */
        post: operations["ProjectResource:channel:create"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/channels/{channel_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** channel detail */
        get: operations["ProjectResource:channel:detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /** channel update */
        patch: operations["ProjectResource:channel:update"];
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/channels/{channel_id}/mutation-preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Resource:channel:preview */
        post: operations["Resource:channel:preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/channels/{channel_id}/mutation-preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** ProjectResource:channel:preview */
        post: operations["ProjectResource:channel:preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/channels/{channel_id}/retire": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** channel retire */
        post: operations["ProjectResource:channel:retire"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}/points/{point_id}/channels/{channel_id}/restore": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** channel restore */
        post: operations["ProjectResource:channel:restore"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/facilities/{facility_id}/changes": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** facility changes */
        get: operations["ProjectResource:facility:changes"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/resources/requests/{request_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** request detail */
        get: operations["ProjectResource:request:detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/resource-access": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** ProjectResourceAccess:access:detail */
        get: operations["ProjectResourceAccess:access:detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/resource-access/changes": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** ProjectResourceAccess:access:changes */
        get: operations["ProjectResourceAccess:access:changes"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/resource-access/policy/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** ProjectResourceAccess:policy:preview */
        post: operations["ProjectResourceAccess:policy:preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/resource-access/policy": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** ProjectResourceAccess:policy:change */
        post: operations["ProjectResourceAccess:policy:change"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/resource-access/facilities/{facility_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** ProjectResourceAccess:facility:detail */
        get: operations["ProjectResourceAccess:facility:detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/resource-access/facilities/{facility_id}/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** ProjectResourceAccess:facility:preview */
        post: operations["ProjectResourceAccess:facility:preview"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/resource-access/facilities/{facility_id}/changes": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** ProjectResourceAccess:facility:change */
        post: operations["ProjectResourceAccess:facility:change"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/resource-access/requests/{request_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** ProjectResourceAccess:request:detail */
        get: operations["ProjectResourceAccess:request:detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/catalog/type-definitions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** ProjectResource:type-definitions:list */
        get: operations["ProjectResource:type-definitions:list"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/catalog/type-definitions/versions/{version_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** ProjectResource:type-definitions:version */
        get: operations["ProjectResource:type-definitions:version"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/catalog/metrics": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** ProjectResource:metrics:list */
        get: operations["ProjectResource:metrics:list"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/catalog/metrics/versions/{version_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** ProjectResource:metrics:version */
        get: operations["ProjectResource:metrics:version"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/catalog/units": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** ProjectResource:units:list */
        get: operations["ProjectResource:units:list"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/projects/{project_id}/engineering/catalog/units/versions/{version_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** ProjectResource:units:version */
        get: operations["ProjectResource:units:version"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tenants/{tenant_id}/engineering/resources/facilities/{facility_id}/project-usage": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** List projects currently using a tenant facility, including archived projects */
        get: operations["ProjectResourceAccess:usage:detail"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
}
export type webhooks = Record<string, never>;
export interface components {
    schemas: {
        Error: {
            /** @description Membership errors: member_already_exists, member_suspended, invitation_pending_exists, invitation_already_accepted, invitation_not_pending, member_state_conflict, target_account_unavailable, tenant_admin_transfer_required, administrator_membership_suspended, tenant_invitation_unavailable, tenant_invitation_not_found, member_not_found, version_conflict, idempotency_conflict, operation_busy. */
            code: string;
            message: string;
            request_id: string;
            fields?: {
                [key: string]: string;
            };
            issues?: components["schemas"]["EngineeringIssue"][];
        };
        Account: {
            /** Format: uuid */
            id: string;
            username: string;
            display_name: string;
            /** @enum {string} */
            status: "active" | "disabled";
            /** Format: date-time */
            created_at: string;
        };
        AccountHelp: {
            support_contact: string;
            console_url: string;
            password_min: number;
            password_max: number;
            username_pattern: string;
        };
        CSRF: {
            token: string;
        };
        Session: {
            /** Format: uuid */
            id: string;
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            last_activity_at: string;
            /** Format: date-time */
            expires_at: string;
            user_agent: string;
            ip: string;
            current: boolean;
        };
        Invitation: {
            /** Format: uuid */
            id: string;
            note: string;
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            expires_at: string;
            /** @enum {string} */
            status: "pending" | "used" | "expired" | "revoked";
        };
        SecurityEvent: {
            /** Format: int64 */
            id: number;
            event_type: string;
            outcome: string;
            source_ip: string;
            user_agent: string;
            /** Format: date-time */
            created_at: string;
        };
        Link: {
            url: string;
            /** Format: date-time */
            expires_at: string;
        };
        Inspection: {
            /** Format: date-time */
            expires_at: string;
        };
        LoginInput: {
            username: string;
            password: string;
        };
        PasswordInput: {
            password: string;
        };
        ProfileInput: {
            display_name: string;
        };
        ChangePasswordInput: {
            current_password: string;
            new_password: string;
        };
        InvitationInput: {
            note?: string;
        };
        TokenInput: {
            token: string;
        };
        AcceptInvitationInput: {
            token: string;
            username: string;
            display_name?: string;
            password: string;
        };
        ResetPasswordInput: {
            token: string;
            password: string;
        };
        Sessions: {
            items: components["schemas"]["Session"][];
            next_cursor: string;
        };
        Invitations: {
            items: components["schemas"]["Invitation"][];
            next_cursor: string;
            total: number;
            counts: components["schemas"]["InvitationCounts"];
        };
        SecurityEvents: {
            items: components["schemas"]["SecurityEvent"][];
            next_cursor: string;
        };
        ProbeStatus: {
            /** @enum {string} */
            status: "ok";
        };
        /** @enum {string} */
        PermissionCode: "platform.tenant.disable" | "platform.tenant.enable" | "tenant.permission.read" | "tenant.role.read" | "tenant.role.create" | "tenant.role.update" | "tenant.role.set_permissions" | "tenant.role.set_status" | "tenant.role.delete" | "tenant.member.assign_roles" | "tenant.member.read" | "tenant.member.suspend" | "tenant.member.resume" | "tenant.member.remove" | "tenant.invitation.read" | "tenant.invitation.create" | "tenant.invitation.revoke" | "tenant.invitation.regenerate" | "platform.tenant.read" | "platform.tenant.create" | "platform.tenant.update" | "platform.tenant.transfer_admin" | "tenant.profile.read" | "tenant.profile.update" | "tenant.audit.read" | "account.profile.read" | "account.profile.update" | "account.password.update" | "account.session.read" | "account.session.revoke" | "account.security_event.read" | "platform.invitation.read" | "platform.invitation.create" | "platform.invitation.revoke" | "platform.invitation.regenerate" | "platform.account.read" | "platform.account.assign_roles" | "platform.account.disable" | "platform.account.enable" | "platform.role.read" | "platform.role.create" | "platform.role.update" | "platform.role.set_permissions" | "platform.role.set_status" | "platform.role.delete" | "platform.permission.read" | "platform.audit.read" | "tenant.project.read" | "tenant.project.create" | "tenant.project.recover_admin" | "project.profile.read" | "project.profile.update" | "project.member.read" | "project.member.add" | "project.member.suspend" | "project.member.resume" | "project.member.remove" | "project.role.read" | "project.permission.read" | "project.authorization.read" | "project.audit.read" | "project.role.create" | "project.role.update" | "project.role.set_permissions" | "project.role.set_status" | "project.role.delete" | "project.member.assign_roles" | "project.administrator.transfer" | "project.owner.change" | "project.archive" | "project.restore" | "engineering.type.read" | "engineering.type.edit" | "engineering.type.publish" | "engineering.type.state" | "engineering.metric.read" | "engineering.metric.edit" | "engineering.metric.publish" | "engineering.metric.state" | "engineering.template.read" | "engineering.template.edit" | "engineering.template.publish" | "engineering.template.state" | "tenant.engineering.template.read" | "tenant.engineering.template.edit" | "tenant.engineering.template.publish" | "tenant.engineering.template.state" | "tenant.engineering.catalog.read" | "tenant.engineering.resource.read" | "tenant.engineering.resource.manage" | "tenant.engineering.resource.authorization.read" | "tenant.engineering.resource.authorization.manage" | "project.engineering.resource.read" | "project.engineering.resource.update" | "project.engineering.resource.history.read" | "project.engineering.resource.lifecycle" | "project.engineering.facility.create" | "project.engineering.component.create" | "project.engineering.point.create" | "project.engineering.channel.create" | "project.engineering.catalog.read";
        Permission: {
            name: string;
            group: string;
            scope: string;
            description: string;
            risk: string;
            lifecycle: string;
            code: components["schemas"]["PermissionCode"];
            tenant_custom_role_assignable: boolean;
            project_custom_role_assignable: boolean;
        };
        Role: {
            id: string;
            name: string;
            description: string;
            builtin_key: string;
            /** @enum {string} */
            status: "active" | "disabled";
            version: string;
            permission_codes: components["schemas"]["PermissionCode"][];
            account_count: number;
            /** Format: date-time */
            created_at: string;
            /** @enum {string} */
            binding_scope: "platform" | "tenant" | "project";
            owner_tenant_id: string;
            source_role_id: string;
            source_role_version: string;
            assignment_policy: string;
            owner_project_id: string;
        };
        Authorization: {
            authorization_revision: string;
            permissions: components["schemas"]["PermissionCode"][];
            roles: components["schemas"]["Role"][];
            sources: {
                [key: string]: string[];
            };
        };
        AuthContext: {
            account: components["schemas"]["Account"];
            permissions: components["schemas"]["PermissionCode"][];
            authorization_revision: string;
        };
        Profile: {
            account: components["schemas"]["Account"];
            authorization: components["schemas"]["Authorization"];
        };
        ManagedAccount: {
            /** Format: uuid */
            id: string;
            username: string;
            display_name: string;
            /** @enum {string} */
            status: "active" | "disabled";
            /** Format: date-time */
            created_at: string;
            management_version: string;
            roles: components["schemas"]["Role"][];
        };
        AccessAuditEvent: {
            id: string;
            event_type: string;
            actor_type: string;
            actor_id: string;
            account_id: string;
            object_type: string;
            object_id: string;
            reason: string;
            request_id: string;
            created_at: string;
            details: {
                [key: string]: unknown;
            };
            tenant_id: string;
            actor_scope: string;
            project_id: string;
            display?: components["schemas"]["AuditDisplay"];
        };
        Roles: {
            items: components["schemas"]["Role"][];
            next_cursor: string;
        };
        ManagedAccounts: {
            items: components["schemas"]["ManagedAccount"][];
            next_cursor: string;
        };
        AccessAuditEvents: {
            items: components["schemas"]["AccessAuditEvent"][];
            next_cursor: string;
        };
        AccessMutation: {
            reason: string;
            expected_version: string;
        };
        CreateRoleInput: {
            name: string;
            description: string;
            reason: string;
        };
        UpdateRoleInput: {
            reason: string;
            expected_version: string;
            name: string;
            description: string;
        };
        RolePermissionsInput: {
            reason: string;
            expected_version: string;
            permission_codes: components["schemas"]["PermissionCode"][];
        };
        RoleStatusInput: {
            reason: string;
            expected_version: string;
            /** @enum {string} */
            status: "active" | "disabled";
        };
        AccountRolesInput: {
            reason: string;
            expected_version: string;
            role_ids: string[];
        };
        Permissions: components["schemas"]["Permission"][];
        TenantAccount: {
            /** Format: uuid */
            id: string;
            username: string;
            display_name: string;
        };
        TenantSummary: {
            /** Format: uuid */
            id: string;
            name: string;
            code: string;
        };
        Tenant: {
            /** Format: uuid */
            id: string;
            name: string;
            code: string;
            description: string;
            administrator: components["schemas"]["TenantAccount"];
            administrator_available: boolean;
            /** Format: uuid */
            administrator_membership_id: string;
            version: string;
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            updated_at: string;
            /** Format: uuid */
            created_by: string;
            /** Format: uuid */
            updated_by: string;
            /** @enum {string} */
            status: "active" | "disabled";
            /** Format: date-time */
            status_changed_at: string | null;
            /** Format: uuid */
            status_changed_by: string | null;
            status_change_reason: string | null;
            status_notice: string;
        };
        TenantContext: {
            tenant: components["schemas"]["TenantSummary"];
            /** Format: uuid */
            membership_id: string;
            roles: components["schemas"]["TenantRole"][];
            permissions: components["schemas"]["PermissionCode"][];
            permission_sources: {
                [key: string]: string[];
            };
            authorization_revision: string;
            authorization: components["schemas"]["TenantAuthorization"];
        };
        MyTenant: {
            /** Format: uuid */
            id: string;
            name: string;
            code: string;
            /** Format: uuid */
            membership_id: string;
            roles: components["schemas"]["TenantRole"][];
            /** @enum {string} */
            status: "active" | "disabled";
            can_enter: boolean;
            restriction_code: string;
        };
        CreateTenantInput: {
            name: string;
            /** @description Trim and lowercase ASCII; first character alphanumeric. */
            code: string;
            description?: string;
            reason: string;
            /** Format: uuid */
            administrator_account_id: string;
            /** Format: uuid */
            client_request_id: string;
        };
        UpdateTenantInput: {
            name?: string;
            /** @description Trim and lowercase ASCII; first character alphanumeric. */
            code?: string;
            description?: string;
            reason: string;
            expected_version: string;
        } | unknown | unknown | unknown;
        TransferTenantInput: {
            expected_version: string;
            reason: string;
            /** Format: uuid */
            new_administrator_account_id: string;
            project_preview_hash?: string;
            confirm_project_impact?: boolean;
        };
        TenantMutationResult: {
            /** Format: uuid */
            tenant_id: string;
            version: string;
            authorization_revision: string;
            replayed: boolean;
        };
        TenantAuditEvent: {
            id: string;
            event_type: string;
            actor_type: string;
            actor_id: string;
            reason: string;
            request_id: string;
            created_at: string;
            details: {
                [key: string]: unknown;
            };
            tenant_id: string;
            actor_scope: string;
        };
        Tenants: {
            items: components["schemas"]["Tenant"][];
            next_cursor: string;
        };
        MyTenants: {
            items: components["schemas"]["MyTenant"][];
            next_cursor: string;
        };
        TenantCandidates: {
            items: components["schemas"]["TenantAdministratorCandidate"][];
            next_cursor: string;
        };
        TenantAuditEvents: {
            items: components["schemas"]["TenantAuditEvent"][];
            next_cursor: string;
        };
        TenantRole: {
            /** Format: uuid */
            id: string;
            name: string;
            /** @enum {string} */
            binding_scope: "tenant";
        };
        TenantProfile: {
            /** Format: uuid */
            id: string;
            name: string;
            code: string;
            description: string;
            administrator: components["schemas"]["TenantAccount"];
            administrator_available: boolean;
            version: string;
            /** Format: date-time */
            updated_at: string;
        };
        TenantRoleGrant: {
            /** Format: uuid */
            binding_id: string;
            name: string;
            key: string;
            /** Format: date-time */
            assigned_at: string;
            /** Format: uuid */
            assigned_by: string;
            /** Format: date-time */
            revoked_at: string | null;
            revoked_by: string | null;
            role_id: string;
            status: string;
            version: string;
            owner_tenant_id: string;
            permission_codes: components["schemas"]["PermissionCode"][];
        };
        TenantMember: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            tenant_id: string;
            account: components["schemas"]["TenantAccount"];
            /** @enum {string} */
            membership_status: "active" | "suspended" | "ended";
            account_available: boolean;
            is_current_administrator: boolean;
            roles: components["schemas"]["TenantRoleGrant"][];
            role_effective: boolean;
            /** Format: date-time */
            joined_at: string;
            /** Format: uuid */
            joined_by: string;
            /** Format: date-time */
            status_changed_at: string;
            /** Format: uuid */
            status_changed_by: string;
            status_reason: string;
            version: string;
            /** Format: date-time */
            ended_at: string | null;
            end_reason: string;
            source_invitation_id: string;
        };
        TenantMembers: {
            items: components["schemas"]["TenantMember"][];
            next_cursor: string;
        };
        TenantMemberMutationInput: {
            expected_version: string;
            reason: string;
        };
        TenantMembershipResult: {
            /** Format: uuid */
            tenant_id: string;
            /** Format: uuid */
            membership_id: string;
            membership_status: string;
            membership_version: string;
            authorization_revision: string;
        };
        TenantInvitationPreview: {
            tenant: components["schemas"]["TenantSummary"];
            account: components["schemas"]["TenantAccount"];
            inviter: components["schemas"]["TenantAccount"];
            role: string;
        };
        TenantInvitation: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            tenant_id: string;
            target: components["schemas"]["TenantAccount"];
            inviter: components["schemas"]["TenantAccount"];
            /** Format: uuid */
            inviter_membership_id: string;
            /** @enum {string} */
            role: "tenant_member";
            /** @enum {string} */
            status: "pending" | "accepted" | "expired" | "revoked" | "replaced" | "invalidated";
            version: string;
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            expires_at: string;
            /** Format: date-time */
            closed_at: string | null;
            closed_by: string;
            close_reason: string;
            /** Format: date-time */
            accepted_at: string | null;
            accepted_membership_id: string;
            membership_status: string;
            replaces_id: string;
            replaced_by_id: string;
        };
        TenantInvitations: {
            items: components["schemas"]["TenantInvitation"][];
            next_cursor: string;
        };
        TenantInvitationCreated: {
            invitation: components["schemas"]["TenantInvitation"];
            link?: string;
            replayed: boolean;
            link_unavailable: boolean;
        };
        CreateTenantInvitationInput: {
            /** Format: uuid */
            target_account_id: string;
            reason: string;
            /** Format: uuid */
            client_request_id: string;
        };
        RegenerateTenantInvitationInput: {
            expected_version: string;
            reason: string;
            /** Format: uuid */
            client_request_id: string;
        };
        ResolveTenantMemberInput: {
            username: string;
        };
        TenantInvitationInspection: {
            /** Format: uuid */
            invitation_id: string;
            status: string;
            version: string;
            can_accept: boolean;
            /** Format: date-time */
            expires_at: string;
            preview?: components["schemas"]["TenantInvitationPreview"];
            membership_id: string;
            membership_status: string;
            tenant_status: string;
            blocked_reason: string;
            availability?: components["schemas"]["TenantAvailability"];
        };
        AcceptTenantInvitationInput: {
            token: string;
            expected_version: string;
        };
        TenantAcceptanceResult: {
            /** Format: uuid */
            tenant_id: string;
            /** Format: uuid */
            membership_id: string;
            membership_version: string;
            /** Format: uuid */
            invitation_id: string;
            invitation_version: string;
            authorization_revision: string;
        };
        TenantAdministratorCandidate: {
            /** Format: uuid */
            id: string;
            username: string;
            display_name: string;
            membership_status?: string;
            /** @enum {string} */
            transfer_mode?: "new" | "reuse" | "resume_required";
            replaced_roles: string[];
        };
        TenantRoleDefinition: {
            id: string;
            name: string;
            description: string;
            builtin_key: string;
            /** @enum {string} */
            status: "active" | "disabled";
            version: string;
            permission_codes: components["schemas"]["PermissionCode"][];
            account_count: number;
            /** Format: date-time */
            created_at: string;
            /** @enum {string} */
            binding_scope: "platform" | "tenant";
            owner_tenant_id: string;
            source_role_id: string;
            source_role_version: string;
            assignment_policy: string;
            member_count: number;
            effective_member_count: number;
            can_edit: boolean;
            can_copy: boolean;
        };
        TenantPermission: {
            name: string;
            group: string;
            scope: string;
            description: string;
            risk: string;
            lifecycle: string;
            code: components["schemas"]["PermissionCode"];
            tenant_custom_role_assignable: boolean;
            selectable: boolean;
            unavailable_reason: string;
        };
        TenantAuthorization: {
            authorization_revision: string;
            permissions: components["schemas"]["PermissionCode"][];
            roles: components["schemas"]["Role"][];
            sources: {
                [key: string]: string[];
            };
            membership_id: string;
            membership_version: string;
            membership_status: string;
            account_id: string;
            account_status: string;
            username: string;
            display_name: string;
            is_current_administrator: boolean;
            configured_permissions: components["schemas"]["PermissionCode"][];
            restorable_permissions: components["schemas"]["PermissionCode"][];
            inactive_reasons: string[];
            /** @enum {string} */
            tenant_status: "active" | "disabled";
        };
        TenantMemberImpact: {
            before: components["schemas"]["TenantAuthorization"];
            after: components["schemas"]["TenantAuthorization"];
            added_permissions: components["schemas"]["PermissionCode"][];
            removed_permissions: components["schemas"]["PermissionCode"][];
            retained_permissions: components["schemas"]["PermissionCode"][];
            added_role_ids: string[];
            removed_role_ids: string[];
            invalidated_invitations: number;
            invitation_grant_lost: boolean;
        };
        TenantRoleDefinitions: {
            items: components["schemas"]["TenantRoleDefinition"][];
            next_cursor: string;
        };
        TenantRoleMembers: {
            items: components["schemas"]["TenantAuthorization"][];
            next_cursor: string;
        };
        TenantImpactMembers: {
            items: components["schemas"]["TenantMemberImpact"][];
            next_cursor: string;
        };
        TenantAuthorizationImpact: {
            preview_hash: string;
            catalog_checksum: string;
            target_version: string;
            role_versions: {
                [key: string]: string;
            };
            bound_members: number;
            effective_members: number;
            gaining_members: number;
            losing_members: number;
            restorable_members: number;
            invalidated_invitations: number;
            blocked_reasons: string[];
            members: components["schemas"]["TenantImpactMembers"];
        };
        TenantRoleResult: {
            role: components["schemas"]["TenantRoleDefinition"];
            authorization_revision: string;
            replayed: boolean;
        };
        TenantAuthorizationResult: {
            role?: components["schemas"]["TenantRoleDefinition"];
            authorization?: components["schemas"]["TenantAuthorization"];
            impact: components["schemas"]["TenantAuthorizationImpact"];
            authorization_revision: string;
        };
        TenantRoleCreateInput: {
            name: string;
            description?: string;
            permission_codes: components["schemas"]["PermissionCode"][];
            /** Format: uuid */
            client_request_id: string;
            reason: string;
            source_version?: string;
        };
        TenantRoleUpdateInput: {
            name: string;
            description: string;
            expected_version: string;
            reason: string;
        };
        TenantRolePermissionsInput: {
            expected_version: string;
            permission_codes: components["schemas"]["PermissionCode"][];
            preview_hash: string;
            reason: string;
        };
        TenantRolePermissionsPreviewInput: {
            expected_version: string;
            permission_codes: components["schemas"]["PermissionCode"][];
        };
        TenantRoleStatusInput: {
            expected_version: string;
            /** @enum {string} */
            status: "active" | "disabled";
            preview_hash: string;
            reason: string;
        };
        TenantRoleStatusPreviewInput: {
            expected_version: string;
            /** @enum {string} */
            status: "active" | "disabled";
        };
        TenantRoleArchiveInput: {
            expected_version: string;
            preview_hash: string;
            reason: string;
        };
        TenantRoleArchivePreviewInput: {
            expected_version: string;
        };
        TenantMemberRolesInput: {
            expected_membership_version: string;
            role_ids: string[];
            role_versions: {
                [key: string]: string;
            };
            preview_hash: string;
            reason: string;
        };
        TenantMemberRolesPreviewInput: {
            expected_membership_version: string;
            role_ids: string[];
        };
        TenantAvailability: {
            /** Format: uuid */
            id: string;
            name: string;
            code: string;
            membership_id?: string;
            /** @enum {string} */
            status: "active" | "disabled";
            /** Format: date-time */
            status_changed_at: string | null;
            status_notice: string;
            can_enter: boolean;
            restriction_code: string;
        };
        TenantStatusImpact: {
            active_members: number;
            suspended_members: number;
            current_business_members: number;
            eligible_members: number;
            unavailable_accounts: number;
            no_permissions: number;
            pending_invitations: number;
        };
        TenantStatusPreview: {
            /** Format: uuid */
            tenant_id: string;
            name: string;
            code: string;
            /** @enum {string} */
            current_status: "active" | "disabled";
            /** @enum {string} */
            target_status: "active" | "disabled";
            expected_version: string;
            authorization_revision: string;
            catalog_checksum: string;
            preview_hash: string;
            /** Format: date-time */
            generated_at: string;
            impact: components["schemas"]["TenantStatusImpact"];
            administrator: components["schemas"]["TenantAccount"];
            administrator_available: boolean;
            administrator_valid: boolean;
            can_submit: boolean;
            blocked_reasons: string[];
        };
        TenantEnableInput: {
            expected_version: string;
            preview_hash: string;
            reason: string;
        };
        TenantDisableInput: {
            expected_version: string;
            preview_hash: string;
            reason: string;
            tenant_notice: string;
        };
        TenantStatusResult: {
            /** Format: uuid */
            tenant_id: string;
            /** @enum {string} */
            status: "active" | "disabled";
            version: string;
            /** Format: date-time */
            status_changed_at: string | null;
            authorization_revision: string;
        };
        ProjectPerson: {
            tenant_membership_id: string;
            account_id: string;
            username: string;
            display_name: string;
            available: boolean;
        };
        ProjectSummary: {
            id: string;
            tenant_id: string;
            tenant_name: string;
            name: string;
            code: string;
            status: string;
            version: string;
            administrator_membership_id: string;
            planned_start: string;
            planned_end: string;
            created_at: string;
            updated_at: string;
            owner: components["schemas"]["ProjectPerson"];
            administrator: components["schemas"]["ProjectPerson"];
            management_gap: boolean;
        };
        ProjectProfile: {
            id: string;
            tenant_id: string;
            tenant_name: string;
            name: string;
            code: string;
            status: string;
            version: string;
            administrator_membership_id: string;
            planned_start: string;
            planned_end: string;
            created_at: string;
            updated_at: string;
            owner: components["schemas"]["ProjectPerson"];
            administrator: components["schemas"]["ProjectPerson"];
            management_gap: boolean;
            purpose: string;
            description: string;
            created_by: string;
            updated_by: string;
            archived_at: string;
            archived_by: string;
            archive_reason: string;
            management_gap_reason: string;
        };
        ProjectBinding: {
            id: string;
            role_id: string;
            role_name: string;
            assigned_at: string;
            revoked_at: string;
        };
        ProjectAuthorization: {
            authorization_revision: string;
            permissions: components["schemas"]["PermissionCode"][];
            roles: components["schemas"]["Role"][];
            sources: {
                [key: string]: string[];
            };
            membership_id: string;
            membership_version: string;
            membership_status: string;
            account_id: string;
            account_status: string;
            username: string;
            display_name: string;
            is_current_administrator: boolean;
            configured_permissions: components["schemas"]["PermissionCode"][];
            restorable_permissions: components["schemas"]["PermissionCode"][];
            inactive_reasons: string[];
            /** @enum {string} */
            tenant_status: "active" | "disabled";
            tenant_id: string;
            project_id: string;
            tenant_membership_id: string;
            tenant_membership_status: string;
            project_status: string;
            allowed_operations: string[];
            bindings: components["schemas"]["ProjectBinding"][];
        };
        ProjectContext: {
            project: components["schemas"]["ProjectSummary"];
            authorization: components["schemas"]["ProjectAuthorization"];
            can_enter: boolean;
            resource_access_revision: string;
            can_create_facility: boolean;
        };
        ProjectMember: {
            id: string;
            tenant_membership_id: string;
            account_id: string;
            username: string;
            display_name: string;
            account_status: string;
            tenant_membership_status: string;
            status: string;
            version: string;
            joined_at: string;
            status_reason: string;
            ended_at: string;
            is_current_administrator: boolean;
        };
        ProjectResult: {
            project_id: string;
            object_id: string;
            version: string;
            project_version: string;
            authorization_revision: string;
            replayed: boolean;
        };
        ProjectMemberImpact: {
            membership_id: string;
            display_name: string;
            status: string;
            added: components["schemas"]["PermissionCode"][];
            removed: components["schemas"]["PermissionCode"][];
            restorable_added: components["schemas"]["PermissionCode"][];
            restorable_removed: components["schemas"]["PermissionCode"][];
            before: components["schemas"]["ProjectAuthorization"];
            after: components["schemas"]["ProjectAuthorization"];
        };
        ProjectPreview: {
            expected_version: string;
            expected_project_version: string;
            preview_hash: string;
            catalog_checksum: string;
            blocked_reason: string;
            can_submit: boolean;
            before: unknown;
            after: unknown;
            members: components["schemas"]["ProjectMemberImpact"][];
            member_count: number;
            losing_members: number;
            role_versions: {
                [key: string]: string;
            };
            notes: string[];
        };
        ProjectCreateInput: {
            name: string;
            code: string;
            purpose: string;
            description?: string;
            planned_start?: string;
            planned_end?: string;
            /** Format: uuid */
            client_request_id: string;
            reason: string;
            /** Format: uuid */
            owner_tenant_membership_id: string;
            /** Format: uuid */
            administrator_tenant_membership_id: string;
        };
        ProjectUpdateInput: {
            name: string;
            code: string;
            purpose: string;
            description?: string;
            planned_start?: string;
            planned_end?: string;
            /** Format: uuid */
            client_request_id: string;
            reason: string;
            expected_version: string;
            expected_project_version: string;
            preview_hash?: string;
        };
        ProjectAddMemberInput: {
            /** Format: uuid */
            client_request_id: string;
            reason: string;
            expected_project_version: string;
            /** Format: uuid */
            tenant_membership_id: string;
        };
        ProjectRoleInput: {
            /** Format: uuid */
            client_request_id: string;
            reason: string;
            expected_version?: string;
            expected_project_version: string;
            preview_hash?: string;
            name: string;
            description?: string;
            permission_codes: components["schemas"]["PermissionCode"][];
            source_version?: string;
        };
        ProjectRoleUpdateInput: {
            /** Format: uuid */
            client_request_id: string;
            reason: string;
            expected_version: string;
            expected_project_version: string;
            preview_hash?: string;
            name: string;
            description: string;
        };
        ProjectChangeInput: {
            /** Format: uuid */
            client_request_id: string;
            reason: string;
            expected_version: string;
            expected_project_version: string;
            preview_hash: string;
        };
        ProjectCandidateInput: {
            /** Format: uuid */
            client_request_id: string;
            reason: string;
            expected_version: string;
            expected_project_version: string;
            preview_hash: string;
            /** Format: uuid */
            tenant_membership_id: string;
        };
        ProjectCandidatePreviewInput: {
            /** Format: uuid */
            client_request_id?: string;
            reason?: string;
            expected_version?: string;
            expected_project_version?: string;
            preview_hash?: string;
            /** Format: uuid */
            tenant_membership_id: string;
        };
        ProjectPermissionsInput: {
            /** Format: uuid */
            client_request_id: string;
            reason: string;
            expected_version: string;
            expected_project_version: string;
            preview_hash: string;
            permission_codes: components["schemas"]["PermissionCode"][];
        };
        ProjectPermissionsPreviewInput: {
            /** Format: uuid */
            client_request_id?: string;
            reason?: string;
            expected_version?: string;
            expected_project_version?: string;
            preview_hash?: string;
            permission_codes: components["schemas"]["PermissionCode"][];
        };
        ProjectRolesInput: {
            /** Format: uuid */
            client_request_id: string;
            reason: string;
            expected_version: string;
            expected_project_version: string;
            preview_hash: string;
            role_ids: string[];
            role_versions: {
                [key: string]: string;
            };
        };
        ProjectRolesPreviewInput: {
            /** Format: uuid */
            client_request_id?: string;
            reason?: string;
            expected_version?: string;
            expected_project_version?: string;
            preview_hash?: string;
            role_ids: string[];
            role_versions?: {
                [key: string]: string;
            };
        };
        ProjectStatusInput: {
            /** Format: uuid */
            client_request_id: string;
            reason: string;
            expected_version: string;
            expected_project_version: string;
            preview_hash: string;
            /** @enum {string} */
            status: "active" | "disabled";
        };
        ProjectStatusPreviewInput: {
            /** Format: uuid */
            client_request_id?: string;
            reason?: string;
            expected_version?: string;
            expected_project_version?: string;
            preview_hash?: string;
            /** @enum {string} */
            status: "active" | "disabled";
        };
        ProjectEmptyPreviewInput: {
            /** Format: uuid */
            client_request_id?: string;
            reason?: string;
            expected_version?: string;
            expected_project_version?: string;
            preview_hash?: string;
        };
        Projects: {
            items: components["schemas"]["ProjectContext"][];
            next_cursor: string;
        };
        ProjectCandidates: {
            items: components["schemas"]["ProjectPerson"][];
            next_cursor: string;
        };
        ProjectMembers: {
            items: components["schemas"]["ProjectMember"][];
            next_cursor: string;
        };
        ProjectRoles: {
            items: components["schemas"]["Role"][];
            next_cursor: string;
        };
        ProjectAuditEvents: {
            items: components["schemas"]["AccessAuditEvent"][];
            next_cursor: string;
        };
        TenantProjectTransferPreview: {
            projects: {
                project_id: string;
                name: string;
                version: string;
                membership_id: string;
                administrator: boolean;
            }[];
            project_preview_hash: string;
            expected_version: string;
            requires_confirmation: boolean;
            administrator_disabled: boolean;
        };
        /** @description Read-only display context. Name sources distinguish historical snapshots from current names. Does not modify persisted details. */
        AuditDisplay: {
            category: string;
            outcome: string;
            actor_name: string;
            actor_username: string;
            actor_name_source: string;
            target_name: string;
            target_username: string;
            target_name_source: string;
            tenant_name: string;
            source_ip: string;
            user_agent: string;
            permission_names: {
                [key: string]: string;
            };
            role_names: {
                [key: string]: string;
            };
        };
        EngineeringBinding: {
            max_count?: number;
            metric_version: string;
            min_count: number;
            /** @enum {string} */
            resource_kind: "observation_channel";
            unit_version: string;
        };
        EngineeringCategory: {
            description: string;
            key: string;
            name: string;
            revision: number;
            used: boolean;
        };
        EngineeringCategoryInput: {
            description: string;
            key: string;
            name: string;
            reason: string;
            request_id: string;
            revision: number;
        };
        EngineeringComponent: {
            description: string;
            fields: components["schemas"]["EngineeringField"][];
            key: string;
            name: string;
        };
        EngineeringContent: {
            components?: components["schemas"]["EngineeringComponent"][];
            description: string;
            fields?: components["schemas"]["EngineeringField"][];
            metamodel_version: string;
            metric?: components["schemas"]["EngineeringMetric"];
            name: string;
            relations?: components["schemas"]["EngineeringRelation"][];
            release_notes: string;
            template?: components["schemas"]["EngineeringTemplate"];
            unit?: components["schemas"]["EngineeringUnit"];
        };
        EngineeringCopyInput: {
            code: string;
            reason: string;
            request_id: string;
            source_version: string;
        };
        EngineeringCreateInput: {
            code: string;
            content: components["schemas"]["EngineeringContent"];
            reason: string;
            request_id: string;
        };
        EngineeringDefinition: {
            code: string;
            /** Format: date-time */
            created_at: string;
            dependencies: components["schemas"]["EngineeringVersion"][];
            draft: components["schemas"]["EngineeringDraft"] | null;
            id: string;
            /** @enum {string} */
            kind: "type-definitions" | "metrics" | "units" | "monitoring-templates";
            revision: number;
            source?: components["schemas"]["EngineeringSource"];
            source_version: string;
            tenant_id: string;
            versions: components["schemas"]["EngineeringVersion"][];
        };
        EngineeringDifference: {
            after: unknown;
            before: unknown;
            path: string;
        };
        EngineeringDraft: {
            base_version: string;
            content: components["schemas"]["EngineeringContent"];
            revision: number;
            /** Format: date-time */
            updated_at: string;
        };
        EngineeringField: {
            default?: unknown;
            description: string;
            group: string;
            key: string;
            max_items?: number;
            max_length?: number;
            maximum?: number;
            min_items?: number;
            min_length?: number;
            minimum?: number;
            multiline: boolean;
            name: string;
            options?: components["schemas"]["EngineeringOption"][];
            order: number;
            precision?: number;
            required: boolean;
            /** @enum {string} */
            shape: "text" | "integer" | "decimal" | "boolean" | "date" | "datetime" | "enum" | "multi_enum";
            unit_version?: string;
        };
        EngineeringGroup: {
            description: string;
            key: string;
            name: string;
            order: number;
        };
        EngineeringImpact: {
            private_references: boolean;
            public: components["schemas"]["EngineeringSummary"][];
            /** @description 是否存在租户实际资源直接或间接引用（包括保留历史）；不返回名称、身份或计数。 */
            resource_references: boolean;
        };
        EngineeringIssue: {
            code: string;
            key: string;
            message: string;
            path: string;
            severity: string;
        };
        EngineeringItem: {
            bindings: components["schemas"]["EngineeringBinding"][];
            description: string;
            enabled: boolean;
            group: string;
            key: string;
            name: string;
            order: number;
            targets: components["schemas"]["EngineeringTarget"][];
        };
        EngineeringMetric: {
            allowed_units: string[];
            category: string;
            maximum?: number;
            minimum?: number;
            options: components["schemas"]["EngineeringOption"][];
            /** @enum {string} */
            shape: "integer" | "decimal" | "enum";
            standard_unit: string;
        };
        EngineeringNewDraftInput: {
            reason: string;
            request_id: string;
            revision: number;
            version_id: string;
        };
        EngineeringNode: {
            category: string;
            id: string;
            parent: string;
            values: {
                [key: string]: unknown;
            };
        };
        EngineeringOption: {
            key: string;
            name: string;
        };
        EngineeringPage: {
            items: components["schemas"]["EngineeringSummary"][];
            limit: number;
            offset: number;
            total: number;
        };
        EngineeringPreview: {
            content: components["schemas"]["EngineeringContent"];
            dependencies: components["schemas"]["EngineeringVersion"][];
            issues: components["schemas"]["EngineeringIssue"][];
            sample_only: boolean;
            valid: boolean;
        };
        EngineeringPreviewInput: {
            revision: number;
            sample?: components["schemas"]["EngineeringSample"];
            type_version: string;
            unit_version: string;
            value?: unknown;
            version_id: string;
        };
        EngineeringRelation: {
            child: string;
            max_count?: number;
            min_count: number;
            order: number;
            parent: string;
        };
        EngineeringResult: {
            definition_id: string;
            issues: components["schemas"]["EngineeringIssue"][];
            replayed: boolean;
            revision: number;
            version_id: string;
        };
        EngineeringRevisionInput: {
            reason: string;
            request_id: string;
            revision: number;
        };
        EngineeringSample: {
            nodes: components["schemas"]["EngineeringNode"][];
            values: {
                [key: string]: unknown;
            };
        };
        EngineeringSaveInput: {
            content: components["schemas"]["EngineeringContent"];
            reason: string;
            request_id: string;
            revision: number;
        };
        EngineeringStateInput: {
            enabled: boolean;
            reason: string;
            request_id: string;
            revision: number;
        };
        EngineeringSummary: {
            code: string;
            has_draft: boolean;
            id: string;
            /** @enum {string} */
            kind: "type-definitions" | "metrics" | "units" | "monitoring-templates";
            latest: number;
            name: string;
            source_version: string;
            usable: boolean;
        };
        EngineeringTarget: {
            component: string;
            type_version: string;
        };
        EngineeringTemplate: {
            groups: components["schemas"]["EngineeringGroup"][];
            items: components["schemas"]["EngineeringItem"][];
            purpose: string;
            type_versions: string[];
        };
        EngineeringUnit: {
            category: string;
            symbol: string;
        };
        EngineeringVersion: {
            code: string;
            content: components["schemas"]["EngineeringContent"];
            definition_id: string;
            dependencies: string[];
            digest: string;
            enabled: boolean;
            id: string;
            issues: components["schemas"]["EngineeringIssue"][];
            /** @enum {string} */
            kind: "type-definitions" | "metrics" | "units" | "monitoring-templates";
            number: number;
            /** Format: date-time */
            published_at: string;
            published_by: string;
            schema_version: string;
            schemas: {
                [key: string]: unknown;
            };
            state_reason: string;
            state_revision: number;
            unit_category?: components["schemas"]["EngineeringCategory"];
            usable: boolean;
        };
        EngineeringSource: {
            code: string;
            definition_id: string;
            name: string;
            number: number;
            version_id: string;
        };
        ResourceLinearReference: {
            route_code: string;
            route_name?: string;
            reference_name?: string;
            start_station?: string;
            end_station?: string;
            side?: string;
            direction?: string;
            offset_m?: number;
            note?: string;
        };
        ResourceGeographic: {
            crs: string;
            longitude: number;
            latitude: number;
        };
        ResourceProjected: {
            crs: string;
            x: number;
            y: number;
            axis_description: string;
            /** @enum {string} */
            unit: "m" | "mm";
        };
        ResourceElevation: {
            value_m: number;
            datum: string;
        };
        ResourceLocal: {
            reference_description: string;
            axes_description: string;
            /** @enum {string} */
            unit: "m" | "mm";
            x: number;
            y: number;
            z?: number;
        };
        ResourceSection: {
            /** @enum {string} */
            kind: "route_cross_section" | "facility_monitoring_section";
            code: string;
            name?: string;
            reference_description: string;
        };
        /** @description 位置整体替换，最多 32 KiB。缺省数组归一为空数组；未知坐标留空，0 是实际值。 */
        ResourcePosition: {
            description?: string;
            linear_references?: components["schemas"]["ResourceLinearReference"][];
            geographic?: components["schemas"]["ResourceGeographic"] | null;
            projected?: components["schemas"]["ResourceProjected"] | null;
            elevation?: components["schemas"]["ResourceElevation"] | null;
            local?: components["schemas"]["ResourceLocal"] | null;
            sections?: components["schemas"]["ResourceSection"][];
            source_reference?: string;
            /** Format: date */
            source_date?: string | null;
            accuracy_m?: number;
        };
        /** @description periodic 必须提供有限正周期；event/manual 不接受周期。仅为计划说明，不执行设备操作。 */
        ResourceSampling: {
            /** @enum {string} */
            mode: "periodic" | "event" | "manual";
            interval_seconds?: number;
            description?: string;
        };
        ResourceFacilityCreate: {
            name: string;
            description?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            /** @description 按固定定义校验的属性；整体替换，最多 128 KiB。 */
            values?: {
                [key: string]: unknown;
            };
            source_reference?: string;
            /** Format: uuid */
            type_version_id: string;
            /** Format: uuid */
            request_id: string;
            code: string;
        };
        ResourceFacilityUpdate: {
            name?: string;
            description?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            /** @description 按固定定义校验的属性；整体替换，最多 128 KiB。 */
            values?: {
                [key: string]: unknown;
            };
            source_reference?: string;
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            preview_hash?: string;
            reason?: string;
        };
        ResourceFacilityState: {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
        };
        ResourceComponentCreate: {
            name: string;
            description?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            /** @description 按固定定义校验的属性；整体替换，最多 128 KiB。 */
            values?: {
                [key: string]: unknown;
            };
            /** Format: uuid */
            parent_component_id?: string | null;
            category_key: string;
            /** Format: uuid */
            request_id: string;
            expected_structure_revision: number;
            code: string;
        };
        ResourceComponentUpdate: {
            name?: string;
            description?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            /** @description 按固定定义校验的属性；整体替换，最多 128 KiB。 */
            values?: {
                [key: string]: unknown;
            };
            /** Format: uuid */
            parent_component_id?: string | null;
            /** Format: uuid */
            request_id: string;
            expected_structure_revision?: number;
            expected_revision: number;
            preview_hash?: string;
            reason?: string;
        };
        ResourceComponentState: {
            /** Format: uuid */
            request_id: string;
            expected_structure_revision: number;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
        };
        ResourcePointCreate: {
            name: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            source_reference?: string;
            /** Format: uuid */
            component_id?: string | null;
            purpose?: string;
            direction_description?: string;
            reference_description?: string;
            /** Format: uuid */
            request_id: string;
            code: string;
        };
        ResourcePointUpdate: {
            name?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            source_reference?: string;
            /** Format: uuid */
            component_id?: string | null;
            purpose?: string;
            direction_description?: string;
            reference_description?: string;
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            preview_hash?: string;
            reason?: string;
        };
        ResourcePointState: {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
        };
        ResourceChannelCreate: {
            name: string;
            description?: string;
            sampling?: components["schemas"]["ResourceSampling"] | null;
            /** Format: uuid */
            metric_version_id: string;
            /** Format: uuid */
            unit_version_id?: string | null;
            observation_direction?: string;
            reference_basis?: string;
            /** @enum {string} */
            timestamp_meaning: "observation_time" | "interval_end";
            aggregation_description?: string;
            /** Format: uuid */
            request_id: string;
            code: string;
        };
        ResourceChannelUpdate: {
            name?: string;
            description?: string;
            sampling?: components["schemas"]["ResourceSampling"] | null;
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            preview_hash?: string;
            reason?: string;
        };
        ResourceChannelState: {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
        };
        Resource: {
            name: string;
            code: string;
            description?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            /** @description 按固定定义校验的属性；整体替换，最多 128 KiB。 */
            values?: {
                [key: string]: unknown;
            };
            source_reference?: string;
            /** Format: uuid */
            parent_component_id?: string | null;
            /** Format: uuid */
            component_id?: string | null;
            purpose?: string;
            direction_description?: string;
            reference_description?: string;
            sampling?: components["schemas"]["ResourceSampling"] | null;
            /** Format: uuid */
            type_version_id?: string;
            category_key?: string;
            /** Format: uuid */
            metric_version_id?: string;
            /** Format: uuid */
            unit_version_id?: string | null;
            observation_direction?: string;
            reference_basis?: string;
            /** @enum {string} */
            timestamp_meaning?: "observation_time" | "interval_end";
            aggregation_description?: string;
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            tenant_id: string;
            /** Format: uuid */
            facility_id?: string;
            /** Format: uuid */
            point_id?: string;
            /** @enum {string} */
            kind: "facility" | "component" | "point" | "channel";
            /** @enum {string} */
            status: "normal" | "retired";
            revision: number;
            structure_revision?: number;
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            updated_at: string;
            /** Format: uuid */
            created_by?: string;
            /** Format: uuid */
            updated_by?: string;
            /** Format: date-time */
            deleted_at: string | null;
            /** Format: uuid */
            created_project_id?: string;
            /** @constant */
            visibility?: "full";
            historical_context_hidden?: boolean;
        };
        ResourceActions: {
            edit: boolean;
            add: boolean;
            retire: boolean;
            restore: boolean;
            delete: boolean;
            reason: string;
            read: boolean;
            create_component: boolean;
            create_point: boolean;
            create_channel: boolean;
        };
        ResourceDetail: {
            name: string;
            code: string;
            description?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            /** @description 按固定定义校验的属性；整体替换，最多 128 KiB。 */
            values?: {
                [key: string]: unknown;
            };
            source_reference?: string;
            /** Format: uuid */
            parent_component_id?: string | null;
            /** Format: uuid */
            component_id?: string | null;
            purpose?: string;
            direction_description?: string;
            reference_description?: string;
            sampling?: components["schemas"]["ResourceSampling"] | null;
            /** Format: uuid */
            type_version_id?: string;
            category_key?: string;
            /** Format: uuid */
            metric_version_id?: string;
            /** Format: uuid */
            unit_version_id?: string | null;
            observation_direction?: string;
            reference_basis?: string;
            /** @enum {string} */
            timestamp_meaning?: "observation_time" | "interval_end";
            aggregation_description?: string;
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            tenant_id: string;
            /** Format: uuid */
            facility_id?: string;
            /** Format: uuid */
            point_id?: string;
            /** @enum {string} */
            kind: "facility" | "component" | "point" | "channel";
            /** @enum {string} */
            status: "normal" | "retired";
            revision: number;
            structure_revision?: number;
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            updated_at: string;
            /** Format: uuid */
            created_by?: string;
            /** Format: uuid */
            updated_by?: string;
            /** Format: date-time */
            deleted_at: string | null;
            ancestors: (components["schemas"]["Resource"] | components["schemas"]["ResourceContext"])[];
            definitions: components["schemas"]["EngineeringVersion"][];
            actions: components["schemas"]["ResourceActions"];
            /** Format: uuid */
            created_project_id?: string;
            /** @constant */
            visibility?: "full";
            historical_context_hidden?: boolean;
            access_revision?: string;
            link_revision?: string;
            /** Format: uuid */
            link_id?: string;
            shared: boolean;
        };
        ResourceItem: components["schemas"]["ResourceFullItem"] | components["schemas"]["ResourceContextItem"];
        ResourcePage: {
            items: components["schemas"]["ResourceItem"][];
            next_cursor: string;
            access_revision?: string;
            can_create_facility: boolean;
        };
        ResourceResult: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            facility_id: string;
            revision: number;
            structure_revision: number;
            changed: boolean;
            replayed: boolean;
            access_revision?: string;
            link_revision?: string;
            /** Format: uuid */
            project_id?: string;
            /** Format: uuid */
            link_id?: string;
        };
        ResourceChange: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            tenant_id: string;
            /** Format: uuid */
            facility_id: string;
            /** @enum {string} */
            resource_kind: "facility" | "component" | "point" | "channel";
            /** Format: uuid */
            resource_id: string;
            resource_revision: number;
            /** @enum {string} */
            operation: "create" | "update" | "retire" | "restore" | "delete";
            /** Format: uuid */
            actor_id?: string;
            actor_name: string;
            /** Format: uuid */
            request_id: string;
            /** Format: date-time */
            changed_at: string;
            reason: string;
            before: components["schemas"]["Resource"] | null;
            after: components["schemas"]["Resource"];
            changed_fields: string[];
            definition_versions: string[];
            /** @enum {string} */
            actor_scope?: "tenant" | "project" | "other";
            /** Format: uuid */
            actor_project_id?: string;
            /** Format: uuid */
            actor_tenant_membership_id?: string;
            /** Format: uuid */
            actor_project_membership_id?: string;
            /** Format: uuid */
            actor_link_id?: string;
        };
        ResourceChanges: {
            items: components["schemas"]["ResourceChange"][];
            next_cursor: string;
            /** @description Fixed published definitions and dependencies used by the authorized history items on this page, including disabled versions. */
            definitions: components["schemas"]["EngineeringVersion"][];
        };
        ResourceError: {
            /**
             * @description 包括 permission_denied、project_not_found、resource_not_found、project_archived、resource_access_changed、preview_required、preview_changed、resource_revision_conflict、structure_revision_conflict、shared_resource_tenant_action_required、tenant_action_required、resource_referenced、resource_code_taken、request_payload_conflict、resource_invalid、grant_capability_invalid、unavailable。
             * @example permission_denied
             * @example project_not_found
             * @example resource_not_found
             * @example project_archived
             * @example resource_access_changed
             * @example preview_required
             * @example preview_changed
             * @example resource_revision_conflict
             * @example structure_revision_conflict
             * @example shared_resource_tenant_action_required
             * @example tenant_action_required
             * @example resource_referenced
             * @example resource_code_taken
             * @example request_payload_conflict
             * @example resource_invalid
             * @example grant_capability_invalid
             * @example unavailable
             */
            code: string;
            message: string;
            issues?: components["schemas"]["EngineeringIssue"][];
            dependencies?: {
                [key: string]: number;
            };
        };
        InvitationCounts: {
            all: number;
            pending: number;
            used: number;
            expired: number;
            revoked: number;
        };
        ResourceContext: {
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            kind: "facility" | "component" | "point" | "channel";
            code: string;
            name: string;
            /** @enum {string} */
            status: "normal" | "retired";
            /** @constant */
            visibility: "context";
            category_key?: string;
            /** Format: uuid */
            parent_component_id?: string;
            /** Format: uuid */
            component_id?: string;
            /** Format: uuid */
            point_id?: string;
            /** Format: uuid */
            facility_id?: string;
        };
        ResourceFullItem: {
            name: string;
            code: string;
            description?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            /** @description 按固定定义校验的属性；整体替换，最多 128 KiB。 */
            values?: {
                [key: string]: unknown;
            };
            source_reference?: string;
            /** Format: uuid */
            parent_component_id?: string | null;
            /** Format: uuid */
            component_id?: string | null;
            purpose?: string;
            direction_description?: string;
            reference_description?: string;
            sampling?: components["schemas"]["ResourceSampling"] | null;
            /** Format: uuid */
            type_version_id?: string;
            category_key?: string;
            /** Format: uuid */
            metric_version_id?: string;
            /** Format: uuid */
            unit_version_id?: string | null;
            observation_direction?: string;
            reference_basis?: string;
            /** @enum {string} */
            timestamp_meaning?: "observation_time" | "interval_end";
            aggregation_description?: string;
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            tenant_id: string;
            /** Format: uuid */
            facility_id?: string;
            /** Format: uuid */
            point_id?: string;
            /** @enum {string} */
            kind: "facility" | "component" | "point" | "channel";
            /** @enum {string} */
            status: "normal" | "retired";
            revision: number;
            structure_revision?: number;
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            updated_at: string;
            /** Format: uuid */
            created_by?: string;
            /** Format: uuid */
            updated_by?: string;
            /** Format: date-time */
            deleted_at: string | null;
            has_children?: boolean;
            type_name?: string;
            type_number?: number;
            /** Format: uuid */
            type_definition_id?: string;
            /** Format: uuid */
            created_project_id?: string;
            /** @constant */
            visibility?: "full";
            historical_context_hidden?: boolean;
            has_visible_children: boolean;
        };
        ResourceContextItem: {
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            kind: "facility" | "component" | "point" | "channel";
            code: string;
            name: string;
            /** @enum {string} */
            status: "normal" | "retired";
            /** @constant */
            visibility: "context";
            category_key?: string;
            /** Format: uuid */
            parent_component_id?: string;
            /** Format: uuid */
            component_id?: string;
            /** Format: uuid */
            point_id?: string;
            /** Format: uuid */
            facility_id?: string;
            has_children?: boolean;
            has_visible_children: boolean;
        };
        ProjectResourceFacilityCreate: {
            name: string;
            description?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            /** @description 按固定定义校验的属性；整体替换，最多 128 KiB。 */
            values?: {
                [key: string]: unknown;
            };
            source_reference?: string;
            /** Format: uuid */
            type_version_id: string;
            /** Format: uuid */
            request_id: string;
            code: string;
            expected_access_revision: string;
        };
        ProjectResourceFacilityUpdate: {
            name?: string;
            description?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            /** @description 按固定定义校验的属性；整体替换，最多 128 KiB。 */
            values?: {
                [key: string]: unknown;
            };
            source_reference?: string;
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            preview_hash?: string;
            reason?: string;
            expected_access_revision: string;
            expected_link_revision: string;
        };
        ProjectResourceFacilityState: {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            expected_access_revision: string;
            expected_link_revision: string;
        };
        ProjectResourceComponentCreate: {
            name: string;
            description?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            /** @description 按固定定义校验的属性；整体替换，最多 128 KiB。 */
            values?: {
                [key: string]: unknown;
            };
            /** Format: uuid */
            parent_component_id?: string | null;
            category_key: string;
            /** Format: uuid */
            request_id: string;
            expected_structure_revision: number;
            code: string;
            expected_access_revision: string;
            expected_link_revision: string;
        };
        ProjectResourceComponentUpdate: {
            name?: string;
            description?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            /** @description 按固定定义校验的属性；整体替换，最多 128 KiB。 */
            values?: {
                [key: string]: unknown;
            };
            /** Format: uuid */
            parent_component_id?: string | null;
            /** Format: uuid */
            request_id: string;
            expected_structure_revision?: number;
            expected_revision: number;
            preview_hash?: string;
            reason?: string;
            expected_access_revision: string;
            expected_link_revision: string;
        };
        ProjectResourceComponentState: {
            /** Format: uuid */
            request_id: string;
            expected_structure_revision: number;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            expected_access_revision: string;
            expected_link_revision: string;
        };
        ProjectResourcePointCreate: {
            name: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            source_reference?: string;
            /** Format: uuid */
            component_id?: string | null;
            purpose?: string;
            direction_description?: string;
            reference_description?: string;
            /** Format: uuid */
            request_id: string;
            code: string;
            expected_access_revision: string;
            expected_link_revision: string;
        };
        ProjectResourcePointUpdate: {
            name?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            source_reference?: string;
            /** Format: uuid */
            component_id?: string | null;
            purpose?: string;
            direction_description?: string;
            reference_description?: string;
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            preview_hash?: string;
            reason?: string;
            expected_access_revision: string;
            expected_link_revision: string;
        };
        ProjectResourcePointState: {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            expected_access_revision: string;
            expected_link_revision: string;
        };
        ProjectResourceChannelCreate: {
            name: string;
            description?: string;
            sampling?: components["schemas"]["ResourceSampling"] | null;
            /** Format: uuid */
            metric_version_id: string;
            /** Format: uuid */
            unit_version_id?: string | null;
            observation_direction?: string;
            reference_basis?: string;
            /** @enum {string} */
            timestamp_meaning: "observation_time" | "interval_end";
            aggregation_description?: string;
            /** Format: uuid */
            request_id: string;
            code: string;
            expected_access_revision: string;
            expected_link_revision: string;
        };
        ProjectResourceChannelUpdate: {
            name?: string;
            description?: string;
            sampling?: components["schemas"]["ResourceSampling"] | null;
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            preview_hash?: string;
            reason?: string;
            expected_access_revision: string;
            expected_link_revision: string;
        };
        ProjectResourceChannelState: {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            expected_access_revision: string;
            expected_link_revision: string;
        };
        ResourceMutationPreviewInput: {
            name?: string;
            description?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            /** @description 按固定定义校验的属性；整体替换，最多 128 KiB。 */
            values?: {
                [key: string]: unknown;
            };
            source_reference?: string;
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            preview_hash?: string;
            reason?: string;
            /** @constant */
            action: "update";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            /** @constant */
            action: "retire";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            /** @constant */
            action: "restore";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            /** @constant */
            action: "delete";
        } | {
            name?: string;
            description?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            /** @description 按固定定义校验的属性；整体替换，最多 128 KiB。 */
            values?: {
                [key: string]: unknown;
            };
            /** Format: uuid */
            parent_component_id?: string | null;
            /** Format: uuid */
            request_id: string;
            expected_structure_revision?: number;
            expected_revision: number;
            preview_hash?: string;
            reason?: string;
            /** @constant */
            action: "update";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_structure_revision: number;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            /** @constant */
            action: "retire";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_structure_revision: number;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            /** @constant */
            action: "restore";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_structure_revision: number;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            /** @constant */
            action: "delete";
        } | {
            name?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            source_reference?: string;
            /** Format: uuid */
            component_id?: string | null;
            purpose?: string;
            direction_description?: string;
            reference_description?: string;
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            preview_hash?: string;
            reason?: string;
            /** @constant */
            action: "update";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            /** @constant */
            action: "retire";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            /** @constant */
            action: "restore";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            /** @constant */
            action: "delete";
        } | {
            name?: string;
            description?: string;
            sampling?: components["schemas"]["ResourceSampling"] | null;
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            preview_hash?: string;
            reason?: string;
            /** @constant */
            action: "update";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            /** @constant */
            action: "retire";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            /** @constant */
            action: "restore";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            /** @constant */
            action: "delete";
        };
        ProjectResourceMutationPreviewInput: {
            name?: string;
            description?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            /** @description 按固定定义校验的属性；整体替换，最多 128 KiB。 */
            values?: {
                [key: string]: unknown;
            };
            source_reference?: string;
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            preview_hash?: string;
            reason?: string;
            expected_access_revision: string;
            expected_link_revision: string;
            /** @constant */
            action: "update";
        } | {
            name?: string;
            description?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            /** @description 按固定定义校验的属性；整体替换，最多 128 KiB。 */
            values?: {
                [key: string]: unknown;
            };
            /** Format: uuid */
            parent_component_id?: string | null;
            /** Format: uuid */
            request_id: string;
            expected_structure_revision?: number;
            expected_revision: number;
            preview_hash?: string;
            reason?: string;
            expected_access_revision: string;
            expected_link_revision: string;
            /** @constant */
            action: "update";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_structure_revision: number;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            expected_access_revision: string;
            expected_link_revision: string;
            /** @constant */
            action: "retire";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_structure_revision: number;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            expected_access_revision: string;
            expected_link_revision: string;
            /** @constant */
            action: "restore";
        } | {
            name?: string;
            position?: components["schemas"]["ResourcePosition"] | null;
            source_reference?: string;
            /** Format: uuid */
            component_id?: string | null;
            purpose?: string;
            direction_description?: string;
            reference_description?: string;
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            preview_hash?: string;
            reason?: string;
            expected_access_revision: string;
            expected_link_revision: string;
            /** @constant */
            action: "update";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            expected_access_revision: string;
            expected_link_revision: string;
            /** @constant */
            action: "retire";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            expected_access_revision: string;
            expected_link_revision: string;
            /** @constant */
            action: "restore";
        } | {
            name?: string;
            description?: string;
            sampling?: components["schemas"]["ResourceSampling"] | null;
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            preview_hash?: string;
            reason?: string;
            expected_access_revision: string;
            expected_link_revision: string;
            /** @constant */
            action: "update";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            expected_access_revision: string;
            expected_link_revision: string;
            /** @constant */
            action: "retire";
        } | {
            /** Format: uuid */
            request_id: string;
            expected_revision: number;
            reason: string;
            preview_hash?: string;
            expected_access_revision: string;
            expected_link_revision: string;
            /** @constant */
            action: "restore";
        };
        ResourceMutationPreview: {
            preview_hash: string;
            required: boolean;
            can_submit: boolean;
            shared: boolean;
            blocked_code?: string;
            blocked_reason?: string;
            changes: components["schemas"]["EngineeringDifference"][];
            projects?: {
                /** Format: uuid */
                id: string;
                name: string;
                status: string;
            }[];
            dependencies?: {
                [key: string]: number;
            };
        };
        ProjectResourcePolicy: {
            /** Format: uuid */
            tenant_id: string;
            /** Format: uuid */
            project_id: string;
            project_name: string;
            /** @enum {string} */
            project_status: "active" | "archived";
            project_version: string;
            allow_facility_create: boolean;
            revision: string;
        };
        ProjectFacilityLink: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            tenant_id: string;
            /** Format: uuid */
            project_id: string;
            /** Format: uuid */
            facility_id: string;
            facility_name: string;
            facility_code: string;
            /** @enum {string} */
            status: "active" | "revoked";
            revision: string;
            /** Format: date-time */
            created_at: string;
        };
        ProjectResourceGrant: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            link_id: string;
            /** @enum {string} */
            kind: "facility" | "component" | "point" | "channel";
            /** Format: uuid */
            resource_id: string;
            /** Format: uuid */
            component_id?: string;
            /** Format: uuid */
            point_id?: string;
            /** Format: uuid */
            channel_id?: string;
            name: string;
            code: string;
            capabilities: ("read" | "update" | "create_component" | "create_point" | "create_channel" | "lifecycle")[];
            /** @enum {string} */
            source: "tenant_grant" | "project_create";
            revision: string;
        };
        ProjectResourceAccessDetail: {
            policy: components["schemas"]["ProjectResourcePolicy"];
            links: components["schemas"]["ProjectFacilityLink"][];
            next_cursor: string;
            link: components["schemas"]["ProjectFacilityLink"] | null;
            grants: components["schemas"]["ProjectResourceGrant"][];
            grant_next_cursor: string;
            candidates: components["schemas"]["ResourcePage"];
            candidate_grants: components["schemas"]["ProjectResourceGrant"][];
        };
        ProjectResourceGrantInput: {
            /** @enum {string} */
            kind: "facility" | "component" | "point" | "channel";
            /** Format: uuid */
            component_id?: string;
            /** Format: uuid */
            point_id?: string;
            /** Format: uuid */
            channel_id?: string;
            capabilities: ("read" | "update" | "create_component" | "create_point" | "create_channel" | "lifecycle")[];
        };
        ProjectResourceGrantUpdate: {
            /** Format: uuid */
            grant_id: string;
            expected_revision: string;
            capabilities: ("read" | "update" | "create_component" | "create_point" | "create_channel" | "lifecycle")[];
        };
        ProjectResourceGrantEnd: {
            /** Format: uuid */
            grant_id: string;
            expected_revision: string;
        };
        ProjectResourceCurrentSelection: {
            /** @enum {string} */
            root_kind: "facility" | "component" | "point" | "channel";
            /** Format: uuid */
            root_id: string;
            kinds: {
                facility?: ("read" | "update" | "create_component" | "create_point" | "create_channel" | "lifecycle")[];
                component?: ("read" | "update" | "create_component" | "create_point" | "create_channel" | "lifecycle")[];
                point?: ("read" | "update" | "create_component" | "create_point" | "create_channel" | "lifecycle")[];
                channel?: ("read" | "update" | "create_component" | "create_point" | "create_channel" | "lifecycle")[];
            };
        };
        ProjectResourceAccessInput: {
            /** Format: uuid */
            request_id: string;
            /** @enum {string} */
            action: "policy" | "create_link" | "change_grants" | "revoke_link";
            expected_access_revision: string;
            expected_link_revision?: string;
            reason: string;
            preview_hash?: string;
            allow_facility_create?: boolean;
            add?: components["schemas"]["ProjectResourceGrantInput"][];
            update?: components["schemas"]["ProjectResourceGrantUpdate"][];
            end?: components["schemas"]["ProjectResourceGrantEnd"][];
            select_current?: components["schemas"]["ProjectResourceCurrentSelection"];
        };
        ProjectResourceAccessPreview: {
            preview_hash: string;
            policy: components["schemas"]["ProjectResourcePolicy"];
            link: components["schemas"]["ProjectFacilityLink"] | null;
            add: components["schemas"]["ProjectResourceGrant"][];
            update: components["schemas"]["ProjectResourceGrant"][];
            end: components["schemas"]["ProjectResourceGrant"][];
            retained_context: components["schemas"]["ResourceContext"][];
            changed: boolean;
        };
        ProjectResourceAccessResult: {
            /** Format: uuid */
            project_id: string;
            /** Format: uuid */
            link_id?: string;
            link_revision?: string;
            access_revision: string;
            changed: boolean;
            replayed: boolean;
        };
        ProjectResourceAccessChange: {
            /** Format: uuid */
            id: string;
            access_revision: string;
            operation: string;
            /** Format: uuid */
            actor_id: string;
            /** @enum {string} */
            actor_scope: "tenant" | "project";
            reason: string;
            before: unknown;
            after: unknown;
            /** Format: date-time */
            changed_at: string;
        };
        ProjectResourceAccessChanges: {
            items: components["schemas"]["ProjectResourceAccessChange"][];
            next_cursor: string;
        };
        ProjectResourceCatalogItem: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            definition_id: string;
            code: string;
            name: string;
            number: number;
        };
        ProjectResourceCatalogPage: {
            items: components["schemas"]["ProjectResourceCatalogItem"][];
            total: number;
            limit: number;
            offset: number;
        };
        ProjectResourceCatalogVersion: {
            version: components["schemas"]["EngineeringVersion"];
            dependencies: components["schemas"]["EngineeringVersion"][];
        };
        ResourceUsagePage: {
            next_cursor: string;
            items: {
                /** Format: uuid */
                project_id: string;
                /** Format: uuid */
                link_id: string;
                name: string;
                code: string;
                /** @enum {string} */
                status: "active" | "archived";
            }[];
        };
    };
    responses: never;
    parameters: never;
    requestBodies: never;
    headers: never;
    pathItems: never;
}
export type $defs = Record<string, never>;
export interface operations {
    getLiveness: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProbeStatus"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    getReadiness: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProbeStatus"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    getAccountHelp: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AccountHelp"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    getCSRF: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CSRF"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    login: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["LoginInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Account"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    currentAuthorization: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AuthContext"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    reauthenticate: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["PasswordInput"];
            };
        };
        responses: {
            /** @description Success */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    logout: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    logoutAll: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    recordActivity: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    readProfile: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Profile"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    updateProfile: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProfileInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Account"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    changePassword: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ChangePasswordInput"];
            };
        };
        responses: {
            /** @description Success */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    getSessions: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Sessions"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    revokeSession: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    getSecurityEvents: {
        parameters: {
            query?: {
                cursor?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["SecurityEvents"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    getInvitations: {
        parameters: {
            query?: {
                cursor?: string;
                /** @description Literal case-insensitive substring of the invitation note, trimmed; searches all invitations owned by the current account. */
                q?: string;
                status?: "" | "pending" | "used" | "expired" | "revoked";
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Invitations"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    createInvitation: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["InvitationInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Link"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    revokeInvitation: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    regenerateInvitation: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Link"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    inspectInvitation: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TokenInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Inspection"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    acceptInvitation: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AcceptInvitationInput"];
            };
        };
        responses: {
            /** @description Success */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    inspectReset: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TokenInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Inspection"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    completeReset: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResetPasswordInput"];
            };
        };
        responses: {
            /** @description Success */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    permissionCatalog: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Permissions"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    listRoles: {
        parameters: {
            query?: {
                cursor?: string;
                q?: string;
                status?: string;
                role_id?: string;
                object_id?: string;
                object_type?: string;
                from?: string;
                to?: string;
                limit?: number;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Roles"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    createRole: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateRoleInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Role"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    getRole: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Role"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    updateRole: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["UpdateRoleInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Role"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    setRolePermissions: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["RolePermissionsInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Role"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    setRoleStatus: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["RoleStatusInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Role"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    archiveRole: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AccessMutation"];
            };
        };
        responses: {
            /** @description Success */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    listAccounts: {
        parameters: {
            query?: {
                cursor?: string;
                q?: string;
                status?: string;
                role_id?: string;
                object_id?: string;
                object_type?: string;
                from?: string;
                to?: string;
                limit?: number;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ManagedAccounts"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    getAccount: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ManagedAccount"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    accountAuthorization: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Authorization"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    assignRoles: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AccountRolesInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ManagedAccount"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    disableAccount: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AccessMutation"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ManagedAccount"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    enableAccount: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AccessMutation"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ManagedAccount"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    accessAudit: {
        parameters: {
            query?: {
                /** @description Opaque chronological cursor returned by next_cursor; numeric legacy cursors are also accepted. */
                cursor?: string;
                /** @description Search visible actor/target/tenant names, usernames, event code and the event reason. */
                q?: string;
                status?: string;
                role_id?: string;
                object_id?: string;
                object_type?: string;
                from?: string;
                to?: string;
                limit?: number;
                tenant_id?: string;
                event_type?: string;
                category?: "management" | "security" | "system";
                outcome?: "success" | "failure" | "rate_limited";
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AccessAuditEvents"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    listTenants: {
        parameters: {
            query?: {
                q?: string;
                cursor?: string;
                limit?: number;
                administrator_unavailable?: boolean;
                status?: "active" | "disabled";
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Tenants"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    createTenant: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateTenantInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantMutationResult"];
                };
            };
            /** @description Created; 200 means replay */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantMutationResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    tenantCandidates: {
        parameters: {
            query?: {
                q?: string;
                cursor?: string;
                limit?: number;
                /** @description Transfer preview; requires transfer_admin permission. */
                tenant_id?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantCandidates"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    myTenants: {
        parameters: {
            query?: {
                q?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MyTenants"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    accountTenants: {
        parameters: {
            query?: {
                q?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MyTenants"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    tenantContext: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantContext"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    platformTenant: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Tenant"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    updateplatformTenant: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["UpdateTenantInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantMutationResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    tenantProfile: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantProfile"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    updatetenantProfile: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["UpdateTenantInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantMutationResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    transferTenant: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TransferTenantInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantMutationResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    tenantAudit: {
        parameters: {
            query?: {
                q?: string;
                cursor?: string;
                limit?: number;
                event_type?: string;
                from?: string;
                to?: string;
                object_type?: string;
                object_id?: string;
            };
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantAuditEvents"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    tenantMembers: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                role?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantMembers"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    tenantMember: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                membership_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantMember"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    suspendTenantMember: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                membership_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TenantMemberMutationInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantMembershipResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    resumeTenantMember: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                membership_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TenantMemberMutationInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantMembershipResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    removeTenantMember: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                membership_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TenantMemberMutationInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantMembershipResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    resolveTenantMember: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResolveTenantMemberInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantAccount"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    tenantInvitations: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                from?: string;
                to?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantInvitations"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    createTenantInvitation: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateTenantInvitationInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantInvitationCreated"];
                };
            };
            /** @description Success */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantInvitationCreated"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    tenantInvitation: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                invitation_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantInvitation"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    revokeTenantInvitation: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                invitation_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TenantMemberMutationInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantInvitation"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    regenerateTenantInvitation: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                invitation_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["RegenerateTenantInvitationInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantInvitationCreated"];
                };
            };
            /** @description Success */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantInvitationCreated"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    inspectTenantInvitation: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TokenInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantInvitationInspection"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    acceptTenantInvitation: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AcceptTenantInvitationInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantAcceptanceResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    tenantPermissions: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantPermission"][];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    tenantRoles: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                source?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantRoleDefinitions"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    createTenantRole: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TenantRoleCreateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantRoleResult"];
                };
            };
            /** @description Success */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantRoleResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    tenantRole: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantRoleDefinition"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    updateTenantRole: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TenantRoleUpdateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantAuthorizationResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    tenantRoleMembers: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantRoleMembers"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    copyTenantRole: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TenantRoleCreateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantRoleResult"];
                };
            };
            /** @description Success */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantRoleResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    previewTenantRolePermissions: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TenantRolePermissionsPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantAuthorizationImpact"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    changeTenantRolePermissions: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TenantRolePermissionsInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantAuthorizationResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    previewTenantRoleStatus: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TenantRoleStatusPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantAuthorizationImpact"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    changeTenantRoleStatus: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TenantRoleStatusInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantAuthorizationResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    previewTenantRoleArchive: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TenantRoleArchivePreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantAuthorizationImpact"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    changeTenantRoleArchive: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TenantRoleArchiveInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantAuthorizationResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    tenantMemberAuthorization: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                membership_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantAuthorization"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    previewTenantMemberRoles: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                membership_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TenantMemberRolesPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantAuthorizationImpact"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    assignTenantMemberRoles: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                membership_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TenantMemberRolesInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantAuthorizationResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    tenantRolePresets: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantRoleDefinitions"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    tenantRolePreset: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                role_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantRoleDefinition"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    disableTenantPreview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": Record<string, never>;
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantStatusPreview"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    disableTenant: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TenantDisableInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantStatusResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    enableTenantPreview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": Record<string, never>;
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantStatusPreview"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    enableTenant: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TenantEnableInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantStatusResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    tenantAvailability: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantAvailability"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    MyProjects: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Projects"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectGovernanceList: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                cursor?: string;
                limit?: number;
                owner_tenant_membership_id?: string;
                administrator_unavailable?: string;
            };
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Projects"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectGovernanceDetail: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectSummary"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectCreate: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectCreateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectCreateCandidates: {
        parameters: {
            query?: {
                q?: string;
            };
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectCandidates"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectRecoveryCandidates: {
        parameters: {
            query?: {
                q?: string;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectCandidates"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectRecoverAdminPreview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectCandidatePreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectPreview"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectRecoverAdmin: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectCandidateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectContext: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectContext"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectAvailability: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectContext"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectProfile: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectProfile"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectUpdate: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectUpdateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectMembers: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectMembers"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectMemberAdd: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectAddMemberInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectMember: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                membership_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectMember"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectAuthorization: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                membership_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectAuthorization"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectRoles: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectRoles"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectRoleCreate: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectRoleInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectRole: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Role"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectRoleUpdate: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectRoleUpdateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectRoleMembers: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectMembers"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectPermissions: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Permission"][];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectAudit: {
        parameters: {
            query?: {
                q?: string;
                cursor?: string;
                limit?: number;
                event_type?: string;
                from?: string;
                to?: string;
                object_id?: string;
                actor_id?: string;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectAuditEvents"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectMemberCandidates: {
        parameters: {
            query?: {
                q?: string;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectCandidates"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectOwnerCandidates: {
        parameters: {
            query?: {
                q?: string;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectCandidates"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectAdministratorCandidates: {
        parameters: {
            query?: {
                q?: string;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectCandidates"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectMemberSuspendPreview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                membership_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectEmptyPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectPreview"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectMemberSuspend: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                membership_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectChangeInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectMemberResumePreview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                membership_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectEmptyPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectPreview"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectMemberResume: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                membership_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectChangeInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectMemberRemovePreview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                membership_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectEmptyPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectPreview"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectMemberRemove: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                membership_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectChangeInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectOwnerChangePreview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectCandidatePreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectPreview"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectOwnerChange: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectCandidateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectTransferPreview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectCandidatePreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectPreview"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectTransfer: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectCandidateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectArchivePreview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectEmptyPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectPreview"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectArchive: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectChangeInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectRestorePreview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectEmptyPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectPreview"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectRestore: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectChangeInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectRolePermissionsPreview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectPermissionsPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectPreview"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectRolePermissions: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectPermissionsInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectRoleStatusPreview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectStatusPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectPreview"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectRoleStatus: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectStatusInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectRoleDeletePreview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectEmptyPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectPreview"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectRoleDelete: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectChangeInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectMemberRolesPreview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                membership_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectRolesPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectPreview"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectMemberRoles: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                membership_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectRolesInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectRoleCopy: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                role_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectRoleInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResult"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectPresetList: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectRoles"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    ProjectPresetDetail: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                role_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Role"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Version, preview, lifecycle, grant or idempotency conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantTransferPreview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": Record<string, never>;
            };
        };
        responses: {
            /** @description Governance impact */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantProjectTransferPreview"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Transaction failed; no partial business writes */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_type_definitions_list: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                offset?: number;
                limit?: number;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPage"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_type_definitions_create: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringCreateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_type_definitions_detail: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDefinition"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_type_definitions_preview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPreview"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_type_definitions_diff: {
        parameters: {
            query?: {
                from?: string;
                to?: string;
                revision?: number;
            };
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDifference"][];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_type_definitions_impact: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
                version_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringImpact"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_type_definitions_save: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringSaveInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_type_definitions_new_draft: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringNewDraftInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_type_definitions_delete_draft: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringRevisionInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_type_definitions_copy: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringCopyInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_type_definitions_publish: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringRevisionInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_type_definitions_state: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
                version_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringStateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineeringCatalog_type_definitions_list: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                offset?: number;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPage"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineeringCatalog_type_definitions_detail: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDefinition"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineeringCatalog_type_definitions_preview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPreview"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineeringCatalog_type_definitions_diff: {
        parameters: {
            query?: {
                from?: string;
                to?: string;
                revision?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDifference"][];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_metrics_list: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                offset?: number;
                limit?: number;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPage"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_metrics_create: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringCreateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_metrics_detail: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDefinition"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_metrics_preview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPreview"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_metrics_diff: {
        parameters: {
            query?: {
                from?: string;
                to?: string;
                revision?: number;
            };
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDifference"][];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_metrics_impact: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
                version_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringImpact"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_metrics_save: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringSaveInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_metrics_new_draft: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringNewDraftInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_metrics_delete_draft: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringRevisionInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_metrics_copy: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringCopyInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_metrics_publish: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringRevisionInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_metrics_state: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
                version_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringStateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineeringCatalog_metrics_list: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                offset?: number;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPage"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineeringCatalog_metrics_detail: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDefinition"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineeringCatalog_metrics_preview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPreview"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineeringCatalog_metrics_diff: {
        parameters: {
            query?: {
                from?: string;
                to?: string;
                revision?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDifference"][];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_units_list: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                offset?: number;
                limit?: number;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPage"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_units_create: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringCreateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_units_detail: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDefinition"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_units_preview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPreview"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_units_diff: {
        parameters: {
            query?: {
                from?: string;
                to?: string;
                revision?: number;
            };
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDifference"][];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_units_impact: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
                version_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringImpact"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_units_save: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringSaveInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_units_new_draft: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringNewDraftInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_units_delete_draft: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringRevisionInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_units_copy: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringCopyInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_units_publish: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringRevisionInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_units_state: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
                version_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringStateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineeringCatalog_units_list: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                offset?: number;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPage"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineeringCatalog_units_detail: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDefinition"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineeringCatalog_units_preview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPreview"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineeringCatalog_units_diff: {
        parameters: {
            query?: {
                from?: string;
                to?: string;
                revision?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDifference"][];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_monitoring_templates_list: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                offset?: number;
                limit?: number;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPage"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_monitoring_templates_create: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringCreateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_monitoring_templates_detail: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDefinition"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_monitoring_templates_preview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPreview"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_monitoring_templates_diff: {
        parameters: {
            query?: {
                from?: string;
                to?: string;
                revision?: number;
            };
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDifference"][];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_monitoring_templates_impact: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
                version_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringImpact"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_monitoring_templates_save: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringSaveInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_monitoring_templates_new_draft: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringNewDraftInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_monitoring_templates_delete_draft: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringRevisionInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_monitoring_templates_copy: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringCopyInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_monitoring_templates_publish: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringRevisionInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_monitoring_templates_state: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                definition_id: string;
                version_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringStateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineering_monitoring_templates_list: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                offset?: number;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPage"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineering_monitoring_templates_create: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringCreateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineering_monitoring_templates_detail: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDefinition"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineering_monitoring_templates_preview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPreview"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineering_monitoring_templates_diff: {
        parameters: {
            query?: {
                from?: string;
                to?: string;
                revision?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDifference"][];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineering_monitoring_templates_impact: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
                version_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringImpact"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineering_monitoring_templates_save: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringSaveInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineering_monitoring_templates_new_draft: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringNewDraftInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineering_monitoring_templates_delete_draft: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringRevisionInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineering_monitoring_templates_copy: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringCopyInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineering_monitoring_templates_publish: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringRevisionInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineering_monitoring_templates_state: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
                version_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringStateInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineeringCatalog_monitoring_templates_list: {
        parameters: {
            query?: {
                q?: string;
                status?: string;
                offset?: number;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPage"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineeringCatalog_monitoring_templates_detail: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDefinition"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineeringCatalog_monitoring_templates_preview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringPreviewInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringPreview"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineeringCatalog_monitoring_templates_diff: {
        parameters: {
            query?: {
                from?: string;
                to?: string;
                revision?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                definition_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringDifference"][];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    EngineeringCategories_list: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringCategory"][];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    EngineeringCategories_save: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringCategoryInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    EngineeringCategories_delete: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                category_key: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EngineeringRevisionInput"];
            };
        };
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    EngineeringCategories_catalog: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringCategory"][];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_type_definitions_receipt: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                request_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_metrics_receipt: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                request_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_units_receipt: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                request_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    Engineering_monitoring_templates_receipt: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                request_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    TenantEngineering_monitoring_templates_receipt: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                request_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EngineeringResult"];
                };
            };
            /** @description Scoped structured error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    "Resource:facility:list": {
        parameters: {
            query?: {
                q?: string;
                status?: "normal" | "retired" | "all";
                limit?: number;
                cursor?: string;
                type_definition_id?: string;
                deleted?: "only";
            };
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourcePage"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:facility:create": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceFacilityCreate"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:facility:detail": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceDetail"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:facility:update": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceFacilityUpdate"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:facility:retire": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceFacilityState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:facility:restore": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceFacilityState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:facility:delete": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceFacilityState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:component:list": {
        parameters: {
            query?: {
                q?: string;
                status?: "normal" | "retired" | "all";
                limit?: number;
                cursor?: string;
                parent_id?: string;
            };
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourcePage"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:component:create": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceComponentCreate"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:component:detail": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                component_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceDetail"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:component:update": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                component_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceComponentUpdate"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:component:retire": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                component_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceComponentState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:component:restore": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                component_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceComponentState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:component:delete": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                component_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceComponentState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:point:list": {
        parameters: {
            query?: {
                q?: string;
                status?: "normal" | "retired" | "all";
                limit?: number;
                cursor?: string;
                owner?: string;
            };
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourcePage"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:point:create": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourcePointCreate"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:point:detail": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                point_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceDetail"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:point:update": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                point_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourcePointUpdate"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:point:retire": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                point_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourcePointState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:point:restore": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                point_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourcePointState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:point:delete": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                point_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourcePointState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:channel:list": {
        parameters: {
            query?: {
                q?: string;
                status?: "normal" | "retired" | "all";
                limit?: number;
                cursor?: string;
            };
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                point_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourcePage"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:channel:create": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                point_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceChannelCreate"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:channel:detail": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                point_id: string;
                channel_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceDetail"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:channel:update": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                point_id: string;
                channel_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceChannelUpdate"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:channel:retire": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                point_id: string;
                channel_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceChannelState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:channel:restore": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                point_id: string;
                channel_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceChannelState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:channel:delete": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                point_id: string;
                channel_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceChannelState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:facility:changes": {
        parameters: {
            query?: {
                kind?: "facility" | "component" | "point" | "channel";
                id?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceChanges"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:request:detail": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                request_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    getInvitation: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Success */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Invitation"];
                };
            };
            /** @description Invalid input */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Not authenticated */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Origin, CSRF or recent password confirmation required */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Resource or credential unavailable */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Conflict */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Rate limited */
            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
            /** @description Dependency unavailable */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    "ProjectResource:facility:list": {
        parameters: {
            query?: {
                q?: string;
                status?: "normal" | "retired" | "all";
                limit?: number;
                cursor?: string;
                type_definition_id?: string;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourcePage"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:facility:create": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceFacilityCreate"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:facility:detail": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceDetail"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:facility:update": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceFacilityUpdate"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:facility:preview": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段；变更前重新校验资格与预览。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceMutationPreviewInput"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceMutationPreview"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:facility:preview": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段；变更前重新校验资格与预览。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceMutationPreviewInput"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceMutationPreview"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:component:list": {
        parameters: {
            query?: {
                q?: string;
                status?: "normal" | "retired" | "all";
                limit?: number;
                cursor?: string;
                parent_id?: string;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourcePage"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:component:create": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceComponentCreate"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:component:detail": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
                component_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceDetail"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:component:update": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
                component_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceComponentUpdate"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:component:preview": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                component_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段；变更前重新校验资格与预览。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceMutationPreviewInput"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceMutationPreview"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:component:preview": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
                component_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段；变更前重新校验资格与预览。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceMutationPreviewInput"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceMutationPreview"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:component:retire": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
                component_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceComponentState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:component:restore": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
                component_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceComponentState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:point:list": {
        parameters: {
            query?: {
                q?: string;
                status?: "normal" | "retired" | "all";
                limit?: number;
                cursor?: string;
                owner?: string;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourcePage"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:point:create": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourcePointCreate"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:point:detail": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
                point_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceDetail"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:point:update": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
                point_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourcePointUpdate"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:point:preview": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                point_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段；变更前重新校验资格与预览。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceMutationPreviewInput"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceMutationPreview"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:point:preview": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
                point_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段；变更前重新校验资格与预览。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceMutationPreviewInput"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceMutationPreview"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:point:retire": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
                point_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourcePointState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:point:restore": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
                point_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourcePointState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:channel:list": {
        parameters: {
            query?: {
                q?: string;
                status?: "normal" | "retired" | "all";
                limit?: number;
                cursor?: string;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
                point_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourcePage"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:channel:create": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
                point_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceChannelCreate"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:channel:detail": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
                point_id: string;
                channel_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceDetail"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:channel:update": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
                point_id: string;
                channel_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceChannelUpdate"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "Resource:channel:preview": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
                point_id: string;
                channel_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段；变更前重新校验资格与预览。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourceMutationPreviewInput"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceMutationPreview"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:channel:preview": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
                point_id: string;
                channel_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段；变更前重新校验资格与预览。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceMutationPreviewInput"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceMutationPreview"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:channel:retire": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
                point_id: string;
                channel_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceChannelState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:channel:restore": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
                point_id: string;
                channel_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段。PATCH 未提交字段不变；嵌套对象整体替换。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceChannelState"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:facility:changes": {
        parameters: {
            query?: {
                kind?: "facility" | "component" | "point" | "channel";
                id?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceChanges"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:request:detail": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                request_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResourceAccess:access:detail": {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResourceAccessDetail"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResourceAccess:access:changes": {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResourceAccessChanges"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResourceAccess:policy:preview": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段；变更前重新校验资格与预览。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceAccessInput"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResourceAccessPreview"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResourceAccess:policy:change": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段；变更前重新校验资格与预览。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceAccessInput"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResourceAccessResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResourceAccess:facility:detail": {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
                grant_cursor?: string;
                candidate_kind?: string;
                candidate_cursor?: string;
                candidate_limit?: number;
                candidate_q?: string;
                candidate_status?: string;
                candidate_parent_id?: string;
                candidate_owner?: string;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResourceAccessDetail"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResourceAccess:facility:preview": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段；变更前重新校验资格与预览。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceAccessInput"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResourceAccessPreview"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResourceAccess:facility:change": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        /** @description 最多 256 KiB；拒绝未知字段；变更前重新校验资格与预览。 */
        requestBody: {
            content: {
                "application/json": components["schemas"]["ProjectResourceAccessInput"];
            };
        };
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResourceAccessResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResourceAccess:request:detail": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                request_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResourceAccessResult"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:type-definitions:list": {
        parameters: {
            query?: {
                q?: string;
                offset?: number;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResourceCatalogPage"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:type-definitions:version": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                version_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResourceCatalogVersion"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:metrics:list": {
        parameters: {
            query?: {
                q?: string;
                offset?: number;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResourceCatalogPage"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:metrics:version": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                version_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResourceCatalogVersion"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:units:list": {
        parameters: {
            query?: {
                q?: string;
                offset?: number;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResourceCatalogPage"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResource:units:version": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tenant_id: string;
                project_id: string;
                version_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectResourceCatalogVersion"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
    "ProjectResourceAccess:usage:detail": {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                tenant_id: string;
                facility_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description 成功；重放返回原提交最小结果。 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceUsagePage"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            413: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
            /** @description 请求未完成；冲突或临时错误保留输入，写请求不得自动重试。 */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceError"];
                };
            };
        };
    };
}
