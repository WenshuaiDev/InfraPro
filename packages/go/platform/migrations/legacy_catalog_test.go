package migrations

import (
	"context"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Seed the old schema directly. The current command intentionally requires the
// current audit protocol and must never be executed against a pre-upgrade schema.
func seedLegacyCatalog(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range access.Catalog() {
		exec(`INSERT INTO permission_catalog(code,name,group_name,scope,description,risk,lifecycle,tenant_custom_role_assignable,project_custom_role_assignable,catalog_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(code) DO UPDATE SET name=$2,group_name=$3,scope=$4,description=$5,risk=$6,lifecycle=$7,tenant_custom_role_assignable=$8,project_custom_role_assignable=$9,catalog_version=$10`, p.Code, p.Name, p.Group, p.Scope, p.Description, p.Risk, p.Lifecycle, p.TenantCustomRoleAssignable, p.ProjectCustomRoleAssignable, access.Checksum())
	}
	for _, b := range access.Builtins() {
		var id string
		err := tx.QueryRow(ctx, `INSERT INTO roles(name,name_normalized,description,builtin_key,binding_scope,assignment_policy) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(builtin_key) DO UPDATE SET name=$1,name_normalized=$2,description=$3,binding_scope=$5,assignment_policy=$6 RETURNING id::text`, b.Name, strings.ToLower(b.Name), b.Description, b.Key, b.BindingScope, b.AssignmentPolicy).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		exec(`DELETE FROM role_permissions WHERE role_id=$1`, id)
		for _, code := range b.Permissions {
			exec(`INSERT INTO role_permissions(role_id,permission_code,binding_scope) VALUES($1,$2,$3)`, id, code, b.BindingScope)
		}
	}
	exec(`UPDATE authorization_state SET catalog_checksum=$1 WHERE id=1`, access.Checksum())
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
