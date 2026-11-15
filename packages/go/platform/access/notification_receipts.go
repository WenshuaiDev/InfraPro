package access

import (
	"context"
	"errors"
	"fmt"
	"slices"

	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/jackc/pgx/v5"
)

// NotificationRecipientsTx is used by a domain command while it holds the
// authorization graph lock. It snapshots today's members, never a future query.
// An empty permission selects all effective members; otherwise any listed code
// qualifies (e.g. the finite display management permissions).
func NotificationRecipientsTx(ctx context.Context, tx pgx.Tx, tenant, project string, permissions ...Code) ([]string, error) {
	query := `SELECT account_id::text FROM tenant_memberships WHERE tenant_id=$1 AND status='active' ORDER BY account_id`
	args := []any{tenant}
	if project != "" {
		query = `SELECT tm.account_id::text FROM project_memberships pm JOIN tenant_memberships tm ON tm.tenant_id=pm.tenant_id AND tm.id=pm.tenant_membership_id WHERE pm.tenant_id=$1 AND pm.project_id=$2 AND pm.status='active' AND tm.status='active' ORDER BY tm.account_id`
		args = append(args, project)
	}
	rows, e := tx.Query(ctx, query, args...)
	if e != nil {
		return nil, e
	}
	ids, e := readIDs(rows)
	if e != nil {
		return nil, e
	}
	out := []string{}
	for _, id := range ids {
		var codes []Code
		if project == "" {
			_, v, err := TenantSnapshotTx(ctx, tx, id, tenant, "")
			e = err
			codes = v.Permissions
		} else {
			v, err := ProjectSnapshotTx(ctx, tx, id, tenant, project, "")
			e = err
			codes = v.Permissions
		}
		if e != nil {
			var denied *Error
			if errors.As(e, &denied) && (denied.Status == 403 || denied.Status == 404) {
				continue
			}
			return nil, e
		}
		if len(permissions) == 0 && len(codes) > 0 {
			out = append(out, id)
			continue
		}
		for _, p := range permissions {
			if slices.Contains(codes, p) {
				out = append(out, id)
				break
			}
		}
	}
	return out, nil
}

// NotificationReceiptTx is explicit at a business transition, not an audit hook.
func NotificationReceiptTx(ctx context.Context, tx pgx.Tx, module, typ, outcome, actor string, scope ne.Scope, target ne.Target, recipients []ne.Recipient) error {
	_, e := NotificationReceiptRefTx(ctx, tx, module, typ, outcome, actor, scope, target, recipients)
	return e
}

// NotificationReceiptRefTx preserves the original source key and payload.
func NotificationReceiptRefTx(ctx context.Context, tx pgx.Tx, module, typ, outcome, actor string, scope ne.Scope, target ne.Target, recipients []ne.Recipient) (string, error) {
	var transaction string
	if e := tx.QueryRow(ctx, `SELECT pg_current_xact_id()::text`).Scan(&transaction); e != nil {
		return "", e
	}
	key := fmt.Sprintf("%s:%s:%s:%s:%s:%s", transaction, typ, scope.Tenant, scope.Project, target.ID, outcome)
	if len(recipients) == 1 {
		key += ":" + recipients[0].Account
	}
	return ne.AppendTx(ctx, tx, ne.Event{Module: module, Key: key, Type: typ, Scope: scope, Actor: actor, Target: target, Outcome: outcome, Recipients: recipients})
}
func NotifyProjectGrantTx(ctx context.Context, tx pgx.Tx, module, tenant, project, actor string) error {
	ids, e := NotificationRecipientsTx(ctx, tx, tenant, project, ProjectProfileUpdate)
	if e != nil {
		return e
	}
	return NotificationReceiptTx(ctx, tx, module, "grant.changed", "changed", actor, ne.BusinessScope(tenant, project), ne.Target{Kind: "project_access", ID: project}, ne.Scoped(ids...))
}

// LifecycleMembersTx intentionally ignores only the container's disabled state
// so an enable event reaches the existing members, not later joiners.
func LifecycleMembersTx(ctx context.Context, tx pgx.Tx, tenant, project string) ([]string, error) {
	q := `SELECT m.account_id::text FROM tenant_memberships m JOIN accounts a ON a.id=m.account_id WHERE m.tenant_id=$1 AND m.status='active' AND a.status='active' ORDER BY m.account_id`
	args := []any{tenant}
	if project != "" {
		q = `SELECT m.account_id::text FROM project_memberships m JOIN tenant_memberships tm ON tm.id=m.tenant_membership_id AND tm.tenant_id=m.tenant_id JOIN accounts a ON a.id=m.account_id WHERE m.tenant_id=$1 AND m.project_id=$2 AND m.status='active' AND tm.status='active' AND a.status='active' ORDER BY m.account_id`
		args = append(args, project)
	}
	rows, e := tx.Query(ctx, q, args...)
	if e != nil {
		return nil, e
	}
	return readIDs(rows)
}
