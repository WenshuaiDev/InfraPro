package resources_test

import (
	"fmt"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestDisplayNotificationLateCommitAndFailedReplay(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "NotificationBridge")
	s := runtime.Compose(f.env.Service.Access()).Displays
	scope := display.Scope{Tenant: f.tenant, Project: p.id}
	meta := display.Meta{Revision: "0", RequestID: uuid.NewString(), Reason: "notification bridge regression"}
	_, e := s.Command(ctx, p.actor, scope, "", "settings", "", display.Command{Meta: meta, Enabled: true})
	must(t, e)
	meta.RequestID = uuid.NewString()
	v, e := s.Command(ctx, p.actor, scope, "", "create", "", display.Command{Meta: meta, Name: "Bridge source"})
	must(t, e)
	instance := v["id"].(string)
	// Insert source records in controlled transactions to reproduce a late
	// sequence ID and a corrupt bridge record without timing-dependent sleeps.
	appendFact := func(tx pgx.Tx, outcome string) (int64, string) {
		t.Helper()
		var event int64
		must(t, tx.QueryRow(ctx, `INSERT INTO display_events(tenant_id,project_id,instance_id,kind,details) VALUES($1,$2,$3,'instance','{"notify_instance":true}') RETURNING id`, f.tenant, p.id, instance).Scan(&event))
		var handoff string
		must(t, tx.QueryRow(ctx, `INSERT INTO display_notification_handoffs(event_id,tenant_id,project_id,instance_id,kind,target_id,outcome,recipients) VALUES($1,$2,$3,$4,'management',$4,$5,ARRAY[$6::uuid]) RETURNING id::text`, event, f.tenant, p.id, instance, outcome, p.actor.ID).Scan(&handoff))
		return event, handoff
	}
	late, e := f.env.Pool.Begin(ctx)
	must(t, e)
	defer late.Rollback(ctx)
	low, _ := appendFact(late, "instance")
	highTx, e := f.env.Pool.Begin(ctx)
	must(t, e)
	high, _ := appendFact(highTx, "instance")
	bad, badHandoff := appendFact(highTx, "invalid_outcome")
	_, e = highTx.Exec(ctx, `UPDATE display_notification_handoffs SET attempts=7 WHERE id=$1`, badHandoff)
	must(t, e)
	must(t, highTx.Commit(ctx))
	if low >= high {
		t.Fatal("fixture did not allocate smaller uncommitted sequence")
	}
	inbox := runtime.ComposeNotifications(f.env.Service.Access())
	_, e = inbox.Tick(ctx)
	must(t, e)
	count := func(source int64) int {
		t.Helper()
		var n int
		must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_events e JOIN account_notifications n ON n.event_id=e.id WHERE e.source_module='display' AND e.source_key=$1`, fmt.Sprintf("%d:display.changed", source)).Scan(&n))
		return n
	}
	if count(high) != 1 || count(low) != 0 || count(bad) != 0 {
		t.Fatal("uncommitted or invalid source published, or valid source blocked")
	}
	diagnostic, e := runtime.DisplayNotificationDiagnostics(ctx, f.env.Service.Access())
	must(t, e)
	if diagnostic["failed"] != int64(1) {
		t.Fatal("poison handoff not diagnosed", diagnostic)
	}
	must(t, late.Commit(ctx))
	_, e = f.env.Pool.Exec(ctx, `UPDATE display_notification_handoffs SET outcome='instance' WHERE id=$1`, badHandoff)
	must(t, e)
	must(t, runtime.ReplayDisplayNotification(ctx, f.env.Service.Access(), badHandoff, "repaired synthetic malformed source"))
	for i := 0; i < 2; i++ {
		_, e = inbox.Tick(ctx)
		must(t, e)
	}
	if count(low) != 1 || count(high) != 1 || count(bad) != 1 {
		t.Fatal("late commit or repaired handoff missing/duplicated")
	}
}
