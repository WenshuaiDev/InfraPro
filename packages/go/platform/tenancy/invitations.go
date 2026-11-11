package tenancy

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

type Invitation struct {
	ID                  string             `json:"id"`
	TenantID            string             `json:"tenant_id"`
	Target              Account            `json:"target"`
	Inviter             Account            `json:"inviter"`
	InviterMembershipID string             `json:"inviter_membership_id"`
	Role                string             `json:"role"`
	Status              string             `json:"status"`
	Version             string             `json:"version"`
	CreatedAt           time.Time          `json:"created_at"`
	ExpiresAt           time.Time          `json:"expires_at"`
	ClosedAt            *time.Time         `json:"closed_at"`
	ClosedBy            string             `json:"closed_by"`
	CloseReason         string             `json:"close_reason"`
	AcceptedAt          *time.Time         `json:"accepted_at"`
	MembershipID        string             `json:"accepted_membership_id"`
	MembershipStatus    string             `json:"membership_status"`
	ReplacesID          string             `json:"replaces_id"`
	ReplacedByID        string             `json:"replaced_by_id"`
	Snapshot            *InvitationPreview `json:"-"`
}
type InvitationPreview struct {
	Tenant  Summary `json:"tenant"`
	Account Account `json:"account"`
	Inviter Account `json:"inviter"`
	Role    string  `json:"role"`
}
type InvitationInspection struct {
	TenantStatus     string             `json:"tenant_status"`
	BlockedReason    string             `json:"blocked_reason"`
	Availability     *Availability      `json:"availability,omitempty"`
	ID               string             `json:"invitation_id"`
	Status           string             `json:"status"`
	Version          string             `json:"version"`
	CanAccept        bool               `json:"can_accept"`
	ExpiresAt        time.Time          `json:"expires_at"`
	Preview          *InvitationPreview `json:"preview,omitempty"`
	MembershipID     string             `json:"membership_id"`
	MembershipStatus string             `json:"membership_status"`
}
type InvitationCreateInput struct {
	TargetID  string `json:"target_account_id"`
	Reason    string `json:"reason"`
	RequestID string `json:"client_request_id"`
}
type InvitationRegenerateInput struct {
	Version   string `json:"expected_version"`
	Reason    string `json:"reason"`
	RequestID string `json:"client_request_id"`
}
type InvitationCreated struct {
	Invitation      Invitation `json:"invitation"`
	Link            string     `json:"link,omitempty"`
	Replayed        bool       `json:"replayed"`
	LinkUnavailable bool       `json:"link_unavailable"`
}
type AcceptanceInput struct {
	Token   string `json:"token"`
	Version string `json:"expected_version"`
}
type AcceptanceResult struct {
	TenantID          string `json:"tenant_id"`
	MembershipID      string `json:"membership_id"`
	Version           string `json:"membership_version"`
	InvitationID      string `json:"invitation_id"`
	InvitationVersion string `json:"invitation_version"`
	Revision          string `json:"authorization_revision"`
}

var errPending = problem("invitation_pending_exists", "已有待接受邀请，请查看或明确重新生成。", 409)
var errInvitation = problem("tenant_invitation_not_found", "邀请不存在或不属于当前租户。", 404)
var errUnavailable = problem("tenant_invitation_unavailable", "邀请不存在、已失效或不属于当前账号。", 404)
var errNotPending = problem("invitation_not_pending", "邀请已结束或过期，请查验当前状态。", 409)
var errAccepted = problem("invitation_already_accepted", "邀请已经接受，请查看原加入结果。", 409)

const invitationSelect = `SELECT i.id::text,i.tenant_id::text,a.id::text,a.username,a.display_name,b.id::text,b.username,b.display_name,i.inviter_membership_id::text,i.role_key,
 CASE WHEN i.status='pending' AND i.expires_at<=clock_timestamp() THEN 'expired' ELSE i.status END,i.version::text,i.created_at,i.expires_at,i.closed_at,COALESCE(i.closed_by::text,''),COALESCE(i.close_reason,''),i.accepted_at,COALESCE(i.accepted_membership_id::text,''),COALESCE(m.status,''),COALESCE(i.replaces_id::text,''),COALESCE(i.replaced_by_id::text,''),i.accepted_context_snapshot
 FROM tenant_invitations i JOIN accounts a ON a.id=i.target_account_id JOIN accounts b ON b.id=i.inviter_account_id LEFT JOIN tenant_memberships m ON m.id=i.accepted_membership_id AND m.tenant_id=i.tenant_id`

func scanInvitation(row pgx.Row) (Invitation, error) {
	var v Invitation
	e := row.Scan(&v.ID, &v.TenantID, &v.Target.ID, &v.Target.Username, &v.Target.DisplayName, &v.Inviter.ID, &v.Inviter.Username, &v.Inviter.DisplayName, &v.InviterMembershipID, &v.Role, &v.Status, &v.Version, &v.CreatedAt, &v.ExpiresAt, &v.ClosedAt, &v.ClosedBy, &v.CloseReason, &v.AcceptedAt, &v.MembershipID, &v.MembershipStatus, &v.ReplacesID, &v.ReplacedByID, &v.Snapshot)
	return v, e
}
func invitationTx(ctx context.Context, tx pgx.Tx, tenant, id string) (Invitation, error) {
	if !access.ValidID(id) {
		return Invitation{}, errInvitation
	}
	v, e := scanInvitation(tx.QueryRow(ctx, invitationSelect+` WHERE i.tenant_id=$1 AND i.id=$2`, tenant, id))
	if e == pgx.ErrNoRows {
		e = errInvitation
	}
	return v, e
}
func (s *Service) Invitations(ctx context.Context, a access.Actor, tenant string, f Filter) (access.Page[Invitation], error) {
	out := access.Page[Invitation]{Items: []Invitation{}}
	e := s.transact(ctx, a, access.OpTenantInvitationList, tenant, "", func(tx pgx.Tx, _ string) error {
		at, id, e := membershipFilter(&f)
		if e != nil {
			return e
		}
		if !oneOf(f.Status, "", "pending", "accepted", "expired", "revoked", "replaced", "invalidated") {
			return access.Validation("status", "邀请状态无效。")
		}
		var from, to *time.Time
		for _, v := range []struct {
			raw string
			dst **time.Time
		}{{f.From, &from}, {f.To, &to}} {
			if v.raw != "" {
				t, e := time.Parse(time.RFC3339, v.raw)
				if e != nil {
					return access.Validation("time", "时间格式无效。")
				}
				*v.dst = &t
			}
		}
		if from != nil && to != nil && from.After(*to) {
			return access.Validation("time", "起始时间不得晚于结束时间。")
		}
		rows, e := tx.Query(ctx, invitationSelect+` WHERE i.tenant_id=$1 AND ($2='' OR (CASE WHEN i.status='pending' AND i.expires_at<=clock_timestamp() THEN 'expired' ELSE i.status END)=$2)
 AND ($3='' OR a.username ILIKE '%'||$3||'%' OR a.display_name ILIKE '%'||$3||'%') AND ($4::timestamptz IS NULL OR (i.created_at,i.id)<($4,$5::uuid))
 AND ($6::timestamptz IS NULL OR i.created_at>=$6) AND ($7::timestamptz IS NULL OR i.created_at<=$7) ORDER BY i.created_at DESC,i.id DESC LIMIT $8`, tenant, f.Status, f.Query, at, nullable(id), from, to, f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := scanInvitation(rows)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			v := out.Items[f.Limit-1]
			out.NextCursor = cursorFor(v.CreatedAt, v.ID)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) Invitation(ctx context.Context, a access.Actor, tenant, id string) (Invitation, error) {
	var out Invitation
	e := s.transact(ctx, a, access.OpTenantInvitationDetail, tenant, "", func(tx pgx.Tx, _ string) error { var e error; out, e = invitationTx(ctx, tx, tenant, id); return e })
	return out, e
}

// Limits are enforced before long transactions; Redis errors remain dependency errors.
func (s *Service) limit(ctx context.Context, scope, key string, max int) error {
	if s.options.Limit != nil {
		return s.options.Limit(ctx, scope, key, max)
	}
	return nil
}

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

func (s *Service) ResolveMember(ctx context.Context, a access.Actor, tenant, username string) (Account, error) {
	var out Account
	if e := s.limit(ctx, "tenant-member-resolve", a.ID+":"+tenant, 20); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, access.OpTenantMemberResolve, tenant, "", func(tx pgx.Tx, _ string) error {
		username = strings.TrimSpace(username)
		if !usernamePattern.MatchString(username) {
			return access.Validation("username", "请输入完整用户名。")
		}
		e := tx.QueryRow(ctx, `SELECT id::text,username,display_name FROM accounts WHERE username_normalized=$1 AND status='active'`, strings.ToLower(username)).Scan(&out.ID, &out.Username, &out.DisplayName)
		if e == pgx.ErrNoRows {
			return problem("target_account_unavailable", "未找到可邀请的账号。", 404)
		}
		return e
	})
	return out, e
}
func digest(s string) string         { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func invitationPayload(v any) string { b, _ := json.Marshal(v); return digest(string(b)) }
func currentAbsent(ctx context.Context, tx pgx.Tx, tenant, account string) error {
	var status string
	e := tx.QueryRow(ctx, `SELECT status FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2 AND status IN ('active','suspended')`, tenant, account).Scan(&status)
	if e == pgx.ErrNoRows {
		return nil
	}
	if e != nil {
		return e
	}
	if status == "suspended" {
		return problem("member_suspended", "该账号的成员关系已暂停，请使用恢复流程。", 409)
	}
	return problem("member_already_exists", "该账号已经是本租户成员。", 409)
}

// The inviter's live grant must cover the complete fixed role, not just its name.
func grantSource(ctx context.Context, tx pgx.Tx, account, tenant, member string) (string, error) {
	m, permissions, e := access.InvitationQualificationTx(ctx, tx, account, tenant)
	if e != nil {
		return "", e
	}
	if member != "" && m != member || !slices.Contains(permissions, access.TenantInvitationCreate) {
		return "", access.ErrDenied
	}
	var role string
	e = tx.QueryRow(ctx, `SELECT id::text FROM roles WHERE builtin_key=$1 AND status='active' AND deleted_at IS NULL AND binding_scope='tenant'`, access.TenantMemberKey).Scan(&role)
	if e != nil {
		return "", access.ErrCatalog
	}
	rows, e := tx.Query(ctx, `SELECT rp.permission_code FROM role_permissions rp JOIN permission_catalog p ON p.code=rp.permission_code WHERE rp.role_id=$1 AND p.lifecycle='active'`, role)
	if e != nil {
		return "", e
	}
	defer rows.Close()
	for rows.Next() {
		var c access.Code
		if e = rows.Scan(&c); e != nil {
			return "", e
		}
		if !slices.Contains(permissions, c) {
			return "", access.ErrDenied
		}
	}
	return role, rows.Err()
}
func expireInvitations(ctx context.Context, tx pgx.Tx, tenant, target string, m access.Meta) error {
	rows, e := tx.Query(ctx, `UPDATE tenant_invitations SET status='expired',closed_at=clock_timestamp(),close_reason='expired',version=version+1 WHERE tenant_id=$1 AND target_account_id=$2 AND status='pending' AND expires_at<=clock_timestamp() RETURNING id::text,expires_at,closed_at`, tenant, target)
	if e != nil {
		return e
	}
	type expiry struct {
		id         string
		at, closed time.Time
	}
	list := []expiry{}
	for rows.Next() {
		var v expiry
		if e = rows.Scan(&v.id, &v.at, &v.closed); e != nil {
			rows.Close()
			return e
		}
		list = append(list, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range list {
		if e = access.Audit(ctx, tx, "tenant.invitation.expired", "tenant_invitation", v.id, "", map[string]string{"status": "pending"}, map[string]any{"status": "expired", "expires_at": v.at, "closed_at": v.closed}, m); e != nil {
			return e
		}
	}
	return nil
}
func replayInvitation(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, op, request, hash string) (InvitationCreated, bool, error) {
	var id, old string
	e := tx.QueryRow(ctx, `SELECT id::text,creation_payload_hash FROM tenant_invitations WHERE tenant_id=$1 AND inviter_account_id=$2 AND creation_operation=$3 AND creation_request_id=$4`, tenant, a.ID, op, request).Scan(&id, &old)
	if e == pgx.ErrNoRows {
		return InvitationCreated{}, false, nil
	}
	if e != nil {
		return InvitationCreated{}, false, e
	}
	if hash != old {
		return InvitationCreated{}, true, problem("idempotency_conflict", "相同请求编号的内容已改变，请核实原结果。", 409)
	}
	v, e := invitationTx(ctx, tx, tenant, id)
	return InvitationCreated{Invitation: v, Replayed: true, LinkUnavailable: true}, true, e
}
func (s *Service) issueInvitation(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, target, request, hash, op, replaces string, m access.Meta) (InvitationCreated, error) {
	var out InvitationCreated
	if !access.ValidID(target) {
		return out, access.Validation("target_account_id", "目标账号编号无效。")
	}
	if e := activeAccount(ctx, tx, target); e != nil {
		if e == errAdmin {
			return out, errTarget
		}
		return out, e
	}
	if e := currentAbsent(ctx, tx, tenant, target); e != nil {
		return out, e
	}
	mid, _, e := access.TenantSnapshotTx(ctx, tx, a.ID, tenant, "")
	if e != nil {
		return out, e
	}
	if _, e = grantSource(ctx, tx, a.ID, tenant, mid); e != nil {
		return out, e
	}
	if e = expireInvitations(ctx, tx, tenant, target, m); e != nil {
		return out, e
	}
	var pending bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tenant_invitations WHERE tenant_id=$1 AND target_account_id=$2 AND status='pending')`, tenant, target).Scan(&pending); e != nil {
		return out, e
	}
	if pending {
		return out, errPending
	}
	var random [32]byte
	if _, e = rand.Read(random[:]); e != nil {
		return out, e
	}
	token := base64.RawURLEncoding.EncodeToString(random[:])
	var id string
	e = tx.QueryRow(ctx, `WITH stamp AS (SELECT clock_timestamp() AS at) INSERT INTO tenant_invitations(tenant_id,target_account_id,inviter_account_id,inviter_membership_id,token_hash,created_at,expires_at,creation_request_id,creation_payload_hash,creation_operation,replaces_id)
 SELECT $1,$2,$3,$4,$5,at,at+interval '24 hours',$6,$7,$8,$9 FROM stamp RETURNING id::text`, tenant, target, a.ID, mid, digest(token), request, hash, op, nullable(replaces)).Scan(&id)
	if e != nil {
		return out, e
	}
	out.Invitation, e = invitationTx(ctx, tx, tenant, id)
	if e != nil {
		return out, e
	}
	out.Link = strings.TrimRight(s.options.ConsoleURL, "/") + "/tenant-invite#token=" + token
	if e = access.Audit(ctx, tx, "tenant.invitation.created", "tenant_invitation", id, "", nil, out.Invitation, m); e != nil {
		return InvitationCreated{}, e
	}
	return out, nil
}
func normalizeInvitation(reason, request *string) error {
	*reason = strings.TrimSpace(*reason)
	*request = strings.ToLower(*request)
	if e := access.ValidReason(*reason); e != nil {
		return e
	}
	if !access.ValidID(*request) {
		return access.Validation("client_request_id", "请求编号需为 UUID。")
	}
	return nil
}
func (s *Service) CreateInvitation(ctx context.Context, a access.Actor, tenant string, in InvitationCreateInput, m access.Meta) (InvitationCreated, error) {
	var out InvitationCreated
	if e := s.limit(ctx, "tenant-invitation-issue", a.ID+":"+tenant, 10); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, access.OpTenantInvitationCreate, tenant, in.TargetID, func(tx pgx.Tx, _ string) error {
		if e := normalizeInvitation(&in.Reason, &in.RequestID); e != nil {
			return e
		}
		in.TargetID = strings.ToLower(in.TargetID)
		hash := invitationPayload(in)
		v, found, e := replayInvitation(ctx, tx, a, tenant, "create", in.RequestID, hash)
		if found || e != nil {
			out = v
			return e
		}
		out, e = s.issueInvitation(ctx, tx, a, tenant, in.TargetID, in.RequestID, hash, "create", "", meta(a, m, tenant, in.Reason, "tenant"))
		return e
	})
	return out, e
}
func (s *Service) RevokeInvitation(ctx context.Context, a access.Actor, tenant, id string, in MemberInput, m access.Meta) (Invitation, error) {
	var out Invitation
	e := s.transact(ctx, a, access.OpTenantInvitationRevoke, tenant, id, func(tx pgx.Tx, _ string) error {
		old, e := invitationTx(ctx, tx, tenant, id)
		if e != nil {
			return e
		}
		if old.Status != "pending" {
			return errNotPending
		}
		if e = version(old.Version, in.Version); e != nil {
			return e
		}
		if e = access.ValidReason(in.Reason); e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, `UPDATE tenant_invitations SET status='revoked',closed_at=clock_timestamp(),closed_by=$3,close_reason=$4,version=version+1 WHERE tenant_id=$1 AND id=$2 AND status='pending' AND expires_at>clock_timestamp()`, tenant, id, a.ID, strings.TrimSpace(in.Reason))
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return errNotPending
		}
		out, e = invitationTx(ctx, tx, tenant, id)
		if e != nil {
			return e
		}
		return access.Audit(ctx, tx, "tenant.invitation.revoked", "tenant_invitation", id, "", old, out, meta(a, m, tenant, in.Reason, "tenant"))
	})
	return out, e
}
func (s *Service) RegenerateInvitation(ctx context.Context, a access.Actor, tenant, id string, in InvitationRegenerateInput, m access.Meta) (InvitationCreated, error) {
	var out InvitationCreated
	if e := s.limit(ctx, "tenant-invitation-issue", a.ID+":"+tenant, 10); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, access.OpTenantInvitationRegenerate, tenant, id, func(tx pgx.Tx, _ string) error {
		if e := normalizeInvitation(&in.Reason, &in.RequestID); e != nil {
			return e
		}
		hash := invitationPayload(struct {
			ID    string
			Input InvitationRegenerateInput
		}{strings.ToLower(id), in})
		v, found, e := replayInvitation(ctx, tx, a, tenant, "regenerate", in.RequestID, hash)
		if found || e != nil {
			out = v
			return e
		}
		old, e := invitationTx(ctx, tx, tenant, id)
		if e != nil {
			return e
		}
		if old.Status == "accepted" {
			return errAccepted
		}
		if old.Status == "replaced" || old.ReplacedByID != "" {
			return errNotPending
		}
		if e = version(old.Version, in.Version); e != nil {
			return e
		}
		m = meta(a, m, tenant, in.Reason, "tenant")
		if e = expireInvitations(ctx, tx, tenant, old.Target.ID, m); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE tenant_invitations SET status=CASE WHEN status='pending' THEN 'replaced' ELSE status END,closed_at=COALESCE(closed_at,clock_timestamp()),closed_by=CASE WHEN status='pending' THEN $3::uuid ELSE closed_by END,close_reason=CASE WHEN status='pending' THEN $4 ELSE close_reason END WHERE tenant_id=$1 AND id=$2`, tenant, id, a.ID, in.Reason); e != nil {
			return e
		}
		out, e = s.issueInvitation(ctx, tx, a, tenant, old.Target.ID, in.RequestID, hash, "regenerate", id, m)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE tenant_invitations SET replaced_by_id=$3,version=version+1 WHERE tenant_id=$1 AND id=$2`, tenant, id, out.Invitation.ID); e != nil {
			return e
		}
		return access.Audit(ctx, tx, "tenant.invitation.regenerated", "tenant_invitation", id, "", old, map[string]any{"replacement": out.Invitation}, m)
	})
	return out, e
}
func (s *Service) credentialRate(ctx context.Context, a access.Actor, token string, m access.Meta) error {
	if e := s.limit(ctx, "tenant-invitation-recipient", a.ID, 30); e != nil {
		return e
	}
	if s.options.CredentialLimit != nil {
		return s.options.CredentialLimit(ctx, "tenant-invitation", m.IP, token)
	}
	return nil
}
func recipientInvitation(ctx context.Context, tx pgx.Tx, a access.Actor, token string) (Invitation, error) {
	if len(token) != 43 {
		return Invitation{}, errUnavailable
	}
	v, e := scanInvitation(tx.QueryRow(ctx, invitationSelect+` WHERE i.token_hash=$1 AND i.target_account_id=$2`, digest(token), a.ID))
	if e == pgx.ErrNoRows {
		e = errUnavailable
	}
	return v, e
}
func previewTx(ctx context.Context, tx pgx.Tx, v Invitation) (*InvitationPreview, error) {
	p := &InvitationPreview{Account: v.Target, Inviter: v.Inviter, Role: "租户普通成员：可查看本租户基本资料"}
	e := tx.QueryRow(ctx, `SELECT id::text,name,code FROM tenants WHERE id=$1`, v.TenantID).Scan(&p.Tenant.ID, &p.Tenant.Name, &p.Tenant.Code)
	return p, e
}
func (s *Service) InspectInvitation(ctx context.Context, a access.Actor, token string, m access.Meta) (InvitationInspection, error) {
	var out InvitationInspection
	if e := s.credentialRate(ctx, a, token, m); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, access.OpTenantInvitationInspect, "", digest(token), func(tx pgx.Tx, _ string) error {
		v, e := recipientInvitation(ctx, tx, a, token)
		if e != nil {
			return e
		}
		out = InvitationInspection{ID: v.ID, Status: v.Status, Version: v.Version, ExpiresAt: v.ExpiresAt, MembershipID: v.MembershipID, MembershipStatus: v.MembershipStatus}
		if v.Status == "accepted" {
			out.Preview = v.Snapshot
			// An accepted token proves a historical result, not current tenant access.
			if v.MembershipStatus != "active" {
				return nil
			}
			av, e := availabilityTx(ctx, tx, v.TenantID)
			out.Availability = &av
			out.TenantStatus = av.Status
			if !av.CanEnter {
				out.BlockedReason = "tenant_disabled"
			}
			return e
		}
		if v.Status != "pending" {
			return nil
		}
		if _, e = grantSource(ctx, tx, v.Inviter.ID, v.TenantID, v.InviterMembershipID); e != nil {
			if e == access.ErrDenied || e == access.ErrTenantNotFound {
				out.Status = "invalidated"
				return nil
			}
			return e
		}
		if e = currentAbsent(ctx, tx, v.TenantID, a.ID); e != nil {
			return e
		}
		av, e := availabilityTx(ctx, tx, v.TenantID)
		if e != nil {
			return e
		}
		out.Availability = &av
		out.TenantStatus = av.Status
		if !av.CanEnter {
			out.BlockedReason = "tenant_disabled"
			return nil
		}
		out.CanAccept = true
		out.Preview, e = previewTx(ctx, tx, v)
		return e
	})
	return out, e
}
func (s *Service) AcceptInvitation(ctx context.Context, a access.Actor, in AcceptanceInput, m access.Meta) (AcceptanceResult, error) {
	var out AcceptanceResult
	if e := s.credentialRate(ctx, a, in.Token, m); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, access.OpTenantInvitationAccept, "", digest(in.Token), func(tx pgx.Tx, _ string) error {
		v, e := recipientInvitation(ctx, tx, a, in.Token)
		if e != nil {
			return e
		}
		if v.Status == "accepted" {
			return errAccepted
		}
		if v.Status != "pending" {
			return errNotPending
		}
		if e = version(v.Version, in.Version); e != nil {
			return e
		}
		role, e := grantSource(ctx, tx, v.Inviter.ID, v.TenantID, v.InviterMembershipID)
		if e != nil {
			return e
		}
		if e = currentAbsent(ctx, tx, v.TenantID, a.ID); e != nil {
			return e
		}
		if e = access.RequireActiveTenantTx(ctx, tx, v.TenantID); e != nil {
			return e
		}
		preview, e := previewTx(ctx, tx, v)
		if e != nil {
			return e
		}
		var member string
		e = tx.QueryRow(ctx, `INSERT INTO tenant_memberships(tenant_id,account_id,joined_by,status_changed_by,source_invitation_id) VALUES($1,$2,$2,$2,$3) RETURNING id::text`, v.TenantID, a.ID, v.ID).Scan(&member)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO tenant_member_roles(tenant_id,membership_id,role_id,assigned_by) VALUES($1,$2,$3,$4)`, v.TenantID, member, role, v.Inviter.ID); e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, `UPDATE tenant_invitations SET status='accepted',version=version+1,closed_at=clock_timestamp(),closed_by=$2,close_reason='accepted',accepted_at=clock_timestamp(),accepted_by=$2,accepted_membership_id=$3,accepted_context_snapshot=$4 WHERE id=$1 AND status='pending' AND expires_at>clock_timestamp()`, v.ID, a.ID, member, preview)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return errNotPending
		}
		if e = access.Bump(ctx, tx); e != nil {
			return e
		}
		m = meta(a, m, v.TenantID, "本人确认加入", "tenant")
		after := map[string]any{"status": "accepted", "membership_id": member, "invitation_id": v.ID, "inviter_account_id": v.Inviter.ID, "inviter_membership_id": v.InviterMembershipID, "target": v.Target, "role": access.TenantMemberKey, "context": "target_invitation_confirmation"}
		if e = access.Audit(ctx, tx, "tenant.invitation.accepted", "tenant_invitation", v.ID, "", v, after, m); e != nil {
			return e
		}
		if e = access.Audit(ctx, tx, "tenant.member.joined", "tenant_member", member, "", nil, after, m); e != nil {
			return e
		}
		out = AcceptanceResult{TenantID: v.TenantID, MembershipID: member, Version: "1", InvitationID: v.ID}
		return tx.QueryRow(ctx, `SELECT i.version::text,s.revision::text FROM tenant_invitations i CROSS JOIN authorization_state s WHERE i.id=$1 AND s.id=1`, v.ID).Scan(&out.InvitationVersion, &out.Revision)
	})
	return out, e
}
