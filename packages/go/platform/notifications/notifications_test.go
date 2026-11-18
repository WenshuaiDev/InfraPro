package notifications_test

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/notifications"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ctx = context.Background()

func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}

type fixture struct {
	env  *testsupport.Environment
	s    *notifications.Service
	a, b access.Actor
	p, q identity.Principal
}

func setup(t *testing.T) fixture {
	t.Helper()
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run pnpm test:notifications")
	}
	env, e := testsupport.New(ctx, slog.Default())
	must(t, e)
	t.Cleanup(env.Close)
	password := "Notify river 桥梁 2026!"
	_, e = env.Service.Bootstrap(ctx, "NotifyAdmin", "通知管理", password, identity.Meta{})
	must(t, e)
	login := func(name string) identity.Principal {
		_, token, _, e := env.Service.Login(ctx, name, password, identity.Meta{})
		must(t, e)
		p, e := env.Service.Authenticate(ctx, token)
		must(t, e)
		return p
	}
	p := login("NotifyAdmin")
	invite, e := env.Service.CreateInvitation(ctx, p, "", "", identity.Meta{})
	must(t, e)
	u, e := url.Parse(invite.URL)
	must(t, e)
	values, _ := url.ParseQuery(u.Fragment)
	must(t, env.Service.AcceptInvitation(ctx, values.Get("token"), "NotifyUser", "通知成员", password, identity.Meta{}))
	q := login("NotifyUser")
	return fixture{env, runtime.ComposeNotifications(env.Service.Access()), env.Service.AccessActor(p), env.Service.AccessActor(q), p, q}
}
func event(account string) ne.Event {
	return ne.Event{Module: "notification_test", Key: uuid.NewString(), Type: "security.changed", Scope: ne.Scope{Kind: "account"}, Target: ne.Target{Kind: "security"}, Outcome: "password_changed", Recipients: ne.Personal(account)}
}
func (f fixture) emit(t *testing.T, v ne.Event) string {
	t.Helper()
	var id string
	must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error { var e error; id, e = ne.AppendTx(ctx, tx, v); return e }))
	return id
}
func (f fixture) drain(t *testing.T) {
	t.Helper()
	for i := 0; i < 10; i++ {
		n, e := f.s.Tick(ctx)
		must(t, e)
		if n == 0 {
			return
		}
	}
	t.Fatal("queue did not drain")
}
func TestTransactionalDeliveryWatermarkIsolationAndPreferences(t *testing.T) {
	f := setup(t)
	v := event(f.a.ID)
	rollback := errors.New("abort business")
	e := f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		_, e := ne.AppendTx(ctx, tx, v)
		if e != nil {
			return e
		}
		return rollback
	})
	if !errors.Is(e, rollback) {
		t.Fatal(e)
	}
	var n int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_events WHERE source_key=$1`, v.Key).Scan(&n))
	if n != 0 {
		t.Fatal("rolled back fact survived")
	}
	id := f.emit(t, v)
	if got := f.emit(t, v); id != got {
		t.Fatal("duplicate source identity")
	}
	conflict := v
	conflict.Outcome = "password_reset"
	e = f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error { _, e := ne.AppendTx(ctx, tx, conflict); return e })
	if !errors.Is(e, ne.ErrConflict) {
		t.Fatalf("want conflict, got %v", e)
	}
	// Concurrent workers race on the same durable queue.
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Go(func() { _, e := f.s.Tick(ctx); errs <- e })
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		must(t, e)
	}
	page, e := f.s.List(ctx, f.a, notifications.Filter{Category: "security"})
	must(t, e)
	if len(page.Items) != 1 || page.Items[0].ReadAt != nil {
		t.Fatalf("unexpected inbox %#v", page)
	}
	index, e := f.s.List(ctx, f.a, notifications.Filter{Category: "security", PageIndexOnly: true, Limit: 1})
	must(t, e)
	if index.Total == nil || *index.Total != len(page.Items) || len(index.Items) != 0 || len(index.PageCursors) != 0 {
		t.Fatal("incorrect personal notification index")
	}
	item := page.Items[0]
	if _, e = f.s.Get(ctx, f.b, item.ID); e != access.ErrNotFound {
		t.Fatalf("cross-account read %v", e)
	}
	if _, e = f.s.Target(ctx, f.b, item.ID); e != access.ErrNotFound {
		t.Fatal("target leaked")
	}
	fresh := event(f.a.ID)
	fresh.OccurredAt = time.Now().Add(-time.Hour)
	f.emit(t, fresh)
	f.drain(t)
	changed, e := f.s.ReadAll(ctx, f.a, notifications.ReadAllInput{Filter: notifications.Filter{Category: "security"}, Watermark: page.Watermark})
	must(t, e)
	if changed != 1 {
		t.Fatal(changed)
	}
	summary, e := f.s.Summary(ctx, f.a)
	must(t, e)
	if summary.Categories["security"] != 1 {
		t.Fatal("late old fact incorrectly read")
	}
	yes, no := true, false
	item, e = f.s.Get(ctx, f.a, item.ID)
	must(t, e)
	archived, e := f.s.Change(ctx, f.a, item.ID, notifications.Change{ExpectedRevision: item.Revision, Archived: &yes})
	must(t, e)
	if archived.ArchivedAt == nil || archived.ReadAt == nil {
		t.Fatal("archive semantics")
	}
	_, e = f.s.Change(ctx, f.a, item.ID, notifications.Change{ExpectedRevision: item.Revision, Read: &no})
	if e != access.ErrConflict {
		t.Fatal("stale state did not conflict")
	}
	item, e = f.s.Change(ctx, f.a, item.ID, notifications.Change{ExpectedRevision: archived.Revision, Read: &no})
	must(t, e)
	if item.ArchivedAt != nil || item.ReadAt != nil {
		t.Fatal("restore unread semantics")
	}
	_, e = f.s.SavePreferences(ctx, f.a, notifications.Preferences{Tasks: false, Updates: false, Revision: "0"})
	must(t, e)
	optional := event(f.a.ID)
	optional.Type = "task.result"
	f.emit(t, optional)
	f.emit(t, event(f.a.ID))
	f.drain(t)
	var suppressed int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_deliveries WHERE account_id=$1 AND reason_code='preference_disabled'`, f.a.ID).Scan(&suppressed))
	if suppressed != 1 {
		t.Fatal("preference not persisted")
	}
	_, e = f.s.SavePreferences(ctx, f.a, notifications.Preferences{Tasks: true, Updates: true, Revision: "1"})
	must(t, e)
	f.drain(t)
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM account_notifications n JOIN notification_events e ON e.id=n.event_id WHERE e.source_key=$1`, optional.Key).Scan(&n))
	if n != 0 {
		t.Fatal("reenable backfilled suppression")
	}
	subject := (&access.ReadSubject{ID: f.a.ID, Check: func(context.Context, pgx.Tx) error { return nil }}).ReadActor()
	if _, e = f.s.Summary(ctx, subject); e != access.ErrDenied {
		t.Fatal("ReadSubject entered personal inbox")
	}
}
func TestPersonalInvitationAndRevocationProjection(t *testing.T) {
	f := setup(t)
	ts := tenancy.New(f.env.Service.Access())
	tenant, e := ts.Create(ctx, f.a, tenancy.CreateInput{Fields: tenancy.Fields{Name: "通知测试租户", Code: "notify-test"}, AdministratorID: f.a.ID, RequestID: uuid.NewString(), Reason: "通知合成验收"}, access.Meta{})
	must(t, e)
	invite, e := ts.CreateInvitation(ctx, f.a, tenant.TenantID, tenancy.InvitationCreateInput{TargetID: f.b.ID, RequestID: uuid.NewString(), Reason: "通知合成邀请"}, access.Meta{})
	must(t, e)
	f.drain(t)
	page, e := f.s.List(ctx, f.b, notifications.Filter{})
	must(t, e)
	if len(page.Items) != 1 || !page.Items[0].TargetAvailable {
		t.Fatalf("personal invitation missing: %+v", page)
	}
	if _, e = ts.PersonalInvitation(ctx, f.a, invite.Invitation.ID, access.Meta{}); e == nil {
		t.Fatal("invitation recipient isolation")
	}
	detail, e := ts.PersonalInvitation(ctx, f.b, invite.Invitation.ID, access.Meta{})
	must(t, e)
	if !detail.CanAccept {
		t.Fatal("invitation not actionable")
	}
	item, e := f.s.Get(ctx, f.b, page.Items[0].ID)
	must(t, e)
	yes := true
	_, e = f.s.Change(ctx, f.b, item.ID, notifications.Change{ExpectedRevision: item.Revision, Read: &yes})
	must(t, e)
	var count int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2`, tenant.TenantID, f.b.ID).Scan(&count))
	if count != 0 {
		t.Fatal("read accepted invitation")
	}
	accepted, e := ts.AcceptPersonalInvitation(ctx, f.b, invite.Invitation.ID, detail.Version, access.Meta{})
	must(t, e)
	f.drain(t)
	// After removal the same receipt is still manageable, but source and names vanish.
	_, e = ts.ChangeMember(ctx, f.a, tenant.TenantID, accepted.MembershipID, access.OpTenantMemberRemove, tenancy.MemberInput{Version: accepted.Version, Reason: "合成撤权验证"}, access.Meta{})
	must(t, e)
	item, e = f.s.Get(ctx, f.b, item.ID)
	must(t, e)
	if !item.Redacted || item.Scope.Tenant != "" || item.ObjectName != "" || item.TargetAvailable {
		t.Fatalf("restricted context leaked: %+v", item)
	}
	filtered, e := f.s.List(ctx, f.b, notifications.Filter{Tenant: tenant.TenantID})
	must(t, e)
	index, e := f.s.List(ctx, f.b, notifications.Filter{Tenant: tenant.TenantID, PageIndexOnly: true})
	must(t, e)
	if index.Total == nil || *index.Total != 0 || len(index.PageCursors) != 0 {
		t.Fatal("scoped index counted revoked receipts")
	}
	if len(filtered.Items) != 0 {
		t.Fatal("scope filter disclosed revoked receipt")
	}
}

func TestLateCommitBatchedSnapshotsConditionCyclesAndSourceFailure(t *testing.T) {
	f := setup(t)
	options := notifications.DefaultOptions()
	options.Batch = 1
	_, e := f.s.WithOptions(options)
	must(t, e)
	late := event(f.a.ID)
	tx, e := f.env.Pool.Begin(ctx)
	must(t, e)
	defer tx.Rollback(ctx)
	_, e = ne.AppendTx(ctx, tx, late)
	must(t, e)
	early := event(f.a.ID)
	early.Recipients = ne.Personal(f.a.ID, f.b.ID)
	f.emit(t, early)
	f.drain(t)
	var before string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT sequence::text FROM notification_inbox_states WHERE account_id=$1`, f.a.ID).Scan(&before))
	must(t, tx.Commit(ctx))
	f.drain(t)
	after, e := f.s.List(ctx, f.a, notifications.Filter{Category: "security"})
	must(t, e)
	if len(after.Items) != 2 || after.Items[0].Sequence == before {
		t.Fatal("late committed candidate was skipped", after)
	}
	index, e := f.s.List(ctx, f.a, notifications.Filter{Category: "security", Limit: 1, PageIndexOnly: true})
	must(t, e)
	if index.Total == nil || *index.Total != 2 || len(index.PageCursors) != 1 {
		t.Fatal("expected a cursor for the second page")
	}
	last, e := f.s.List(ctx, f.a, notifications.Filter{Category: "security", Limit: 1, Cursor: index.PageCursors[0]})
	must(t, e)
	if len(last.Items) != 1 || last.Items[0].ID != after.Items[1].ID || last.NextCursor != "" {
		t.Fatal("index cursor did not reopen the last page")
	}
	var recipients, expanded int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*),count(expanded_at) FROM notification_recipients r JOIN notification_events e ON e.id=r.event_id WHERE e.source_key=$1`, early.Key).Scan(&recipients, &expanded))
	if recipients != 2 || expanded != 2 {
		t.Fatal(recipients, expanded)
	}
	condition := event(f.a.ID)
	condition.Type = "derived.condition"
	condition.Outcome = "failed"
	condition.Module = "cycle_test"
	for _, value := range []bool{true, true, false, false} {
		must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error { return ne.ConditionTx(ctx, tx, "generation-1:source_missing", value, condition) }))
	}
	var events, occurrences int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_events WHERE source_module='cycle_test'`).Scan(&events))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT occurrences FROM notification_conditions WHERE source_module='cycle_test'`).Scan(&occurrences))
	if events != 2 || occurrences != 2 {
		t.Fatal("continuous condition spam or missing recovery", events, occurrences)
	}
	must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error { return ne.ConditionTx(ctx, tx, "generation-2:source_missing", false, condition) }))
	// A producer incompatibility rolls back the actual tenant creation, including
	// its administrator, instead of allowing a success without a durable receipt.
	_, e = f.env.Pool.Exec(ctx, `UPDATE notification_protocol SET version=99`)
	must(t, e)
	ts := tenancy.New(f.env.Service.Access())
	_, e = ts.Create(ctx, f.a, tenancy.CreateInput{Fields: tenancy.Fields{Name: "应回滚租户", Code: "must-rollback"}, AdministratorID: f.a.ID, RequestID: uuid.NewString(), Reason: "通知事实异常验证"}, access.Meta{})
	if e == nil {
		t.Fatal("source failure did not roll back business")
	}
	var n int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM tenants WHERE code='must-rollback'`).Scan(&n))
	if n != 0 {
		t.Fatal("business committed without notification")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE notification_protocol SET version=1`)
	must(t, e)
}

func TestExpiryMaintenancePersistsOnceWithoutOpeningInvitation(t *testing.T) {
	f := setup(t)
	ts := tenancy.New(f.env.Service.Access())
	tenant, e := ts.Create(ctx, f.a, tenancy.CreateInput{Fields: tenancy.Fields{Name: "邀请期限租户", Code: "expiry"}, AdministratorID: f.a.ID, RequestID: uuid.NewString(), Reason: "合成邀请期限验证"}, access.Meta{})
	must(t, e)
	invite, e := ts.CreateInvitation(ctx, f.a, tenant.TenantID, tenancy.InvitationCreateInput{TargetID: f.b.ID, RequestID: uuid.NewString(), Reason: "合成过期邀请"}, access.Meta{})
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `UPDATE tenant_invitations SET expires_at=clock_timestamp()-interval '1 second',created_at=clock_timestamp()-interval '2 days' WHERE id=$1`, invite.Invitation.ID)
	must(t, e)
	f.drain(t)
	f.drain(t)
	detail, e := ts.PersonalInvitation(ctx, f.b, invite.Invitation.ID, access.Meta{})
	must(t, e)
	if detail.Status != "expired" || detail.CanAccept {
		t.Fatal(detail)
	}
	var n int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_events WHERE target_id=$1 AND outcome='expired'`, invite.Invitation.ID).Scan(&n))
	if n != 1 {
		t.Fatal("expiry duplicate or missing", n)
	}
	if _, e = ts.AcceptPersonalInvitation(ctx, f.b, invite.Invitation.ID, detail.Version, access.Meta{}); e == nil {
		t.Fatal("expired invitation accepted")
	}
}
