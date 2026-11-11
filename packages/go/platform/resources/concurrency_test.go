package resources_test

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
)

func TestConcurrentAuthorizationChangesPreventResourceCommit(t *testing.T) {
	for _, change := range []string{"permission", "membership", "tenant"} {
		t.Run(change, func(t *testing.T) {
			f := resourcesSetup(t)
			facility := f.facility(t, "F")
			actor, role, member := f.reader(t)
			_, e := f.env.Pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_code,binding_scope) VALUES($1,$2,'tenant')`, role, access.TenantEngineeringResourceManage)
			must(t, e)
			target := res.Target{Kind: res.Facility, ID: facility.ID}
			requestID := f.wm(t).RequestID
			raw, _ := json.Marshal(map[string]any{"request_id": requestID, "expected_revision": 1, "name": "失权后不得提交"})
			tx, e := f.env.Pool.Begin(ctx)
			must(t, e)
			defer tx.Rollback(ctx)
			_, e = access.Lock(ctx, tx, true)
			must(t, e)
			want := "permission_denied"
			switch change {
			case "permission":
				_, e = tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_code=$2`, role, access.TenantEngineeringResourceManage)
			case "membership":
				_, e = tx.Exec(ctx, `UPDATE tenant_member_roles SET revoked_at=now(),revoked_by=$2 WHERE membership_id=$1 AND revoked_at IS NULL`, member, f.actor.ID)
				must(t, e)
				_, e = tx.Exec(ctx, `UPDATE tenant_memberships SET status='ended',ended_at=now(),ended_by=$2,end_reason='测试结束关系' WHERE id=$1`, member, f.actor.ID)
				want = "tenant_not_found"
			case "tenant":
				_, e = tx.Exec(ctx, `UPDATE tenants SET status='disabled',status_changed_at=now(),status_changed_by=$2,status_change_reason='测试停用',status_notice='测试停用' WHERE id=$1`, f.tenant, f.actor.ID)
				want = "tenant_disabled"
			}
			must(t, e)
			done := make(chan error, 1)
			go func() {
				_, err := f.srv.Mutate(ctx, actor, res.TenantScope(f.tenant), target, "update", raw, access.Meta{})
				done <- err
			}()
			select {
			case e := <-done:
				t.Fatalf("write bypassed authorization lock: %v", e)
			case <-time.After(30 * time.Millisecond):
			}
			must(t, tx.Commit(ctx))
			code(t, <-done, want)
			var revision, history, receipts int
			must(t, f.env.Pool.QueryRow(ctx, `SELECT revision,(SELECT count(*) FROM engineering_resource_changes WHERE facility_id=$1),(SELECT count(*) FROM engineering_resource_receipts WHERE request_id=$2) FROM engineering_facilities WHERE id=$1`, facility.ID, requestID).Scan(&revision, &history, &receipts))
			if revision != 1 || history != 1 || receipts != 0 {
				t.Fatalf("denied write left data: revision=%d history=%d receipts=%d", revision, history, receipts)
			}
		})
	}
}

func TestConcurrentIdenticalCreateAndDeleteAgainstChildCreate(t *testing.T) {
	f := resourcesSetup(t)
	requestID := f.wm(t).RequestID
	raw, _ := json.Marshal(map[string]any{"request_id": requestID, "code": "ONCE", "name": "同一次创建", "type_version_id": f.typ, "values": map[string]any{"length": 0}})
	target := res.Target{Kind: res.Facility}
	var wg sync.WaitGroup
	results := make(chan res.Result, 2)
	errors := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			r, e := f.srv.Mutate(ctx, f.actor, res.TenantScope(f.tenant), target, "create", raw, access.Meta{})
			results <- r
			errors <- e
		})
	}
	wg.Wait()
	close(results)
	close(errors)
	for e := range errors {
		must(t, e)
	}
	var facility res.Result
	replayed := 0
	for r := range results {
		if facility.ID != "" && facility.ID != r.ID {
			t.Fatal("same request created multiple identities")
		}
		facility = r
		if r.Replayed {
			replayed++
		}
	}
	if replayed != 1 {
		t.Fatalf("expected one replay, got %d", replayed)
	}
	var count int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM engineering_resource_changes WHERE facility_id=$1`, facility.ID).Scan(&count))
	if count != 1 {
		t.Fatal("duplicate creation history", count)
	}
	// Use two distinct accounts so the facility lock, rather than the account
	// lock used by receipt serialization, protects this structural race.
	actor, role, _ := f.reader(t)
	_, e := f.env.Pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_code,binding_scope) VALUES($1,$2,'tenant')`, role, access.TenantEngineeringResourceManage)
	must(t, e)
	deleteRaw, _ := json.Marshal(map[string]any{"request_id": f.wm(t).RequestID, "expected_revision": 1, "reason": "误建设施"})
	childRaw, _ := json.Marshal(map[string]any{"request_id": f.wm(t).RequestID, "code": "P", "name": "新增测点"})
	errors = make(chan error, 2)
	wg.Go(func() {
		_, err := f.srv.Mutate(ctx, f.actor, res.TenantScope(f.tenant), res.Target{Kind: res.Facility, ID: facility.ID}, "delete", deleteRaw, access.Meta{})
		errors <- err
	})
	wg.Go(func() {
		_, err := f.srv.Mutate(ctx, actor, res.TenantScope(f.tenant), res.Target{Kind: res.Point, Facility: facility.ID}, "create", childRaw, access.Meta{})
		errors <- err
	})
	wg.Wait()
	close(errors)
	success := 0
	for e := range errors {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("delete/create must have one winner, got %d", success)
	}
	var deleted bool
	must(t, f.env.Pool.QueryRow(ctx, `SELECT deleted_at IS NOT NULL,(SELECT count(*) FROM engineering_points WHERE facility_id=$1 AND deleted_at IS NULL) FROM engineering_facilities WHERE id=$1`, facility.ID).Scan(&deleted, &count))
	if deleted && count != 0 || !deleted && count != 1 {
		t.Fatal("orphan or lost child", deleted, count)
	}
}
