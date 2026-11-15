//go:build monitoringintegration

package resources_test

import (
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"testing"
)

func taskTenantRole(t *testing.T, f resourceFixture, actor access.Actor, codes []access.Code) (string, func()) {
	t.Helper()
	s := f.env.Service.Access()
	role, e := s.CreateTenantRole(ctx, f.actor, f.tenant, "", access.TenantRoleCreateInput{Name: "任务最小职责 " + uuid.NewString()[:8], Permissions: codes, RequestID: uuid.NewString(), Reason: "独立任务管理资格"}, access.Meta{})
	must(t, e)
	var member string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2 AND status='active'`, f.tenant, actor.ID).Scan(&member))
	set := func(ids []string) {
		t.Helper()
		a, e := s.TenantMemberAuthorization(ctx, f.actor, f.tenant, member)
		must(t, e)
		in := access.TenantChange{ExpectedMembershipVersion: a.MembershipVersion, RoleIDs: ids, Reason: "任务权限功能验证"}
		p, e := s.PreviewTenantChange(ctx, f.actor, f.tenant, member, access.OpTenantMemberRolesPreview, in, access.Filter{})
		must(t, e)
		in.PreviewHash = p.Hash
		in.RoleVersions = p.RoleVersions
		_, e = s.ChangeTenantAuthorization(ctx, f.actor, f.tenant, member, access.OpTenantMemberRoles, in, access.Meta{})
		must(t, e)
	}
	set([]string{role.Role.ID})
	return member, func() { set([]string{}) }
}
func TestTaskMonitoringManagerBackgroundQueryAndPagination(t *testing.T) {
	f := monitoringSetup(t)
	member := f.project(t, "TaskManager")
	_, revoke := taskTenantRole(t, f.resourceFixture, member.actor, []access.Code{access.TenantMonitoringJobsManage})
	batch := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,12,good\n")))
	query, e := f.service.Analytics(ctx, f.actor, f.scope, "overview", f.query(t))
	must(t, e)
	if query.JobID == "" || query.State != "pending" {
		t.Fatal("missing summary should queue a real exact query", query)
	}
	worked, e := f.service.RunTask(ctx, "task-query", f.tenant, query.JobID)
	must(t, e)
	if !worked {
		t.Fatal("query not executed")
	}
	j, e := f.service.Job(ctx, f.actor, f.scope, query.JobID)
	must(t, e)
	if j.Query == nil || j.Query.Action != "overview" || j.Query.Input.Snapshot != query.Snapshot.ID {
		t.Fatal("query target lost fixed snapshot", j.Query)
	}
	// The browser returns the public job's exact query through the strict HTTP
	// decoder, which rejects null optional values from internal storage JSON.
	wire, e := json.Marshal(j.Query)
	must(t, e)
	var wireQuery struct {
		Input json.RawMessage `json:"input"`
	}
	must(t, json.Unmarshal(wire, &wireQuery))
	var fixedQuery md.QueryInput
	must(t, md.DecodeRequest(wireQuery.Input, &fixedQuery))
	result, e := f.service.Analytics(ctx, f.actor, f.scope, j.Query.Action, fixedQuery)
	must(t, e)
	if result.State != "ready" || result.Snapshot.ID != query.Snapshot.ID || result.Overview.Samples != "1" {
		t.Fatal("query result changed fixed input", result)
	}
	if _, e = f.service.Job(ctx, member.actor, f.scope, query.JobID); e == nil {
		t.Fatal("manager read full query parameters")
	}
	service := runtime.ComposeTasks(f.env.Service.Access())
	index := runtime.ComposeTaskIndexer(f.env.Service.Access())
	_, e = index.Tick(ctx, "manager-test")
	must(t, e)
	page, e := service.List(ctx, member.actor, tasks.Filter{Mode: "managed", Limit: 1})
	must(t, e)
	if len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatal("authorized page not filled", page)
	}
	seen := map[string]bool{}
	cursor := ""
	for n := 0; n < 10; n++ {
		p, e := service.List(ctx, member.actor, tasks.Filter{Mode: "managed", Limit: 1, Cursor: cursor})
		must(t, e)
		for _, v := range p.Items {
			if seen[v.ID] {
				t.Fatal("duplicate across pages")
			}
			seen[v.ID] = true
			if v.ResultReadable || v.ResultState != "restricted" {
				t.Fatal("management leaked result access")
			}
		}
		cursor = p.NextCursor
		if cursor == "" {
			break
		}
	}
	if len(seen) != 3 {
		t.Fatalf("expected validate/adopt/query once, got %d", len(seen))
	}
	summary, e := service.Summary(ctx, member.actor, tasks.Filter{Mode: "managed"})
	must(t, e)
	if summary.CountsState != "exact" || *summary.Total != 3 {
		t.Fatal(summary)
	}
	source := taskrecord.SourceRef{Module: "monitoring", Kind: "job", RootID: query.JobID}
	target, e := service.Target(ctx, member.actor, source.ID(), "1", "result")
	must(t, e)
	if target.Available {
		t.Fatal("manager got query result")
	}
	if _, e = f.service.Job(ctx, member.actor, f.scope, query.JobID); e == nil {
		t.Fatal("manager gained old full job reader")
	}
	b := f.upload(t, "observed_at,value,quality\n2024-01-02T00:00:00Z,15,good\n")
	b, e = f.service.QueueImport(ctx, f.actor, f.scope, b.ID, "validate", f.meta(b.Revision))
	must(t, e)
	ref := b.TaskRef
	if ref == nil {
		t.Fatal("source response omitted task_ref")
	}
	detail, e := service.Get(ctx, member.actor, ref.TaskID, "")
	must(t, e)
	cmd := tasks.Command{RequestID: uuid.NewString(), Action: "cancel", ExpectedGeneration: detail.Generation, ExpectedRevision: detail.ControlRevision, Reason: "只有管理摘要资格时停止工作"}
	stopped, e := service.Command(ctx, member.actor, ref.TaskID, cmd)
	must(t, e)
	if stopped.State != "cancelled" {
		t.Fatal(stopped)
	}
	again, e := service.Command(ctx, member.actor, ref.TaskID, cmd)
	must(t, e)
	if !again.Replayed {
		t.Fatal("cancel receipt not replayed")
	}
	revoke()
	if _, e = service.List(ctx, member.actor, tasks.Filter{Mode: "managed", Limit: 1, Cursor: page.NextCursor}); e == nil {
		t.Fatal("revoked cursor survived")
	}
	if _, e = service.Get(ctx, member.actor, ref.TaskID, ""); !tasks.Denied(e) {
		t.Fatalf("revoked summary: %v", e)
	}
	// A successful task remains readable after the original download expires.
	out, e := f.service.CreateExport(ctx, f.actor, f.scope, md.ExportInput{WriteMeta: f.meta("0"), Query: f.query(t), Kind: "details", Format: "csv", CSVMode: "exact"})
	must(t, e)
	_, e = f.service.RunTask(ctx, "task-export", f.tenant, out.JobID)
	must(t, e)
	// Simulate the existing seven-day deadline only in this isolated schema.
	must(t, pgx.BeginFunc(ctx, f.env.Pool, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `ALTER TABLE monitoring_exports DISABLE TRIGGER monitoring_export_source_guard`); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE monitoring_exports SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, out.ID); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `ALTER TABLE monitoring_exports ENABLE TRIGGER monitoring_export_source_guard`)
		return e
	}))
	expired, e := service.Get(ctx, f.actor, out.TaskRef.TaskID, "")
	must(t, e)
	if expired.State != "succeeded" || expired.ResultState != "expired" {
		t.Fatal("expiry overwrote execution", expired)
	}
	_ = batch
}
