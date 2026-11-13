package derivedmonitoring_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ctx = context.Background()

const password = "Derived river 桥梁 2026!"

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		b, _ := json.Marshal(err)
		t.Fatalf("%v: %s", err, b)
	}
}
func code(t *testing.T, err error, want string) {
	t.Helper()
	got := identity.PublicError(err).Code
	var de *dm.Error
	if errors.As(err, &de) {
		got = de.Code
	}
	if err == nil || got != want {
		t.Fatalf("want %s, got %s: %v", want, got, err)
	}
}

type fixture struct {
	e               *testsupport.Environment
	s               *dm.Service
	engineering     *eng.Service
	platform, admin identity.Principal
	scope           dm.Scope
	unit            eng.Result
	rule            json.RawMessage
}

func (f fixture) actor(p identity.Principal) access.Actor { return f.e.Service.AccessActor(p) }
func wm(rev string) dm.WriteMeta {
	return dm.WriteMeta{RequestID: uuid.NewString(), Revision: rev, Reason: "派生功能自验"}
}
func ewm() eng.WriteMeta {
	return eng.WriteMeta{RequestID: uuid.NewString(), Reason: "派生功能自验"}
}
func login(t *testing.T, e *testsupport.Environment, name string) identity.Principal {
	t.Helper()
	_, token, _, err := e.Service.Login(ctx, name, password, identity.Meta{})
	must(t, err)
	p, err := e.Service.Authenticate(ctx, token)
	must(t, err)
	return p
}
func (f fixture) member(t *testing.T, name string) identity.Principal {
	t.Helper()
	l, err := f.e.Service.CreateInvitation(ctx, f.platform, "", "", identity.Meta{})
	must(t, err)
	u, err := url.Parse(l.URL)
	must(t, err)
	q, err := url.ParseQuery(u.Fragment)
	must(t, err)
	must(t, f.e.Service.AcceptInvitation(ctx, q.Get("token"), name, name, password, identity.Meta{}))
	return login(t, f.e, name)
}
func setup(t *testing.T) fixture {
	t.Helper()
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("requires isolated test database")
	}
	e, err := testsupport.New(ctx, slog.Default())
	must(t, err)
	t.Cleanup(e.Close)
	_, err = e.Service.Bootstrap(ctx, "Platform", "平台", password, identity.Meta{})
	must(t, err)
	f := fixture{e: e, s: dm.New(e.Service.Access()), engineering: eng.New(e.Service.Access()), platform: login(t, e, "Platform")}
	f.admin = f.member(t, "DerivedAdmin")
	tenant, err := tenancy.New(e.Service.Access()).Create(ctx, f.actor(f.platform), tenancy.CreateInput{Fields: tenancy.Fields{Name: "派生测试", Code: "derived-test"}, AdministratorID: f.admin.Account.ID, RequestID: uuid.NewString(), Reason: "派生测试"}, access.Meta{})
	must(t, err)
	f.scope = dm.TenantScope(tenant.TenantID)
	_, err = f.engineering.SaveCategory(ctx, f.actor(f.platform), eng.CategoryInput{WriteMeta: ewm(), Key: "length", Name: "长度", Description: "空间长度"}, access.Meta{})
	must(t, err)
	u, err := f.engineering.Create(ctx, f.actor(f.platform), eng.Scope{Kind: eng.UnitDefinition}, eng.CreateInput{WriteMeta: ewm(), Code: "mm", Content: eng.Content{MetamodelVersion: "1", Name: "毫米", Unit: &eng.Unit{Symbol: "mm", Category: "length"}}}, access.Meta{})
	must(t, err)
	f.unit, err = f.engineering.Publish(ctx, f.actor(f.platform), eng.Scope{Kind: eng.UnitDefinition}, u.DefinitionID, eng.RevisionInput{WriteMeta: ewm(), Revision: u.Revision}, access.Meta{})
	must(t, err)
	rule, err := f.s.ExpandTemplate(ctx, f.actor(f.admin), f.scope, "difference", dm.ExpandTemplateInput{Inputs: []dm.InputSpec{{Key: "x", Name: "位移", Shape: "numeric", SourceKinds: []string{"direct", "derived"}, Quantity: dm.Quantity{BaseUnitVersionID: f.unit.VersionID, BasePower: 1, Mode: "level", LevelScale: "ratio"}, Direction: "must_match", Reference: "must_match", Time: dm.TimeSemantics{Kind: "instant"}}}})
	must(t, err)
	f.rule, err = json.Marshal(rule)
	must(t, err)
	return f
}
func (f fixture) create(t *testing.T, name string) dm.MetricResult {
	t.Helper()
	v, err := f.s.CreateMetric(ctx, f.actor(f.admin), f.scope, dm.CreateMetricInput{WriteMeta: wm(""), Code: name, Name: name, Description: "定义测试", Rule: f.rule}, access.Meta{})
	must(t, err)
	return v
}
func (f fixture) publishInput(t *testing.T, d dm.MetricDefinition) dm.PublishMetricInput {
	t.Helper()
	v, err := f.s.ValidateMetric(ctx, f.actor(f.admin), f.scope, d.ID, dm.ValidateInput{DraftRevision: d.Draft.Revision})
	must(t, err)
	return dm.PublishMetricInput{WriteMeta: wm(d.Revision), DraftRevision: d.Draft.Revision, ValidationHash: v.ValidationHash}
}
func (f fixture) publish(t *testing.T, d dm.MetricDefinition) dm.MetricResult {
	t.Helper()
	v, err := f.s.PublishMetric(ctx, f.actor(f.admin), f.scope, d.ID, f.publishInput(t, d), access.Meta{})
	must(t, err)
	return v
}

func TestDefinitionLifecycleConcurrencyAndPermanentVersions(t *testing.T) {
	f := setup(t)
	actor := f.actor(f.admin)
	in := dm.CreateMetricInput{WriteMeta: wm(""), Code: "delta", Name: "变化", Rule: f.rule}
	d, err := f.s.CreateMetric(ctx, actor, f.scope, in, access.Meta{})
	must(t, err)
	replay, err := f.s.CreateMetric(ctx, actor, f.scope, in, access.Meta{})
	must(t, err)
	if !replay.Replayed || replay.Definition.ID != d.Definition.ID {
		t.Fatal("create replay changed identity")
	}
	in.Name = "different"
	_, err = f.s.CreateMetric(ctx, actor, f.scope, in, access.Meta{})
	code(t, err, "idempotency_conflict")
	publish := f.publishInput(t, d.Definition)
	type outcome struct {
		result dm.MetricResult
		err    error
	}
	ch := make(chan outcome, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			v, e := f.s.PublishMetric(ctx, actor, f.scope, d.Definition.ID, publish, access.Meta{})
			ch <- outcome{v, e}
		})
	}
	wg.Wait()
	close(ch)
	var published dm.MetricResult
	replays := 0
	for r := range ch {
		must(t, r.err)
		if r.result.Replayed {
			replays++
		}
		if published.Version != nil && published.Version.ID != r.result.Version.ID {
			t.Fatal("duplicate publish")
		}
		published = r.result
	}
	if replays != 1 || published.Version.Number != "1" || published.Definition.Draft != nil {
		t.Fatal("publish did not atomically replace draft", published)
	}
	for _, sql := range []string{`UPDATE derived_metric_versions SET rule='{}',state_revision=state_revision+1 WHERE id=$1`, `DELETE FROM derived_metric_versions WHERE id=$1`} {
		_, err = f.e.Pool.Exec(ctx, sql, published.Version.ID)
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != "23514" {
			t.Fatal("version not immutable", err)
		}
	}
	_, err = f.s.DeleteMetricDraft(ctx, actor, f.scope, d.Definition.ID, wm(published.Definition.Revision), access.Meta{})
	code(t, err, "definition_in_use")
	save := dm.SaveMetricInput{WriteMeta: wm(published.Definition.Revision), Name: "新版变化", Rule: f.rule, BaseVersionID: published.Version.ID}
	draft, err := f.s.SaveMetricDraft(ctx, actor, f.scope, d.Definition.ID, save, access.Meta{})
	must(t, err)
	save.RequestID = uuid.NewString()
	_, err = f.s.SaveMetricDraft(ctx, actor, f.scope, d.Definition.ID, save, access.Meta{})
	code(t, err, "revision_conflict")
	old, err := f.s.MetricVersion(ctx, actor, f.scope, published.Version.ID)
	must(t, err)
	if old.Name != "变化" {
		t.Fatal("draft modified published content")
	}
	validation := f.publishInput(t, draft.Definition)
	validation.ValidationHash = "changed"
	_, err = f.s.PublishMetric(ctx, actor, f.scope, d.Definition.ID, validation, access.Meta{})
	code(t, err, "preview_changed")
	second := f.publish(t, draft.Definition)
	if second.Version.Number != "2" || second.Version.ID == old.ID {
		t.Fatal(second)
	}
	disabled, err := f.s.SetMetricVersionState(ctx, actor, f.scope, old.ID, dm.VersionStateInput{WriteMeta: wm(old.StateRevision), Enabled: false}, access.Meta{})
	must(t, err)
	if disabled.Version.Usable || disabled.Version.ContentHash != old.ContentHash {
		t.Fatal("disable changed content")
	}
	copy, err := f.s.CopyMetric(ctx, actor, f.scope, d.Definition.ID, dm.CopyMetricInput{WriteMeta: wm(""), SourceVersionID: old.ID, Code: "independent", Name: "独立复制"}, access.Meta{})
	must(t, err)
	if copy.Definition.ID == d.Definition.ID || copy.Definition.Draft == nil {
		t.Fatal("copy did not get independent identity")
	}
	deleted, err := f.s.DeleteMetricDraft(ctx, actor, f.scope, copy.Definition.ID, wm(copy.Definition.Revision), access.Meta{})
	must(t, err)
	if deleted.Definition.Status != "deleted" {
		t.Fatal(deleted)
	}
	_, err = f.s.CreateMetric(ctx, actor, f.scope, dm.CreateMetricInput{WriteMeta: wm(""), Code: "INDEPENDENT", Name: "编号重用", Rule: f.rule}, access.Meta{})
	code(t, err, "definition_conflict")
	page, err := f.s.MetricVersions(ctx, actor, f.scope, d.Definition.ID, dm.MetricFilter{Limit: 1})
	must(t, err)
	if len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatal("missing version cursor", page)
	}
	next, err := f.s.MetricVersions(ctx, actor, f.scope, d.Definition.ID, dm.MetricFilter{Limit: 1, Cursor: page.NextCursor})
	must(t, err)
	if len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID {
		t.Fatal("version cursor repeated")
	}
	var changes, receipts int
	must(t, f.e.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM derived_changes),(SELECT count(*) FROM derived_operation_receipts)`).Scan(&changes, &receipts))
	if changes != receipts || changes != 7 {
		t.Fatal("failed or replayed command wrote audit evidence", changes, receipts)
	}
}

func TestDefinitionIsolationPublishedProjectionAndDependencyState(t *testing.T) {
	f := setup(t)
	actor := f.actor(f.admin)
	d := f.create(t, "published")
	v := f.publish(t, d.Definition)
	_, err := f.s.SaveMetricDraft(ctx, actor, f.scope, d.Definition.ID, dm.SaveMetricInput{WriteMeta: wm(v.Definition.Revision), Name: "private-draft-name", Description: "private-draft-text", Rule: f.rule, BaseVersionID: v.Version.ID}, access.Meta{})
	must(t, err)
	tenant, err := tenancy.New(f.e.Service.Access()).Context(ctx, actor, f.scope.Tenant)
	must(t, err)
	project, err := projects.New(f.e.Service.Access()).Create(ctx, actor, f.scope.Tenant, projects.CreateInput{Fields: projects.Fields{Name: "项目", Code: "derived-project", Purpose: "派生监测"}, WriteMeta: projects.WriteMeta{RequestID: uuid.NewString(), Reason: "定义自验"}, Owner: tenant.MembershipID, Administrator: tenant.MembershipID}, access.Meta{})
	must(t, err)
	ps := dm.ProjectScope(f.scope.Tenant, project.ProjectID)
	page, err := f.s.Metrics(ctx, actor, ps, dm.MetricFilter{Query: "private-draft"})
	must(t, err)
	if len(page.Items) != 0 {
		t.Fatal("project search revealed draft")
	}
	page, err = f.s.Metrics(ctx, actor, ps, dm.MetricFilter{Query: "published"})
	must(t, err)
	if len(page.Items) != 1 || page.Items[0].Draft != nil || page.Items[0].Name != "published" || page.Items[0].Description != "" || page.Items[0].Revision != "0" {
		t.Fatal("project projection leaked tenant draft", page)
	}
	_, err = f.s.CreateMetric(ctx, actor, ps, dm.CreateMetricInput{WriteMeta: wm(""), Code: "bad", Name: "bad", Rule: f.rule}, access.Meta{})
	code(t, err, "permission_denied")
	_, err = f.s.Metric(ctx, f.actor(f.platform), f.scope, d.Definition.ID)
	code(t, err, "tenant_not_found")
	other := f.member(t, "OtherAdmin")
	ot, err := tenancy.New(f.e.Service.Access()).Create(ctx, f.actor(f.platform), tenancy.CreateInput{Fields: tenancy.Fields{Name: "其他", Code: "other"}, AdministratorID: other.Account.ID, RequestID: uuid.NewString(), Reason: "隔离验证"}, access.Meta{})
	must(t, err)
	_, err = f.s.Metric(ctx, f.actor(other), dm.TenantScope(ot.TenantID), d.Definition.ID)
	code(t, err, "not_found")
	_, err = f.s.MetricVersion(ctx, f.actor(other), dm.TenantScope(ot.TenantID), v.Version.ID)
	code(t, err, "not_found")
	_, err = f.s.Metric(ctx, actor, dm.ProjectScope(f.scope.Tenant, uuid.NewString()), d.Definition.ID)
	code(t, err, "project_not_found")
	var used bool
	must(t, f.e.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		var e error
		used, e = f.s.HasResourceReferences(ctx, tx, f.unit.VersionID)
		return e
	}))
	if !used {
		t.Fatal("public unit reference not protected")
	}
	_, err = f.engineering.SetState(ctx, f.actor(f.platform), eng.Scope{Kind: eng.UnitDefinition}, f.unit.DefinitionID, f.unit.VersionID, eng.StateInput{WriteMeta: ewm(), Revision: 1, Enabled: false}, access.Meta{})
	must(t, err)
	old, err := f.s.MetricVersion(ctx, actor, f.scope, v.Version.ID)
	must(t, err)
	if old.Usable || old.ContentHash != v.Version.ContentHash {
		t.Fatal("dependency state changed interpretation", old)
	}
	_, err = f.s.MetricVersion(ctx, actor, ps, v.Version.ID)
	code(t, err, "not_found")
	_, err = f.s.CreateMetric(ctx, actor, f.scope, dm.CreateMetricInput{WriteMeta: wm(""), Code: "disabled-dependency", Name: "停用依赖", Rule: f.rule}, access.Meta{})
	code(t, err, "dependency_unavailable")
	// Permanent evidence stays private even when platform audit permissions exist.
	events, err := f.e.Service.Access().AuditEvents(ctx, f.actor(f.platform), access.Filter{ObjectType: "derived_metric_definition"})
	must(t, err)
	if len(events.Items) != 0 {
		t.Fatal("platform audit exposed tenant derived definitions")
	}
	var count int
	must(t, f.e.Pool.QueryRow(ctx, `SELECT count(*) FROM derived_metric_versions WHERE tenant_id=$1 AND id=$2`, f.scope.Tenant, v.Version.ID).Scan(&count))
	if count != 1 {
		t.Fatal("disabled history was lost")
	}
}
