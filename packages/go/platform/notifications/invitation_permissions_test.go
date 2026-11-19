package notifications_test

import (
	"crypto/rand"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/notifications"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This uses the release DCL and a TCP login which can inherit only control-api.
// Producers and the delivery worker use their normal domain methods to create
// the facts. Every inbox read and disclosure assertion uses the issued login.
func TestRestrictedPersonalInvitationReceipts(t *testing.T) {
	f := setup(t)
	root := os.Getenv("INFRAPRO_TEST_REPO_ROOT")
	if root == "" {
		root = "../../../.."
	}
	for _, file := range []string{
		"deploy/architecture/restore-base-grants.sql", "deploy/display/package-worker-role.sql",
		"deploy/architecture/postgres-roles.sql", "deploy/events/postgres-grants.sql",
	} {
		body, err := os.ReadFile(filepath.Join(root, file))
		must(t, err)
		_, err = f.env.Pool.Exec(ctx, string(body), pgx.QueryExecModeSimpleProtocol)
		must(t, err)
	}
	login := "arc_notify_test_" + strings.ToLower(rand.Text())
	password := rand.Text()
	_, err := f.env.Pool.Exec(ctx, "CREATE ROLE "+pgx.Identifier{login}.Sanitize()+" LOGIN CONNECTION LIMIT 2 PASSWORD '"+password+"' IN ROLE infrapro_control_api")
	must(t, err)
	t.Cleanup(func() {
		_, err := f.env.Pool.Exec(ctx, "DROP ROLE "+pgx.Identifier{login}.Sanitize())
		must(t, err)
	})
	address, err := url.Parse(f.env.Config.DatabaseURL)
	must(t, err)
	address.User = url.UserPassword(login, password)
	pool, err := pgxpool.New(ctx, address.String())
	must(t, err)
	t.Cleanup(pool.Close)
	var authentic bool
	must(t, pool.QueryRow(ctx, "SELECT current_user::text=$1 AND NOT rolsuper AND NOT rolbypassrls FROM pg_roles WHERE rolname=current_user", login).Scan(&authentic))
	if !authentic {
		t.Fatal("expected a restricted control-api login")
	}
	control := runtime.ComposeNotifications(access.New(pool, slog.Default()))
	const signature = "architecture_notification_invitation_targets(uuid,uuid,uuid[])"
	t.Run("fixed search path and private data", func(t *testing.T) {
		var pathOK, private bool
		must(t, pool.QueryRow(ctx, `SELECT p.prosecdef AND p.proconfig=ARRAY[format('search_path=pg_catalog, %I, pg_temp',current_schema())],
			NOT EXISTS(SELECT FROM aclexplode(coalesce(p.proacl,acldefault('f',p.proowner))) a
				WHERE a.privilege_type='EXECUTE' AND a.grantee<>p.proowner AND a.grantee<>'infrapro_control_api'::regrole)
			AND has_function_privilege(current_user,p.oid,'EXECUTE')
			AND NOT has_function_privilege('infrapro_query_api',p.oid,'EXECUTE')
			AND NOT has_function_privilege('infrapro_notification_worker',p.oid,'EXECUTE')
			FROM pg_proc p WHERE p.oid=to_regprocedure($1)`, signature).Scan(&pathOK, &private))
		if !pathOK || !private {
			t.Fatal("unexpected notification reader path or ACL", pathOK, private)
		}
		for _, statement := range []string{
			"SELECT token_hash FROM tenant_invitations LIMIT 0",
			"SELECT accepted_context_snapshot FROM tenant_invitations LIMIT 0",
			"SELECT close_reason FROM tenant_invitations LIMIT 0",
			"SELECT target_account_id FROM tenant_invitations LIMIT 0",
			"UPDATE tenant_invitations SET status='revoked' WHERE false",
			"UPDATE tenant_invitations SET accepted_context_snapshot='{}' WHERE false",
		} {
			_, err := pool.Exec(ctx, statement)
			var denied *pgconn.PgError
			if !errors.As(err, &denied) || denied.Code != "42501" {
				t.Fatalf("%s: expected SQLSTATE 42501, got %v", statement, err)
			}
		}
	})
	ts := tenancy.New(f.env.Service.Access())
	const original = "通知时的租户名称"
	tenant, err := ts.Create(ctx, f.a, tenancy.CreateInput{Fields: tenancy.Fields{Name: original, Code: "notify-scoped"}, AdministratorID: f.a.ID, RequestID: uuid.NewString(), Reason: "验证通知披露资格"}, access.Meta{})
	must(t, err)
	created, err := ts.CreateInvitation(ctx, f.a, tenant.TenantID, tenancy.InvitationCreateInput{TargetID: f.b.ID, RequestID: uuid.NewString(), Reason: "验证受邀人披露"}, access.Meta{})
	must(t, err)
	f.drain(t)
	var initialEvent string
	must(t, f.env.Pool.QueryRow(ctx, "SELECT id::text FROM notification_events WHERE target_kind='tenant_invitation' AND target_id=$1", created.Invitation.ID).Scan(&initialEvent))
	count := func(account, tenant, event string) int {
		t.Helper()
		var found int
		must(t, pool.QueryRow(ctx, "SELECT count(*) FROM architecture_notification_invitation_targets($1,$2,ARRAY[$3::uuid])", account, tenant, event).Scan(&found))
		return found
	}
	page, err := control.List(ctx, f.b, notifications.Filter{})
	must(t, err)
	if len(page.Items) != 1 {
		t.Fatalf("recipient got %d receipts", len(page.Items))
	}
	itemID := page.Items[0].ID
	t.Run("pending recipient and event scope", func(t *testing.T) {
		summary, err := control.Summary(ctx, f.b)
		must(t, err)
		if summary.Unread != 1 || summary.Categories["organization"] != 1 || !page.Items[0].TargetAvailable || page.Items[0].ObjectName != original || page.Items[0].CurrentState != "pending" {
			t.Fatal("pending invitation missing from restricted inbox")
		}
		if count(f.b.ID, tenant.TenantID, initialEvent) != 1 || count(f.a.ID, tenant.TenantID, initialEvent) != 0 || count(uuid.NewString(), tenant.TenantID, initialEvent) != 0 || count(f.b.ID, uuid.NewString(), initialEvent) != 0 || count(f.b.ID, tenant.TenantID, created.Invitation.ID) != 0 {
			t.Fatal("personal event, account or tenant boundary was not enforced")
		}
	})
	t.Run("expired receipt before maintenance", func(t *testing.T) {
		tx, err := f.env.Pool.Begin(ctx)
		must(t, err)
		defer tx.Rollback(ctx)
		// Move only this isolated row's clock, then use the production login's
		// permissions in the same transaction before maintenance has run.
		_, err = tx.Exec(ctx, "UPDATE tenant_invitations SET created_at=now()-interval '2 days',expires_at=now()-interval '1 second' WHERE id=$1", created.Invitation.ID)
		must(t, err)
		_, err = tx.Exec(ctx, "SET LOCAL ROLE "+pgx.Identifier{login}.Sanitize())
		must(t, err)
		resolved, err := tenancy.NotificationInvitationsTx(ctx, tx, f.b.ID, tenant.TenantID, []string{initialEvent})
		must(t, err)
		item := resolved[created.Invitation.ID]
		if item.Allowed || item.Available || item.Name != "" || item.State != "expired" || item.Reason != "invitation_expired" {
			t.Fatal("expired personal notification still disclosed its invitation", item)
		}
	})
	accepted, err := ts.AcceptPersonalInvitation(ctx, f.b, created.Invitation.ID, created.Invitation.Version, access.Meta{})
	must(t, err)
	f.drain(t)
	renamed := "接受之后的新名称"
	_, err = ts.Update(ctx, f.a, tenant.TenantID, tenancy.UpdateInput{Name: &renamed, Version: tenant.Version, Reason: "测试接受时的历史名称"}, false, access.Meta{})
	must(t, err)
	t.Run("accepted recipient and sender", func(t *testing.T) {
		item, err := control.Get(ctx, f.b, itemID)
		must(t, err)
		if item.Redacted || !item.TargetAvailable || item.CurrentState != "accepted" || item.ObjectName != original {
			t.Fatalf("accepted recipient did not receive historical name from own receipt: %+v", item)
		}
		inbox, err := control.List(ctx, f.a, notifications.Filter{})
		must(t, err)
		for _, sender := range inbox.Items {
			if sender.Type == "invitation.changed" && sender.Outcome == "accepted" {
				if sender.Redacted || sender.TargetAvailable || sender.ObjectName != "" || sender.UnavailableReason != "invitation_accepted" {
					t.Fatal("sender received more than the accepted status")
				}
				return
			}
		}
		t.Fatal("sender never received the accepted status")
	})
	t.Run("disabled tenant hides the accepted name", func(t *testing.T) {
		disable, err := ts.PreviewStatus(ctx, f.a, tenant.TenantID, "disabled")
		must(t, err)
		notice := "暂时停用以核对通知权限"
		_, err = ts.ChangeStatus(ctx, f.a, tenant.TenantID, "disabled", tenancy.StatusInput{Version: disable.Version, Hash: disable.Hash, Reason: notice, Notice: &notice}, access.Meta{})
		must(t, err)
		item, err := control.Get(ctx, f.b, itemID)
		must(t, err)
		var hidden bool
		must(t, pool.QueryRow(ctx, "SELECT accepted_tenant_name IS NULL FROM architecture_notification_invitation_targets($1,$2,ARRAY[$3::uuid])", f.b.ID, tenant.TenantID, initialEvent).Scan(&hidden))
		if !hidden || !item.Redacted || item.TargetAvailable || item.ObjectName != "" || item.Scope.Tenant != "" {
			t.Fatal("disabled tenant exposed its historical invitation name")
		}
		enable, err := ts.PreviewStatus(ctx, f.a, tenant.TenantID, "active")
		must(t, err)
		_, err = ts.ChangeStatus(ctx, f.a, tenant.TenantID, "active", tenancy.StatusInput{Version: enable.Version, Hash: enable.Hash, Reason: "恢复租户继续核对成员撤权"}, access.Meta{})
		must(t, err)
	})
	_, err = ts.ChangeMember(ctx, f.a, tenant.TenantID, accepted.MembershipID, access.OpTenantMemberRemove, tenancy.MemberInput{Version: accepted.Version, Reason: "验证撤回后的旧通知"}, access.Meta{})
	must(t, err)
	t.Run("revoked recipients cannot read historical names", func(t *testing.T) {
		item, err := control.Get(ctx, f.b, itemID)
		must(t, err)
		if !item.Redacted || item.ObjectName != "" || item.Scope.Tenant != "" || item.TargetAvailable {
			t.Fatal("revoked recipient retained invitation context")
		}
		var hidden bool
		must(t, pool.QueryRow(ctx, "SELECT accepted_tenant_name IS NULL FROM architecture_notification_invitation_targets($1,$2,ARRAY[$3::uuid])", f.b.ID, tenant.TenantID, initialEvent).Scan(&hidden))
		if !hidden {
			t.Fatal("database projection exposed a revoked tenant name")
		}
	})
}
