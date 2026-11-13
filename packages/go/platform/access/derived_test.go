package access_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
)

func TestDerivedPermissionsStayExplicitAndScoped(t *testing.T) {
	for _, scope := range []string{"tenant", "project"} {
		for _, action := range []string{"templates.list", "templates.expand", "metrics.list", "metrics.detail", "metrics.versions", "versions.detail"} {
			p, ok := access.PolicyFor(access.DerivedOperation(scope, action))
			if !ok || p.Class != scope || !slices.Contains(p.Permissions, access.DerivedCode(scope, "metrics.read")) || access.ProjectActiveOnly(p.Operation) {
				t.Fatal("read authorization mismatch", scope, action, p)
			}
		}
	}
	if _, ok := access.PolicyFor(access.DerivedOperation("project", "metrics.create")); ok {
		t.Fatal("project path must not modify tenant definitions")
	}
	for _, b := range access.Builtins() {
		for _, p := range access.Catalog() {
			if !strings.Contains(string(p.Code), ".derived.") {
				continue
			}
			want := b.Key == access.TenantAdminKey && p.Scope == "tenant" || b.Key == access.ProjectAdminKey && p.Scope == "project"
			if slices.Contains(b.Permissions, p.Code) != want {
				t.Fatal("implicit grant or missing administrator grant", b.Key, p.Code)
			}
		}
	}
}
