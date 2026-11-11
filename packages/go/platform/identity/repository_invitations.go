package identity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const invitationColumns = `id::text,note,created_at,expires_at,CASE WHEN used_at IS NOT NULL THEN 'used' WHEN revoked_at IS NOT NULL THEN 'revoked' WHEN expires_at<=now() THEN 'expired' ELSE 'pending' END AS status`

func (r *Repository) CreateInvitation(ctx context.Context, p Principal, oldID, note, hash string, ttl, idle time.Duration, max int, m Meta) (time.Time, error) {
	var expires time.Time
	e := r.transact(ctx, func(tx pgx.Tx) error {
		op := access.OpInvitationCreate
		if oldID != "" {
			op = access.OpInvitationRegenerate
		}
		if e := access.Require(ctx, tx, p.Account.ID, op); e != nil {
			return e
		}
		if err := lockPrincipal(ctx, tx, p, idle, true); err != nil {
			return err
		}
		if oldID != "" {
			var used bool
			err := tx.QueryRow(ctx, "SELECT note,used_at IS NOT NULL FROM account_invitations WHERE id=$1 AND inviter_id=$2 FOR UPDATE", oldID, p.Account.ID).Scan(&note, &used)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			if used {
				return problem("invitation_used", "已使用的邀请不能重新生成，请新建邀请。", 409)
			}
			if _, err = tx.Exec(ctx, "UPDATE account_invitations SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1", oldID); err != nil {
				return err
			}
			if err = audit(ctx, tx, "invitation_revoked", p.Account.ID, p.Account.ID, oldID, "success", m); err != nil {
				return err
			}
		}
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM account_invitations WHERE inviter_id=$1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at>now()", p.Account.ID).Scan(&count); err != nil {
			return err
		}
		if count >= max {
			return problem("invitation_limit", "有效邀请已达上限，请先撤销不再使用的邀请。", 409)
		}
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO account_invitations(inviter_id,token_hash,note,expires_at) VALUES($1,$2,$3,now()+$4::interval) RETURNING id::text,expires_at`, p.Account.ID, hash, note, interval(ttl)).Scan(&id, &expires); err != nil {
			return err
		}
		return audit(ctx, tx, "invitation_created", p.Account.ID, p.Account.ID, id, "success", m)
	})
	return expires, e
}
func (r *Repository) RevokeInvitation(ctx context.Context, p Principal, id string, idle time.Duration, m Meta) error {
	return r.transact(ctx, func(tx pgx.Tx) error {
		if e := access.Require(ctx, tx, p.Account.ID, access.OpInvitationRevoke); e != nil {
			return e
		}
		if e := lockPrincipal(ctx, tx, p, idle, false); e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, "UPDATE account_invitations SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1 AND inviter_id=$2 AND used_at IS NULL", id, p.Account.ID)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return audit(ctx, tx, "invitation_revoked", p.Account.ID, p.Account.ID, id, "success", m)
	})
}

type invitationCursor struct {
	At     time.Time `json:"at"`
	ID     string    `json:"id"`
	Query  string    `json:"q,omitempty"`
	Status string    `json:"status,omitempty"`
}

func (r *Repository) Invitations(ctx context.Context, tx pgx.Tx, id string, filter InvitationFilter) (InvitationsPage, error) {
	page := InvitationsPage{Page: Page[Invitation]{Items: []Invitation{}}}
	filter.Query = strings.TrimSpace(filter.Query)
	if !utf8.ValidString(filter.Query) || utf8.RuneCountInString(filter.Query) > 200 {
		return page, FieldError("q", "搜索内容不能超过 200 个字符。")
	}
	switch filter.Status {
	case "", "pending", "used", "expired", "revoked":
	default:
		return page, FieldError("status", "邀请状态无效。")
	}
	pos := invitationCursor{At: time.Now().Add(time.Hour), ID: "ffffffff-ffff-ffff-ffff-ffffffffffff"}
	if filter.Cursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(filter.Cursor)
		if e != nil || json.Unmarshal(b, &pos) != nil || !uuidPattern.MatchString(pos.ID) || pos.At.IsZero() || pos.Query != filter.Query || pos.Status != filter.Status {
			return page, FieldError("cursor", "分页位置无效。")
		}
	}
	// One statement gives the page, matching total and unfiltered owner counts the
	// same snapshot. Literal substring search treats % and _ as ordinary text.
	e := tx.QueryRow(ctx, `WITH owned AS MATERIALIZED (
		SELECT `+invitationColumns+` FROM account_invitations WHERE inviter_id=$1
	), matched AS (
		SELECT * FROM owned WHERE ($4='' OR strpos(lower(note),lower($4))>0) AND ($5='' OR status=$5)
	), selected AS (
		SELECT * FROM matched WHERE (created_at,id::uuid)<($2,$3::uuid) ORDER BY created_at DESC,id::uuid DESC LIMIT 51
	)
	SELECT COALESCE((SELECT jsonb_agg(selected ORDER BY created_at DESC,id::uuid DESC) FROM selected),'[]'::jsonb),
		(SELECT count(*) FROM matched),count(*),count(*) FILTER (WHERE status='pending'),count(*) FILTER (WHERE status='used'),
		count(*) FILTER (WHERE status='expired'),count(*) FILTER (WHERE status='revoked') FROM owned`,
		id, pos.At, pos.ID, filter.Query, filter.Status).Scan(&page.Items, &page.Total, &page.Counts.All, &page.Counts.Pending, &page.Counts.Used, &page.Counts.Expired, &page.Counts.Revoked)
	if e != nil {
		return page, e
	}
	if len(page.Items) > 50 {
		page.Items = page.Items[:50]
		v := page.Items[49]
		b, _ := json.Marshal(invitationCursor{At: v.CreatedAt, ID: v.ID, Query: filter.Query, Status: filter.Status})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(b)
	}
	return page, nil
}
func (r *Repository) Invitation(ctx context.Context, tx pgx.Tx, owner, id string) (Invitation, error) {
	var v Invitation
	if !uuidPattern.MatchString(id) {
		return v, ErrNotFound
	}
	e := tx.QueryRow(ctx, "SELECT "+invitationColumns+" FROM account_invitations WHERE inviter_id=$1 AND id=$2", owner, id).Scan(&v.ID, &v.Note, &v.CreatedAt, &v.ExpiresAt, &v.Status)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return v, e
}
func (r *Repository) InspectInvitation(ctx context.Context, hash string) (Inspection, error) {
	var result Inspection
	e := r.transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		var inviter string
		e := tx.QueryRow(ctx, `SELECT i.expires_at,i.inviter_id::text FROM account_invitations i JOIN accounts a ON a.id=i.inviter_id WHERE i.token_hash=$1 AND i.used_at IS NULL AND i.revoked_at IS NULL AND i.expires_at>now() AND a.status='active'`, hash).Scan(&result.ExpiresAt, &inviter)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrLink
		}
		if e != nil {
			return e
		}
		ok, e := access.HasTx(ctx, tx, inviter, access.InvitationCreate)
		if e != nil {
			return e
		}
		if !ok {
			return ErrLink
		}
		return nil
	})
	return result, e
}
func (r *Repository) AcceptInvitation(ctx context.Context, tokenHash, username, display, passwordHash string, m Meta) error {
	return r.transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, true); e != nil {
			return e
		}
		var inviter, id string
		e := tx.QueryRow(ctx, `SELECT a.id::text FROM accounts a JOIN account_invitations i ON i.inviter_id=a.id WHERE i.token_hash=$1 AND a.status='active' FOR UPDATE OF a`, tokenHash).Scan(&inviter)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrLink
		}
		if e != nil {
			return e
		}
		allowed, e := access.HasTx(ctx, tx, inviter, access.InvitationCreate)
		if e != nil {
			return e
		}
		if !allowed {
			return ErrLink
		}
		e = tx.QueryRow(ctx, `SELECT id::text FROM account_invitations WHERE token_hash=$1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at>now() FOR UPDATE`, tokenHash).Scan(&id)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrLink
		}
		if e != nil {
			return e
		}
		a, e := insertAccount(ctx, tx, username, display, passwordHash)
		if e != nil {
			return e
		}
		if e = access.BindBuiltin(ctx, tx, a.ID, access.MemberKey, access.Meta{ActorID: inviter, ActorType: "account", Reason: "邀请开户", RequestID: m.RequestID}); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE account_invitations SET used_at=now(),invitee_id=$2 WHERE id=$1", id, a.ID); e != nil {
			return e
		}
		if e = audit(ctx, tx, "invitation_accepted", a.ID, inviter, id, "success", m); e != nil {
			return e
		}
		return audit(ctx, tx, "account_created", a.ID, a.ID, id, "success", m)
	})
}
