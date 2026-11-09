package identity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"time"

	"github.com/jackc/pgx/v5"
)

const invitationColumns = `id::text,note,created_at,expires_at,CASE WHEN used_at IS NOT NULL THEN 'used' WHEN revoked_at IS NOT NULL THEN 'revoked' WHEN expires_at<=now() THEN 'expired' ELSE 'pending' END`

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
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func (r *Repository) Invitations(ctx context.Context, tx pgx.Tx, id, cursor string) (Page[Invitation], error) {
	page := Page[Invitation]{Items: []Invitation{}}
	pos := invitationCursor{At: time.Now().Add(time.Hour), ID: "ffffffff-ffff-ffff-ffff-ffffffffffff"}
	if cursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(cursor)
		if e != nil || json.Unmarshal(b, &pos) != nil || !uuidPattern.MatchString(pos.ID) || pos.At.IsZero() {
			return page, FieldError("cursor", "分页位置无效。")
		}
	}
	rows, e := tx.Query(ctx, "SELECT "+invitationColumns+" FROM account_invitations WHERE inviter_id=$1 AND (created_at,id)<($2,$3::uuid) ORDER BY created_at DESC,id DESC LIMIT 51", id, pos.At, pos.ID)
	if e != nil {
		return page, e
	}
	defer rows.Close()
	for rows.Next() {
		var v Invitation
		if e = rows.Scan(&v.ID, &v.Note, &v.CreatedAt, &v.ExpiresAt, &v.Status); e != nil {
			return page, e
		}
		page.Items = append(page.Items, v)
	}
	if len(page.Items) > 50 {
		page.Items = page.Items[:50]
		v := page.Items[49]
		b, _ := json.Marshal(invitationCursor{At: v.CreatedAt, ID: v.ID})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(b)
	}
	return page, rows.Err()
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
