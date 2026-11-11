package tenancy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

type Availability struct {
	Summary
	MembershipID string     `json:"membership_id,omitempty"`
	Status       string     `json:"status"`
	ChangedAt    *time.Time `json:"status_changed_at"`
	Notice       string     `json:"status_notice"`
	CanEnter     bool       `json:"can_enter"`
	Restriction  string     `json:"restriction_code"`
}
type StatusImpact struct {
	ActiveMembers          int `json:"active_members"`
	SuspendedMembers       int `json:"suspended_members"`
	CurrentBusinessMembers int `json:"current_business_members"`
	EligibleMembers        int `json:"eligible_members"`
	UnavailableAccounts    int `json:"unavailable_accounts"`
	NoPermissions          int `json:"no_permissions"`
	PendingInvitations     int `json:"pending_invitations"`
}
type StatusPreview struct {
	TenantID               string       `json:"tenant_id"`
	Name                   string       `json:"name"`
	Code                   string       `json:"code"`
	CurrentStatus          string       `json:"current_status"`
	TargetStatus           string       `json:"target_status"`
	Version                string       `json:"expected_version"`
	Revision               string       `json:"authorization_revision"`
	Checksum               string       `json:"catalog_checksum"`
	Hash                   string       `json:"preview_hash"`
	GeneratedAt            time.Time    `json:"generated_at"`
	Impact                 StatusImpact `json:"impact"`
	Administrator          Account      `json:"administrator"`
	AdministratorAvailable bool         `json:"administrator_available"`
	AdministratorValid     bool         `json:"administrator_valid"`
	CanSubmit              bool         `json:"can_submit"`
	BlockedReasons         []string     `json:"blocked_reasons"`
}
type StatusInput struct {
	Version string  `json:"expected_version"`
	Hash    string  `json:"preview_hash"`
	Reason  string  `json:"reason"`
	Notice  *string `json:"tenant_notice,omitempty"`
}
type StatusResult struct {
	TenantID  string     `json:"tenant_id"`
	Status    string     `json:"status"`
	Version   string     `json:"version"`
	ChangedAt *time.Time `json:"status_changed_at"`
	Revision  string     `json:"authorization_revision"`
}

func availabilityTx(ctx context.Context, tx pgx.Tx, id string) (Availability, error) {
	var v Availability
	e := tx.QueryRow(ctx, `SELECT id::text,name,code,status,status_changed_at,status_notice FROM tenants WHERE id=$1`, id).Scan(&v.ID, &v.Name, &v.Code, &v.Status, &v.ChangedAt, &v.Notice)
	v.CanEnter = v.Status == "active"
	if !v.CanEnter {
		v.Restriction = "tenant_disabled"
	}
	return v, e
}
func (s *Service) Availability(ctx context.Context, a access.Actor, id string) (Availability, error) {
	var v Availability
	e := s.transact(ctx, a, access.OpTenantAvailability, id, "", func(tx pgx.Tx, _ string) error {
		if !access.ValidID(id) {
			return access.ErrTenantNotFound
		}
		var member string
		e := tx.QueryRow(ctx, `SELECT m.id::text FROM tenant_memberships m JOIN accounts a ON a.id=m.account_id WHERE m.tenant_id=$1 AND m.account_id=$2 AND m.status='active' AND a.status='active'`, id, a.ID).Scan(&member)
		if e == pgx.ErrNoRows {
			return access.ErrTenantNotFound
		}
		if e != nil {
			return e
		}
		v, e = availabilityTx(ctx, tx, id)
		v.MembershipID = member
		return e
	})
	return v, e
}
func statusOperation(target string, preview bool) (access.Operation, error) {
	switch target {
	case "disabled":
		if preview {
			return access.OpTenantDisablePreview, nil
		}
		return access.OpTenantDisable, nil
	case "active":
		if preview {
			return access.OpTenantEnablePreview, nil
		}
		return access.OpTenantEnable, nil
	default:
		return "", access.Validation("status", "租户状态无效。")
	}
}
func statusPreviewTx(ctx context.Context, tx pgx.Tx, t Tenant, target, rev string) (StatusPreview, error) {
	p := StatusPreview{TenantID: t.ID, Name: t.Name, Code: t.Code, CurrentStatus: t.Status, TargetStatus: target, Version: t.Version, Revision: rev, Checksum: access.Checksum(), Administrator: t.Administrator, AdministratorAvailable: t.AdministratorAvailable, BlockedReasons: []string{}}
	if e := access.CheckTenantStructure(ctx, tx); e != nil {
		return p, e
	}
	rows, e := tx.Query(ctx, `SELECT id::text FROM tenant_memberships WHERE tenant_id=$1 AND status<>'ended' ORDER BY id`, t.ID)
	if e != nil {
		return p, e
	}
	mids, e := ids(rows)
	if e != nil {
		return p, e
	}
	for _, mid := range mids {
		a, e := access.TenantAuthorizationTx(ctx, tx, t.ID, mid, rev)
		if e != nil {
			return p, e
		}
		if a.MembershipStatus == "active" {
			p.Impact.ActiveMembers++
		} else {
			p.Impact.SuspendedMembers++
		}
		if a.AccountStatus != "active" {
			p.Impact.UnavailableAccounts++
		}
		if len(a.RestorablePermissions) == 0 {
			p.Impact.NoPermissions++
		}
		if a.AccountStatus == "active" && a.MembershipStatus == "active" && len(a.RestorablePermissions) > 0 {
			p.Impact.EligibleMembers++
		}
		if len(a.Permissions) > 0 {
			p.Impact.CurrentBusinessMembers++
		}
		if mid == t.MembershipID {
			p.AdministratorValid = a.Administrator && a.MembershipStatus == "active" && len(a.RestorablePermissions) > 0
		}
	}
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM tenant_invitations WHERE tenant_id=$1 AND status='pending' AND expires_at>clock_timestamp()`, t.ID).Scan(&p.Impact.PendingInvitations); e != nil {
		return p, e
	}
	if t.Status == target {
		p.BlockedReasons = append(p.BlockedReasons, "tenant_status_unchanged")
	}
	if target == "active" && (!p.AdministratorAvailable || !p.AdministratorValid) {
		p.BlockedReasons = append(p.BlockedReasons, "administrator_unavailable")
	}
	p.CanSubmit = len(p.BlockedReasons) == 0
	b, e := json.Marshal(p)
	if e != nil {
		return p, e
	}
	h := sha256.Sum256(b)
	p.Hash = hex.EncodeToString(h[:])
	e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&p.GeneratedAt)
	return p, e
}
func (s *Service) PreviewStatus(ctx context.Context, a access.Actor, id, target string) (StatusPreview, error) {
	var p StatusPreview
	op, e := statusOperation(target, true)
	if e != nil {
		return p, e
	}
	e = s.transact(ctx, a, op, id, "", func(tx pgx.Tx, rev string) error {
		t, e := tenantTx(ctx, tx, id, false)
		if e != nil {
			return e
		}
		p, e = statusPreviewTx(ctx, tx, t, target, rev)
		return e
	})
	return p, e
}
func (s *Service) ChangeStatus(ctx context.Context, a access.Actor, id, target string, in StatusInput, m access.Meta) (StatusResult, error) {
	var out StatusResult
	op, e := statusOperation(target, false)
	if e != nil {
		return out, e
	}
	e = s.transact(ctx, a, op, id, "", func(tx pgx.Tx, rev string) error {
		in.Reason = strings.TrimSpace(in.Reason)
		if e := access.ValidReason(in.Reason); e != nil {
			return e
		}
		notice := ""
		if target == "disabled" {
			if in.Notice != nil {
				notice = strings.TrimSpace(*in.Notice)
			}
			if !utf8.ValidString(notice) || utf8.RuneCountInString(notice) < 1 || utf8.RuneCountInString(notice) > 500 {
				return access.Validation("tenant_notice", "请填写 1–500 字符的租户可见说明。")
			}
		} else if in.Notice != nil {
			return access.Validation("tenant_notice", "恢复租户不接受停用说明。")
		}
		if decoded, err := hex.DecodeString(in.Hash); err != nil || len(decoded) != 32 {
			return access.Validation("preview_hash", "请重新读取操作影响。")
		}
		old, e := tenantTx(ctx, tx, id, true)
		if e != nil {
			return e
		}
		if e = version(old.Version, in.Version); e != nil {
			return e
		}
		if old.Status == target {
			return problem("tenant_status_unchanged", "租户已经处于目标状态，请核对当前结果。", 409)
		}
		p, e := statusPreviewTx(ctx, tx, old, target, rev)
		if e != nil {
			return e
		}
		if !p.CanSubmit {
			return problem("tenant_restore_blocked", "当前管理员不可用，请先完成合法交接，再单独恢复租户。", 409)
		}
		if p.Hash != in.Hash {
			return problem("preview_changed", "操作影响已变化，请重新查看并确认。", 409)
		}
		_, e = tx.Exec(ctx, `UPDATE tenants SET status=$2,status_changed_at=clock_timestamp(),status_changed_by=$3,status_change_reason=$4,status_notice=$5,version=version+1,updated_by=$3,updated_at=clock_timestamp() WHERE id=$1`, id, target, a.ID, in.Reason, notice)
		if e != nil {
			return e
		}
		if e = access.Bump(ctx, tx); e != nil {
			return e
		}
		next, e := tenantTx(ctx, tx, id, false)
		if e != nil {
			return e
		}
		if e = tx.QueryRow(ctx, `SELECT revision::text FROM authorization_state WHERE id=1`).Scan(&out.Revision); e != nil {
			return e
		}
		event := "tenant.disabled"
		if target == "active" {
			event = "tenant.enabled"
		}
		// Explicit public projection is also used for tenant-side filtering, never raw internal reason/details.
		public := map[string]any{"tenant_id": id, "name": old.Name, "code": old.Code, "before_status": old.Status, "after_status": target, "tenant_notice": notice, "status_changed_at": next.StatusChangedAt}
		if e = access.Audit(ctx, tx, event, "tenant", id, "", old, map[string]any{"tenant": next, "impact": p.Impact, "authorization_revision": out.Revision, "public": public}, meta(a, m, id, in.Reason, "platform")); e != nil {
			return e
		}
		out.TenantID = id
		out.Status = target
		out.Version = next.Version
		out.ChangedAt = next.StatusChangedAt
		return nil
	})
	return out, e
}
