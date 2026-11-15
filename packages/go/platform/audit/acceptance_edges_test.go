package audit_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/notifications"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAuditAtomicTaskAndNotificationFailures(t *testing.T) {
	f := setup(t)
	exportStorage(t, f)
	s := f.business.Audit
	scope := audit.Scope{Kind: "platform"}
	_, e := f.env.Pool.Exec(ctx, `CREATE FUNCTION audit_test_abort() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'synthetic persistence failure';END$$;CREATE TRIGGER audit_test_abort BEFORE INSERT ON task_records FOR EACH ROW WHEN(NEW.source_module='audit') EXECUTE FUNCTION audit_test_abort()`)
	must(t, e)
	command := uuid.NewString()
	in := exportInput(t, s, f.actor, scope, audit.Filter{})
	_, e = s.CreateExport(ctx, f.actor, scope, command, in)
	if e == nil {
		t.Fatal("task failure committed export")
	}
	var roots, facts, receipts int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_exports),(SELECT count(*) FROM account_security_events WHERE event_type='audit.export.requested'),(SELECT count(*) FROM audit_command_receipts r JOIN audit_operations o ON o.id=r.operation_id WHERE o.command_id=$1)`, command).Scan(&roots, &facts, &receipts))
	if roots+facts+receipts != 0 {
		t.Fatal("partial request commit", roots, facts, receipts)
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER audit_test_abort ON task_records;CREATE TRIGGER audit_test_abort BEFORE INSERT ON notification_events FOR EACH ROW WHEN(NEW.source_module='audit') EXECUTE FUNCTION audit_test_abort()`)
	must(t, e)
	ex, e := s.CreateExport(ctx, f.actor, scope, command, in)
	must(t, e)
	_, _ = s.RunExportOnce(ctx)
	v, e := s.Export(ctx, f.actor, scope, ex.ID, "1")
	must(t, e)
	if v.State == "succeeded" || v.File != nil {
		t.Fatal("notification failure partially published")
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM account_security_events WHERE object_id=$1 AND event_type='audit.export.succeeded'`, ex.ID).Scan(&facts))
	if facts != 0 {
		t.Fatal("false terminal audit")
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER audit_test_abort ON notification_events`)
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `UPDATE audit_export_generations SET lease_until=clock_timestamp()-interval '1 second',next_attempt_at=clock_timestamp()-interval '1 second' WHERE export_id=$1`, ex.ID)
	must(t, e)
	_, e = s.RunExportOnce(ctx)
	must(t, e)
	v, e = s.Export(ctx, f.actor, scope, ex.ID, "1")
	must(t, e)
	if v.State != "succeeded" {
		t.Fatalf("recovery did not publish %+v", v)
	}
	n := runtime.ComposeNotifications(f.env.Service.Access())
	_, e = n.SavePreferences(ctx, f.actor, notifications.Preferences{Tasks: false, Updates: false, Revision: "0"})
	must(t, e)
	var event string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM notification_events WHERE source_module='audit' AND source_key=$1`, "export:"+ex.ID+":generation:1:terminal").Scan(&event))
	summary := func() string {
		t.Helper()
		state := ""
		must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
			v, e := notifications.AuditDeliverySummaryTx(ctx, tx, []string{event})
			if e == nil {
				state = v[event].State
				raw := string(ar.JSON(v[event]))
				if strings.Contains(raw, f.actor.ID) || strings.Contains(raw, "preference") || strings.Contains(raw, "ReadAt") {
					t.Fatal("summary leaked personal state")
				}
			}
			return e
		}))
		return state
	}
	if summary() != "pending" {
		t.Fatal("stopped delivery not pending")
	}
	for range 10 {
		worked, e := n.Tick(ctx)
		must(t, e)
		if worked == 0 {
			break
		}
	}
	if summary() != "suppressed" {
		t.Fatal("disabled preference not suppressed", summary())
	}
	for range 2 {
		_, e = n.Tick(ctx)
		must(t, e)
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_events WHERE id=$1`, event).Scan(&facts))
	if facts != 1 {
		t.Fatal("delivery duplicated source fact")
	}
	var recipients int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_recipients WHERE event_id=$1`, event).Scan(&recipients))
	if recipients != 1 {
		t.Fatal("export broadcast to unrelated administrators")
	}
	t.Log("task failure rolls back root/fact/receipt; notification failure rolls back publication; stopped delivery remains pending; disabled preference suppresses once")
}

func TestAuditSecretsProjectionCSVLegacyAndCausality(t *testing.T) {
	f := setup(t)
	store := exportStorage(t, f)
	s := f.business.Audit
	scope := audit.Scope{Kind: "platform"}
	var ref ar.EventRef
	must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		var e error
		ref, e = ar.AppendLegacyTx(ctx, tx, ar.LegacyInput{Event: "role_created", ActorID: f.actor.ID, ObjectType: "role", ObjectID: uuid.NewString(), RequestID: "same-request-is-not-causality", Reason: "password=NO_REASON https://u:NO_URL@example.test/?token=NO_QUERY", Before: map[string]any{"name": nil, "permissions": []string{}}, After: map[string]any{"name": "historical 名称", "password": "NO_PASSWORD", "body": "NO_BODY", "access_token": "NO_TOKEN", "description": strings.Repeat("长", 600)}})
		return e
	}))
	check := func(raw []byte) {
		t.Helper()
		for _, bad := range []string{"NO_REASON", "NO_URL", "NO_QUERY", "NO_PASSWORD", "NO_BODY", "NO_TOKEN"} {
			if bytes.Contains(raw, []byte(bad)) {
				t.Fatal("forbidden marker", bad)
			}
		}
	}
	var stored []byte
	must(t, f.env.Pool.QueryRow(ctx, `SELECT row_to_json(e)::text FROM account_security_events e WHERE id=$1`, ref.ID).Scan(&stored))
	check(stored)
	d, e := s.Event(ctx, f.actor, scope, ref.ID)
	must(t, e)
	check(ar.JSON(d))
	if d.Object.LabelSource != "snapshot" {
		t.Fatal("lost historical name provenance")
	}
	page, e := s.Events(ctx, f.actor, scope, audit.Filter{})
	must(t, e)
	check(ar.JSON(page))
	legacy, e := f.env.Service.Access().AuditEvents(ctx, f.actor, access.Filter{})
	must(t, e)
	check(ar.JSON(legacy))
	_, e = f.env.Service.Access().CreateRole(ctx, f.actor, "same filename", "", access.Meta{Reason: "distinct intent", RequestID: "same-request-is-not-causality"})
	must(t, e)
	trace, e := s.Trace(ctx, f.actor, scope, ref.ID, audit.TraceFilter{})
	must(t, e)
	if len(trace.Items) != 1 {
		t.Fatal("client request ID incorrectly joined operations")
	}
	in := exportInput(t, s, f.actor, scope, audit.Filter{})
	in.FieldProfile = "technical"
	ex, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), in)
	must(t, e)
	_, e = s.RunExportOnce(ctx)
	must(t, e)
	var key string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT object_key FROM audit_export_objects WHERE export_id=$1 AND state='available'`, ex.ID).Scan(&key))
	body, e := store.Get(ctx, key)
	must(t, e)
	csv, e := io.ReadAll(body)
	must(t, e)
	must(t, body.Close())
	check(csv)
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO account_security_events(event_type,object_type,object_id,actor_type,outcome,visibility_class,reason) SELECT 'role_created','role',gen_random_uuid()::text,'system','success','platform','budget fixture' FROM generate_series(1,2001)`)
	must(t, e)
	_, e = s.Events(ctx, f.actor, scope, audit.Filter{Query: "no-match-query-budget"})
	errorCode(t, e, "audit_query_budget_exceeded")
	t.Log("forbidden markers absent from stored fact, list, detail, legacy response and technical CSV; causal trace ignores identical request IDs; candidate cap fails without partial rows")
}

func TestAuditUnavailableDatabaseHasBoundedSafeDiagnostic(t *testing.T) {
	c, e := pgxpool.ParseConfig("postgres://synthetic:NEVER_LOG_PASSWORD@127.0.0.1:1/unavailable?connect_timeout=1")
	must(t, e)
	p, e := pgxpool.NewWithConfig(ctx, c)
	must(t, e)
	defer p.Close()
	var logs bytes.Buffer
	s := access.New(p, slog.New(slog.NewJSONHandler(&logs, nil)))
	a := access.Actor{ID: uuid.NewString(), Check: func(context.Context, pgx.Tx, bool) error { return nil }}
	start := time.Now()
	_, e = s.CreateRole(ctx, a, "unavailable database", "", access.Meta{Reason: "must fail", CommandRequired: true, CommandID: uuid.NewString(), RequestID: "diagnostic-bounded-test"})
	if e == nil {
		t.Fatal("offline database returned success")
	}
	if time.Since(start) > 4*time.Second {
		t.Fatal("failure persistence exceeded bounded retry")
	}
	if !strings.Contains(logs.String(), "audit_persist_failure") || !strings.Contains(logs.String(), "diagnostic-bounded-test") || strings.Contains(logs.String(), "NEVER_LOG_PASSWORD") {
		t.Fatal("unsafe or absent diagnostic", logs.String())
	}
}

func TestAuditLowerConfiguredScopeAndInstanceQuotas(t *testing.T) {
	f := setup(t)
	exportStorage(t, f)
	s := f.business.Audit
	scope := audit.Scope{Kind: "platform"}
	opts := config.DefaultAudit()
	opts.TempDirectory = t.TempDir()
	opts.ActivePerAccount = 2
	opts.ActivePerScope = 1
	opts.ActivePerInstance = 1
	s.WithOptions(opts)
	in := exportInput(t, s, f.actor, scope, audit.Filter{})
	key := uuid.NewString()
	ex, e := s.CreateExport(ctx, f.actor, scope, key, in)
	must(t, e)
	again, e := s.CreateExport(ctx, f.actor, scope, key, in)
	must(t, e)
	if again.ID != ex.ID {
		t.Fatal("replay duplicated quota")
	}
	_, e = s.CreateExport(ctx, f.actor, scope, uuid.NewString(), in)
	errorCode(t, e, "audit_export_limit")
}

func TestAuditMembershipRejoinNoticeAndPermissionSeparation(t *testing.T) {
	f := setup(t)
	exportStorage(t, f)
	s := f.business.Audit
	ts := tenancy.New(f.env.Service.Access())
	as := f.env.Service.Access()
	tenant := f.tenant(t, "audit-edge-membership")
	project := f.project(t, tenant, "audit-edge-project")
	scope := audit.Scope{Kind: "tenant", Tenant: tenant}
	p := f.member(t, "RejoinAuditor", nil)
	a := f.env.Service.AccessActor(p)
	join := func() string {
		t.Helper()
		inv, e := ts.CreateInvitation(ctx, f.actor, tenant, tenancy.InvitationCreateInput{TargetID: p.Account.ID, RequestID: uuid.NewString(), Reason: "audit membership rejoin"}, access.Meta{})
		must(t, e)
		_, e = ts.AcceptInvitation(ctx, a, tenancy.AcceptanceInput{Token: credential(inv.Link), Version: inv.Invitation.Version}, access.Meta{})
		must(t, e)
		var id string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2 AND status='active'`, tenant, p.Account.ID).Scan(&id))
		return id
	}
	member := join()
	role, e := as.CreateTenantRole(ctx, f.actor, tenant, "", access.TenantRoleCreateInput{Name: "Independent audit", Permissions: []access.Code{access.TenantAuditRead, access.TenantAuditExport, access.TenantAuditTechnical}, RequestID: uuid.NewString(), Reason: "audit permission split"}, access.Meta{})
	must(t, e)
	grant := func(id string) {
		t.Helper()
		auth, e := as.TenantMemberAuthorization(ctx, f.actor, tenant, id)
		must(t, e)
		in := access.TenantChange{ExpectedMembershipVersion: auth.MembershipVersion, RoleIDs: []string{role.Role.ID}, Reason: "grant current membership"}
		pre, e := as.PreviewTenantChange(ctx, f.actor, tenant, id, access.OpTenantMemberRolesPreview, in, access.Filter{})
		must(t, e)
		in.PreviewHash = pre.Hash
		in.RoleVersions = pre.RoleVersions
		_, e = as.ChangeTenantAuthorization(ctx, f.actor, tenant, id, access.OpTenantMemberRoles, in, access.Meta{})
		must(t, e)
	}
	grant(member)
	_, e = s.Events(ctx, a, audit.Scope{Kind: "project", Tenant: tenant, Project: project}, audit.Filter{})
	if e == nil {
		t.Fatal("tenant auditor entered project history")
	}
	page, e := s.Events(ctx, a, scope, audit.Filter{EventType: "tenant.project.governance"})
	must(t, e)
	if len(page.Items) != 0 {
		t.Fatal("governance lacked paired project permission")
	}
	page, e = s.Events(ctx, a, scope, audit.Filter{Limit: 1})
	must(t, e)
	if page.NextCursor == "" {
		t.Fatal("rejoin cursor missing")
	}
	cursor := page.NextCursor
	ex, e := s.CreateExport(ctx, a, scope, uuid.NewString(), exportInput(t, s, a, scope, audit.Filter{}))
	must(t, e)
	_, e = s.RunExportOnce(ctx)
	must(t, e)
	n := runtime.ComposeNotifications(as)
	for range 20 {
		worked, e := n.Tick(ctx)
		must(t, e)
		if worked == 0 {
			break
		}
	}
	inbox, e := n.List(ctx, a, notifications.Filter{Category: "tasks"})
	must(t, e)
	detail, e := ts.Member(ctx, f.actor, tenant, member)
	must(t, e)
	_, e = ts.ChangeMember(ctx, f.actor, tenant, member, access.OpTenantMemberRemove, tenancy.MemberInput{Version: detail.Version, Reason: "remove old relationship"}, access.Meta{})
	must(t, e)
	_, e = s.Events(ctx, a, scope, audit.Filter{})
	if e == nil {
		t.Fatal("removed member retained audit")
	}
	next := join()
	if next == member {
		t.Fatal("removed relationship reused identity")
	}
	grant(next)
	_, e = s.Events(ctx, a, scope, audit.Filter{Limit: 1, Cursor: cursor})
	errorCode(t, e, "audit_cursor_stale")
	_, e = s.OpenRead(ctx, a, scope, ex.ID, "1", "HEAD", "rejoined-denial")
	errorCode(t, e, "audit_export_invalidated")
	for _, item := range inbox.Items {
		if _, e = n.Get(ctx, a, item.ID); e == nil {
			t.Fatal("old relationship notification revived")
		}
	}
	manager := f.member(t, "ManageOnlyAudit", []access.Code{access.PlatformAuditManage})
	_, e = s.Exports(ctx, f.env.Service.AccessActor(manager), audit.Scope{Kind: "platform"}, audit.ExportFilter{}, true)
	if e == nil {
		t.Fatal("manage without read gained list")
	}
	sub := (&access.ReadSubject{ID: uuid.NewString(), Tenant: tenant, Project: project, Check: func(context.Context, pgx.Tx) error { return nil }}).ReadActor()
	_, e = s.Capabilities(ctx, sub, audit.Scope{Kind: "project", Tenant: tenant, Project: project})
	if e == nil {
		t.Fatal("ReadSubject entered personnel audit")
	}
	pre, e := ts.PreviewStatus(ctx, f.actor, tenant, "disabled")
	must(t, e)
	notice := "公开维护说明"
	_, e = ts.ChangeStatus(ctx, f.actor, tenant, "disabled", tenancy.StatusInput{Version: pre.Version, Hash: pre.Hash, Reason: "PRIVATE_INTERNAL_REASON", Notice: &notice}, access.Meta{})
	must(t, e)
	_, e = s.Events(ctx, a, scope, audit.Filter{})
	if e == nil {
		t.Fatal("disabled tenant audit remained accessible")
	}
	pre, e = ts.PreviewStatus(ctx, f.actor, tenant, "active")
	must(t, e)
	_, e = ts.ChangeStatus(ctx, f.actor, tenant, "active", tenancy.StatusInput{Version: pre.Version, Hash: pre.Hash, Reason: "PRIVATE_ENABLE_REASON"}, access.Meta{})
	must(t, e)
	page, e = s.Events(ctx, a, scope, audit.Filter{EventType: "tenant.disabled"})
	must(t, e)
	raw := string(ar.JSON(page))
	if strings.Contains(raw, "PRIVATE_") || !strings.Contains(raw, notice) {
		t.Fatal("tenant notice disclosure", raw)
	}
	platform, e := s.Events(ctx, f.actor, audit.Scope{Kind: "platform"}, audit.Filter{EventType: "tenant.disabled"})
	must(t, e)
	if !strings.Contains(string(ar.JSON(platform)), "PRIVATE_INTERNAL_REASON") {
		t.Fatal("platform lost internal reason")
	}
	t.Log("real removal/rejoin invalidates old cursor, export and notification; tenant/project/read/manage/ReadSubject separation and public notice verified")
}
