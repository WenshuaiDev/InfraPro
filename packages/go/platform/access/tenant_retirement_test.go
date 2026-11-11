package access

import (
	"slices"
	"testing"
)

// A release must evaluate each side against its own catalog lifecycle, including copies.
func TestTenantPermissionRetirementKeepsConfigurationButRemovesEverySource(t *testing.T) {
	old := map[Code]Permission{TenantInvitationCreate: {Code: TenantInvitationCreate, Scope: "tenant", Lifecycle: "active", TenantCustomRoleAssignable: true}}
	next := map[Code]Permission{TenantInvitationCreate: {Code: TenantInvitationCreate, Scope: "tenant", Lifecycle: "retired", TenantCustomRoleAssignable: true}}
	v := TenantAuthorization{AccountStatus: "active", MembershipStatus: "active", Snapshot: Snapshot{Roles: []Role{{ID: "preset", Status: "active", Permissions: []Code{TenantInvitationCreate}}, {ID: "independent-copy", Status: "active", Permissions: []Code{TenantInvitationCreate}}}}}
	explainTenant(&v, old)
	if len(v.Sources[TenantInvitationCreate]) != 2 {
		t.Fatal(v)
	}
	before := slices.Clone(v.Permissions)
	explainTenant(&v, next)
	if !slices.Contains(difference(before, v.Permissions), TenantInvitationCreate) || len(v.Permissions) != 0 || len(v.ConfiguredPermissions) != 0 || len(v.Roles) != 2 || len(v.Roles[1].Permissions) != 1 {
		t.Fatal(v)
	}
}
