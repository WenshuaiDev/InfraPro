package access_test

import (
	"slices"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
)

func TestAuditNamesOutcomesAndScope(t *testing.T) {
	e := env(t)
	p := principal(t, e, "Admin")
	s, actor := e.Service.Access(), e.Service.AccessActor(p)
	r := role(t, e, p, "审计原始角色名", []access.Code{access.AuditRead})
	_, err := s.ChangeRole(ctx, actor, r.ID, access.OpRoleDelete, access.RoleChange{ExpectedVersion: r.Version}, meta())
	must(t, err)
	page, err := s.AuditEvents(ctx, actor, access.Filter{Query: "审计原始角色名", Category: "management"})
	must(t, err)
	if len(page.Items) != 3 {
		t.Fatalf("role history: %+v", page)
	}
	for _, event := range page.Items {
		d := event.Display
		if d == nil || d.ActorName != "管理" || d.ActorUsername != "Admin" || d.ActorNameSource != "current" || d.TargetName != r.Name || d.TargetNameSource != "snapshot" || d.Outcome != "success" {
			t.Fatalf("unreadable role: %+v", d)
		}
		if event.EventType == "RoleSetPermissions" && d.PermissionNames[string(access.AuditRead)] != "查看平台审计" {
			t.Fatalf("missing permission names: %+v", d)
		}
	}
	_, _, _, err = e.Service.Login(ctx, "Admin", secret+"wrong", identity.Meta{IP: "192.0.2.8", UserAgent: "audit test"})
	if err == nil {
		t.Fatal("wrong password accepted")
	}
	page, err = s.AuditEvents(ctx, actor, access.Filter{Category: "security", Outcome: "failure", Query: "Admin"})
	must(t, err)
	if len(page.Items) != 1 || page.Items[0].ActorID != "" || page.Items[0].Display.ActorName != "" || page.Items[0].Display.TargetName != "管理" || page.Items[0].Display.SourceIP != "192.0.2.8" {
		t.Fatalf("failed login misattributed: %+v", page)
	}
	page, err = s.AuditEvents(ctx, actor, access.Filter{Category: "system", Event: "bootstrap"})
	must(t, err)
	if len(page.Items) != 1 || page.Items[0].Display.Outcome != "success" {
		t.Fatal("missing initialization outcome")
	}

	reader := member(t, e, p, "AuditReader")
	readerRole := role(t, e, p, "仅审计", []access.Code{access.AuditRead})
	assign(t, e, p, reader.Account.ID, []string{readerRole.ID})
	var requestID string
	must(t, e.Pool.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&requestID))
	tenant, err := tenancy.New(s).Create(ctx, actor, tenancy.CreateInput{Fields: tenancy.Fields{Name: "不能越权搜索的租户", Code: "audit-scope"}, AdministratorID: p.Account.ID, RequestID: requestID, Reason: "审计范围回归"}, access.Meta{})
	must(t, err)
	page, err = s.AuditEvents(ctx, actor, access.Filter{TenantID: tenant.TenantID, Query: "不能越权搜索的租户"})
	must(t, err)
	if len(page.Items) != 1 || page.Items[0].Display.TenantName != "不能越权搜索的租户" || page.Items[0].Display.ActorNameSource != "snapshot" {
		t.Fatalf("missing tenant display: %+v", page)
	}
	tenantEvent := page.Items[0].ID
	for _, query := range []string{"不能越权搜索的租户", "审计范围回归"} {
		limited, err := s.AuditEvents(ctx, e.Service.AccessActor(reader), access.Filter{Query: query})
		must(t, err)
		if len(limited.Items) != 0 {
			t.Fatal("tenant audit leaked through search")
		}
	}
	_, err = s.AuditEvents(ctx, e.Service.AccessActor(reader), access.Filter{TenantID: tenant.TenantID})
	code(t, err, "permission_denied")
	_, err = s.AuditEvents(ctx, e.Service.AccessActor(reader), access.Filter{Cursor: tenantEvent})
	code(t, err, "validation_failed")
	page, err = s.AuditEvents(ctx, e.Service.AccessActor(reader), access.Filter{Query: "审计原始角色名"})
	must(t, err)
	if len(page.Items) != 3 || page.Items[0].Display.TargetName != r.Name {
		t.Fatal("readable audit incorrectly requires role/account read permissions")
	}
}

func TestAuditChronologicalPagination(t *testing.T) {
	e := env(t)
	p := principal(t, e, "Admin")
	s, actor := e.Service.Access(), e.Service.AccessActor(p)
	ids := []string{}
	// Cross the single/double-digit ID boundary: ordering the text projection of
	// an ID would disagree with the numeric keyset predicate and skip records.
	for _, hour := range []int{2, 1, 3, 2, 2, 2, 2, 2, 2, 2, 2, 2} {
		var id string
		must(t, e.Pool.QueryRow(ctx, `INSERT INTO account_security_events(event_type,actor_id,account_id,object_type,object_id,outcome,reason,created_at) VALUES('audit.test',$1::uuid,$1::uuid,'account',$1::text,'success','时间分页回归',$2) RETURNING id::text`, p.Account.ID, time.Date(2026, 1, 1, hour, 0, 0, 0, time.UTC)).Scan(&id))
		ids = append(ids, id)
	}
	first, err := s.AuditEvents(ctx, actor, access.Filter{Query: "时间分页回归", Limit: 2})
	must(t, err)
	if len(first.Items) != 2 || first.Items[0].ID != ids[2] || first.Items[1].ID != ids[11] || first.NextCursor == "" {
		t.Fatalf("first page: %+v", first)
	}
	second, err := s.AuditEvents(ctx, actor, access.Filter{Query: "时间分页回归", Limit: 2, Cursor: first.NextCursor})
	must(t, err)
	if len(second.Items) != 2 || second.Items[0].ID != ids[10] || second.Items[1].ID != ids[9] || second.NextCursor == "" {
		t.Fatalf("second page: %+v", second)
	}
	seen := []string{first.Items[0].ID, first.Items[1].ID}
	next := first.NextCursor
	for next != "" {
		page, err := s.AuditEvents(ctx, actor, access.Filter{Query: "时间分页回归", Limit: 2, Cursor: next})
		must(t, err)
		for _, event := range page.Items {
			seen = append(seen, event.ID)
		}
		if len(seen) > len(ids) {
			t.Fatal("pagination repeated records")
		}
		next = page.NextCursor
	}
	expected := []string{ids[2]}
	for i := len(ids) - 1; i >= 3; i-- {
		expected = append(expected, ids[i])
	}
	expected = append(expected, ids[0], ids[1])
	if !slices.Equal(seen, expected) {
		t.Fatalf("page traversal: got %v want %v", seen, expected)
	}
	legacy, err := s.AuditEvents(ctx, actor, access.Filter{Query: "时间分页回归", Cursor: ids[3], Limit: 2})
	must(t, err)
	if len(legacy.Items) != 2 || legacy.Items[0].ID != ids[0] {
		t.Fatal("legacy cursor is incompatible")
	}
	bounded, err := s.AuditEvents(ctx, actor, access.Filter{Query: "时间分页回归", From: "2026-01-01T02:00:00Z", To: "2026-01-01T02:00:00Z"})
	must(t, err)
	if len(bounded.Items) != 10 || !slices.Equal([]string{bounded.Items[0].ID, bounded.Items[9].ID}, []string{ids[11], ids[0]}) {
		t.Fatal("time boundary lost events")
	}
	for _, filter := range []access.Filter{{Cursor: "invalid"}, {Cursor: "0"}, {Category: "bad"}, {Outcome: "bad"}, {From: "bad"}, {From: "2026-02-01T00:00:00Z", To: "2026-01-01T00:00:00Z"}} {
		_, err := s.AuditEvents(ctx, actor, filter)
		code(t, err, "validation_failed")
	}
}
