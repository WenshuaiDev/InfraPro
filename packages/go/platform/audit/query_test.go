package audit_test

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ctx = context.Background()

const password = "Audit tests synthetic bridge 2026!"

func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func errorCode(t *testing.T, e error, want string) {
	t.Helper()
	var a *audit.Error
	var b *access.Error
	got := ""
	if errors.As(e, &a) {
		got = a.Code
	}
	if errors.As(e, &b) {
		got = b.Code
	}
	if got != want {
		t.Fatalf("want %s, got %s (%v)", want, got, e)
	}
}

type fixture struct {
	env      *testsupport.Environment
	business runtime.Business
	admin    identity.Principal
	actor    access.Actor
}

func setup(t *testing.T) fixture {
	t.Helper()
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run pnpm test:audit")
	}
	e, err := testsupport.New(ctx, slog.Default())
	must(t, err)
	t.Cleanup(e.Close)
	_, err = e.Service.Bootstrap(ctx, "AuditPlatform", "平台验收", password, identity.Meta{})
	must(t, err)
	p := login(t, e, "AuditPlatform")
	return fixture{e, runtime.Compose(e.Service.Access()), p, e.Service.AccessActor(p)}
}
func login(t *testing.T, e *testsupport.Environment, user string) identity.Principal {
	t.Helper()
	_, token, _, err := e.Service.Login(ctx, user, password, identity.Meta{})
	must(t, err)
	p, err := e.Service.Authenticate(ctx, token)
	must(t, err)
	return p
}
func credential(link string) string {
	u, _ := url.Parse(link)
	q, _ := url.ParseQuery(u.Fragment)
	return q.Get("token")
}
func (f fixture) member(t *testing.T, name string, codes []access.Code) identity.Principal {
	t.Helper()
	l, e := f.env.Service.CreateInvitation(ctx, f.admin, "", "", identity.Meta{})
	must(t, e)
	must(t, f.env.Service.AcceptInvitation(ctx, credential(l.URL), name, name, password, identity.Meta{}))
	p := login(t, f.env, name)
	if codes != nil {
		s := f.env.Service.Access()
		role, e := s.CreateRole(ctx, f.actor, name+"权限", "", access.Meta{Reason: "审计权限自验"})
		must(t, e)
		role, e = s.ChangeRole(ctx, f.actor, role.ID, access.OpRoleSetPermissions, access.RoleChange{ExpectedVersion: role.Version, Permissions: codes}, access.Meta{Reason: "审计权限自验"})
		must(t, e)
		account, e := s.Account(ctx, f.actor, p.Account.ID)
		must(t, e)
		_, e = s.AssignRoles(ctx, f.actor, p.Account.ID, account.Version, []string{role.ID}, access.Meta{Reason: "审计权限自验"})
		must(t, e)
	}
	return login(t, f.env, name)
}
func (f fixture) tenant(t *testing.T, code string) string {
	t.Helper()
	r, e := tenancy.New(f.env.Service.Access()).Create(ctx, f.actor, tenancy.CreateInput{Fields: tenancy.Fields{Name: "审计租户 " + code, Code: code}, AdministratorID: f.admin.Account.ID, RequestID: uuid.NewString(), Reason: "真实租户创建自验"}, access.Meta{})
	must(t, e)
	return r.TenantID
}
func (f fixture) project(t *testing.T, tenant, code string) string {
	t.Helper()
	var member string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2 AND status='active'`, tenant, f.admin.Account.ID).Scan(&member))
	r, e := projects.New(f.env.Service.Access()).Create(ctx, f.actor, tenant, projects.CreateInput{Fields: projects.Fields{Name: "审计项目 " + code, Code: code, Purpose: "验证审计空间隔离"}, WriteMeta: projects.WriteMeta{RequestID: uuid.NewString(), Reason: "真实项目创建自验"}, Owner: member, Administrator: member}, access.Meta{})
	must(t, e)
	return r.ProjectID
}

func TestAuditThreeScopesAndBoundCursors(t *testing.T) {
	f := setup(t)
	tenant := f.tenant(t, "audit-a")
	project := f.project(t, tenant, "audit-a-1")
	other := f.tenant(t, "audit-b")
	f.project(t, other, "audit-b-1")
	s := f.business.Audit
	platform := audit.Scope{Kind: "platform"}
	ts := audit.Scope{Kind: "tenant", Tenant: tenant}
	ps := audit.Scope{Kind: "project", Tenant: tenant, Project: project}
	page, e := s.Events(ctx, f.actor, platform, audit.Filter{EventType: "project.ProjectCreate"})
	must(t, e)
	if len(page.Items) != 0 {
		t.Fatal("project history leaked to platform")
	}
	page, e = s.Events(ctx, f.actor, ts, audit.Filter{EventType: "project.ProjectCreate"})
	must(t, e)
	if len(page.Items) != 0 {
		t.Fatal("project full history leaked to tenant")
	}
	page, e = s.Events(ctx, f.actor, ps, audit.Filter{EventType: "project.ProjectCreate"})
	must(t, e)
	if len(page.Items) != 1 {
		t.Fatalf("project creation missing: %+v", page)
	}
	projectEvent := page.Items[0].ID
	_, e = s.Event(ctx, f.actor, platform, projectEvent)
	errorCode(t, e, "audit_not_found")
	_, e = s.Event(ctx, f.actor, audit.Scope{Kind: "project", Tenant: other, Project: project}, projectEvent)
	if e == nil {
		t.Fatal("cross-tenant project ID accepted")
	}
	reader := f.member(t, "OnlyPlatform", []access.Code{access.AuditRead})
	caps, e := s.Capabilities(ctx, f.env.Service.AccessActor(reader), platform)
	must(t, e)
	if !caps.Read || caps.Export || caps.Technical || caps.Manage {
		t.Fatal("audit capability separation failed", caps)
	}
	_, e = s.Events(ctx, f.env.Service.AccessActor(reader), ts, audit.Filter{})
	if e == nil {
		t.Fatal("pure platform account entered tenant audit")
	}
	page, e = s.Events(ctx, f.env.Service.AccessActor(reader), platform, audit.Filter{Query: "审计租户"})
	must(t, e)
	if len(page.Items) != 0 {
		t.Fatal("hidden tenant governance searchable")
	}
	page, e = s.Events(ctx, f.actor, platform, audit.Filter{Limit: 1})
	must(t, e)
	if page.NextCursor == "" {
		t.Fatal("no cursor")
	}
	cursor := page.NextCursor
	second, e := s.Events(ctx, f.actor, platform, audit.Filter{Limit: 1, Cursor: cursor})
	must(t, e)
	if len(second.Items) != 1 || second.Items[0].ID == page.Items[0].ID {
		t.Fatal("pagination repeated or lost record")
	}
	_, e = s.Events(ctx, f.env.Service.AccessActor(reader), platform, audit.Filter{Limit: 1, Cursor: cursor})
	errorCode(t, e, "audit_cursor_stale")
	_, e = s.Events(ctx, f.actor, platform, audit.Filter{Limit: 1, Cursor: cursor + "tampered"})
	errorCode(t, e, "audit_cursor_stale")
	_, e = s.Events(ctx, f.actor, ts, audit.Filter{Limit: 1, Cursor: cursor})
	errorCode(t, e, "audit_cursor_stale")
	_, e = s.Events(ctx, f.actor, platform, audit.Filter{Limit: 1, Cursor: cursor, Query: "changed"})
	errorCode(t, e, "audit_cursor_stale")
	_, e = f.env.Service.Access().CreateRole(ctx, f.actor, "权限修订变化", "", access.Meta{Reason: "使旧游标失效"})
	must(t, e)
	_, e = s.Events(ctx, f.actor, platform, audit.Filter{Limit: 1, Cursor: cursor})
	errorCode(t, e, "audit_cursor_stale")
}

func TestAuditInvitationEffectsAndLegacyUnknown(t *testing.T) {
	f := setup(t)
	tenant := f.tenant(t, "audit-invite")
	p := f.member(t, "InviteAudit", nil)
	ts := tenancy.New(f.env.Service.Access())
	invitation, e := ts.CreateInvitation(ctx, f.actor, tenant, tenancy.InvitationCreateInput{TargetID: p.Account.ID, RequestID: uuid.NewString(), Reason: "同一操作的两个实际效果"}, access.Meta{})
	must(t, e)
	_, e = ts.AcceptInvitation(ctx, f.env.Service.AccessActor(p), tenancy.AcceptanceInput{Token: credential(invitation.Link), Version: invitation.Invitation.Version}, access.Meta{RequestID: "same-client-request"})
	must(t, e)
	s := f.business.Audit
	scope := audit.Scope{Kind: "tenant", Tenant: tenant}
	page, e := s.Events(ctx, f.actor, scope, audit.Filter{EventType: "tenant.invitation.accepted"})
	must(t, e)
	if len(page.Items) != 1 {
		t.Fatal("invitation effect missing")
	}
	trace, e := s.Trace(ctx, f.actor, scope, page.Items[0].ID, audit.TraceFilter{})
	must(t, e)
	found := false
	for _, event := range trace.Items {
		if event.EventType == "tenant.member.joined" {
			found = true
			if event.OperationID != page.Items[0].OperationID {
				t.Fatal("effects lost operation identity")
			}
		}
	}
	if !found {
		t.Fatal("member effect missing from explicit operation")
	}
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO account_security_events(event_type,outcome,reason) VALUES('unregistered.legacy','success','UNKNOWN_HIDDEN_NAME')`)
	must(t, e)
	page, e = s.Events(ctx, f.actor, audit.Scope{Kind: "platform"}, audit.Filter{Query: "UNKNOWN_HIDDEN_NAME"})
	must(t, e)
	if len(page.Items) != 0 {
		t.Fatal("unknown legacy event treated as platform")
	}
}

func TestAuditMetadataRedactionAndQueryFailures(t *testing.T) {
	f := setup(t)
	tenant := f.tenant(t, "audit-redaction")
	scope := audit.Scope{Kind: "tenant", Tenant: tenant}
	var id ar.EventRef
	// Local projection fixture: an absent business object must not restore its
	// historical name or diff merely because its audit fact is still retained.
	must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		var e error
		id, e = ar.AppendLegacyTx(ctx, tx, ar.LegacyInput{Event: "resource.update", ObjectType: "engineering_facility", ObjectID: uuid.NewString(), ActorID: f.admin.Account.ID, Tenant: tenant, Entry: "tenant", Before: map[string]any{"name": "HIDDEN_OLD_NAME"}, After: map[string]any{"name": "HIDDEN_NEW_NAME"}, Reason: "HIDDEN_REASON"})
		return e
	}))
	detail, e := f.business.Audit.Event(ctx, f.actor, scope, id.ID)
	must(t, e)
	encoded := string(ar.JSON(detail))
	if strings.Contains(encoded, "HIDDEN_") || detail.Object.LabelState != ar.Redacted {
		t.Fatal("historical content bypassed current metadata authorization", encoded)
	}
	page, e := f.business.Audit.Events(ctx, f.actor, scope, audit.Filter{Query: "HIDDEN_NEW_NAME"})
	must(t, e)
	if len(page.Items) != 0 {
		t.Fatal("hidden content searchable")
	}
	_, e = f.business.Audit.Events(ctx, f.actor, scope, audit.Filter{From: time.Now().Add(-367 * 24 * time.Hour).Format(time.RFC3339), To: time.Now().Format(time.RFC3339)})
	errorCode(t, e, "audit_filter_invalid")
	_, e = f.env.Pool.Exec(ctx, `ALTER TABLE engineering_resource_nodes RENAME TO audit_test_source_unavailable`)
	must(t, e)
	_, e = f.business.Audit.Event(ctx, f.actor, scope, id.ID)
	errorCode(t, e, "audit_source_unavailable")
	_, e = f.env.Pool.Exec(ctx, `ALTER TABLE audit_test_source_unavailable RENAME TO engineering_resource_nodes`)
	must(t, e)
}
