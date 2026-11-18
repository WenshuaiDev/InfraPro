package identity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/contentcheck"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const AvatarMaxBytes = contentcheck.AvatarInputBytes

type AvatarStore interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Get(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}

// WithAvatarStorage is a composition/test seam, never a request parameter.
func (s *Service) WithAvatarStorage(store AvatarStore) *Service { s.avatars = store; return s }

type AvatarScope struct{ Kind, Tenant, Project, ID string }
type avatarObject struct {
	ID, Key, Hash, Revision string
	Size                    int64
}

func avatarRevision(value string) error {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 || strconv.FormatInt(n, 10) != value {
		return FieldError("expected_revision", "请刷新头像后重试。")
	}
	return nil
}

func (s *Service) avatarWrite(ctx context.Context, p Principal, expected string, fn func(pgx.Tx) error) error {
	return s.repo.transact(ctx, func(tx pgx.Tx) error {
		if err := access.Require(ctx, tx, p.Account.ID, access.OpProfileUpdate); err != nil {
			return err
		}
		if err := lockPrincipal(ctx, tx, p, s.cfg.SessionIdle, false); err != nil {
			return err
		}
		var revision string
		if err := tx.QueryRow(ctx, `SELECT avatar_revision::text FROM accounts WHERE id=$1`, p.Account.ID).Scan(&revision); err != nil {
			return err
		}
		if revision != expected {
			return problem("avatar_changed", "头像已在其他页面更新，请刷新后重新选择。", 409)
		}
		return fn(tx)
	})
}

func (s *Service) UpdateAvatar(ctx context.Context, p Principal, expected string, input []byte, m Meta) (Account, error) {
	if err := avatarRevision(expected); err != nil {
		return Account{}, err
	}
	if err := s.Limit(ctx, "avatar-write", p.Account.ID, 10); err != nil {
		return Account{}, err
	}
	if err := s.avatarWrite(ctx, p, expected, func(pgx.Tx) error { return nil }); err != nil {
		return Account{}, err
	}
	if s.avatars == nil {
		return Account{}, problem("avatar_storage_unavailable", "头像存储暂不可用，请稍后重试。", 503)
	}
	select {
	case s.avatarSlots <- struct{}{}:
		defer func() { <-s.avatarSlots }()
	default:
		return Account{}, problem("avatar_busy", "头像处理繁忙，请稍后重试。", 429)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	data, err := contentcheck.Avatar(ctx, input)
	if err != nil {
		return Account{}, FieldError("image", "请选择有效的 JPG 或 PNG 图片（不超过 5 MB、2500 万像素）。")
	}
	sum := sha256.Sum256(data)
	id := uuid.NewString()
	key := "account-avatars/v1/" + p.Account.ID + "/" + id + ".png"
	// Register before I/O. Even uncertain writes/crashes leave a durable cleanup
	// intent. Pending objects can only publish before their deadline.
	err = s.avatarWrite(ctx, p, expected, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO account_avatar_objects(id,account_id,object_key,sha256,bytes,state,cleanup_after) VALUES($1,$2,$3,$4,$5,'pending',now()+interval '1 hour')`, id, p.Account.ID, key, hex.EncodeToString(sum[:]), len(data))
		return e
	})
	if err != nil {
		return Account{}, err
	}
	if err = s.avatars.Put(ctx, key, bytes.NewReader(data), int64(len(data)), "image/png"); err != nil {
		return Account{}, problem("avatar_storage_unavailable", "头像未保存，请稍后重试。", 503)
	}
	// Verify actual bytes before publishing a reference.
	if _, err = s.avatarBytes(ctx, avatarObject{Key: key, Hash: hex.EncodeToString(sum[:]), Size: int64(len(data))}); err != nil {
		return Account{}, err
	}
	var result Account
	err = s.avatarWrite(ctx, p, expected, func(tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, `UPDATE account_avatar_objects SET state='ready',cleanup_after=NULL WHERE id=$1 AND state='pending' AND cleanup_after>now()`, id)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return ErrConflict
		}
		if _, e = tx.Exec(ctx, `UPDATE account_avatar_objects SET cleanup_after=now()+interval '24 hours' WHERE id=(SELECT avatar_object_id FROM accounts WHERE id=$1)`, p.Account.ID); e != nil {
			return e
		}
		result, e = scanAccount(tx.QueryRow(ctx, `UPDATE accounts SET avatar_object_id=$2,avatar_revision=avatar_revision+1,updated_at=now() WHERE id=$1 RETURNING `+accountColumns, p.Account.ID, id))
		if e != nil {
			return e
		}
		return audit(ctx, tx, "avatar_updated", p.Account.ID, p.Account.ID, p.Account.ID, "success", m)
	})
	return result, err
}

func (s *Service) RemoveAvatar(ctx context.Context, p Principal, expected string, m Meta) (Account, error) {
	if err := avatarRevision(expected); err != nil {
		return Account{}, err
	}
	if err := s.Limit(ctx, "avatar-write", p.Account.ID, 10); err != nil {
		return Account{}, err
	}
	var result Account
	err := s.avatarWrite(ctx, p, expected, func(tx pgx.Tx) error {
		var exists bool
		if e := tx.QueryRow(ctx, `SELECT avatar_object_id IS NOT NULL FROM accounts WHERE id=$1`, p.Account.ID).Scan(&exists); e != nil {
			return e
		}
		if !exists {
			var e error
			result, e = scanAccount(tx.QueryRow(ctx, `SELECT `+accountColumns+` FROM accounts WHERE id=$1`, p.Account.ID))
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE account_avatar_objects SET cleanup_after=now()+interval '24 hours' WHERE id=(SELECT avatar_object_id FROM accounts WHERE id=$1)`, p.Account.ID); e != nil {
			return e
		}
		var e error
		result, e = scanAccount(tx.QueryRow(ctx, `UPDATE accounts SET avatar_object_id=NULL,avatar_revision=avatar_revision+1,updated_at=now() WHERE id=$1 RETURNING `+accountColumns, p.Account.ID))
		if e != nil {
			return e
		}
		return audit(ctx, tx, "avatar_removed", p.Account.ID, p.Account.ID, p.Account.ID, "success", m)
	})
	return result, err
}

func (s *Service) avatarObject(ctx context.Context, p Principal, scope AvatarScope, version string) (avatarObject, error) {
	var object avatarObject
	err := s.repo.transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if e := lockPrincipal(ctx, tx, p, s.cfg.SessionIdle, false); e != nil {
			return e
		}
		account := p.Account.ID
		switch scope.Kind {
		case "self":
		case "platform":
			if !access.ValidID(scope.ID) {
				return ErrNotFound
			}
			if e := access.RequireTx(ctx, tx, p.Account.ID, access.OpAccountRead); e != nil {
				return e
			}
			account = scope.ID
		case "tenant", "project":
			if !access.ValidID(scope.Tenant) || !access.ValidID(scope.ID) {
				return ErrNotFound
			}
			if scope.Kind == "tenant" {
				if e := access.RequireScopedTx(ctx, tx, p.Account.ID, access.OpTenantMemberDetail, scope.Tenant); e != nil {
					return e
				}
				if e := tx.QueryRow(ctx, `SELECT account_id::text FROM tenant_memberships WHERE tenant_id=$1 AND id=$2`, scope.Tenant, scope.ID).Scan(&account); e != nil {
					return e
				}
			} else {
				if !access.ValidID(scope.Project) {
					return ErrNotFound
				}
				if _, e := access.RequireProjectTx(ctx, tx, p.Account.ID, access.OpProjectMember, scope.Tenant, scope.Project); e != nil {
					return e
				}
				if e := tx.QueryRow(ctx, `SELECT account_id::text FROM project_memberships WHERE tenant_id=$1 AND project_id=$2 AND id=$3`, scope.Tenant, scope.Project, scope.ID).Scan(&account); e != nil {
					return e
				}
			}
		default:
			return access.ErrDenied
		}
		if e := tx.QueryRow(ctx, `SELECT o.id::text,o.object_key,o.sha256,o.bytes,a.avatar_revision::text FROM accounts a JOIN account_avatar_objects o ON o.id=a.avatar_object_id AND o.account_id=a.id WHERE a.id=$1 AND o.state='ready'`, account).Scan(&object.ID, &object.Key, &object.Hash, &object.Size, &object.Revision); e != nil {
			return e
		}
		if version != "" && object.Revision != version {
			return ErrNotFound
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return object, err
}

func (s *Service) avatarBytes(ctx context.Context, o avatarObject) ([]byte, error) {
	if s.avatars == nil {
		return nil, ErrUnavailable
	}
	if o.Size < 1 || o.Size > 524288 {
		return nil, ErrUnavailable
	}
	reader, err := s.avatars.Get(ctx, o.Key)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, o.Size+1))
	if err != nil {
		return nil, ErrUnavailable
	}
	sum := sha256.Sum256(data)
	if int64(len(data)) != o.Size || hex.EncodeToString(sum[:]) != o.Hash {
		return nil, ErrUnavailable
	}
	return data, nil
}

func (s *Service) ReadAvatar(ctx context.Context, p Principal, scope AvatarScope, version string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	object, err := s.avatarObject(ctx, p, scope, version)
	if err != nil {
		return nil, err
	}
	data, err := s.avatarBytes(ctx, object)
	if err != nil {
		return nil, err
	}
	// Recheck the live session, scoped permission and current reference after I/O.
	current, err := s.avatarObject(ctx, p, scope, version)
	if err != nil {
		return nil, err
	}
	if current.ID != object.ID {
		return nil, ErrNotFound
	}
	return data, nil
}

// SweepAvatars retries durable deletion intents. Only expired, unreferenced
// objects are eligible; object I/O never holds a business transaction.
func (s *Service) SweepAvatars(ctx context.Context) error {
	if s.avatars == nil {
		return nil
	}
	rows, err := s.repo.pool.Query(ctx, `UPDATE account_avatar_objects SET state='deleting' WHERE id IN (SELECT o.id FROM account_avatar_objects o WHERE o.state<>'deleted' AND o.cleanup_after<=now() AND NOT EXISTS(SELECT 1 FROM accounts a WHERE a.avatar_object_id=o.id) ORDER BY o.cleanup_after LIMIT 20 FOR UPDATE SKIP LOCKED) RETURNING id::text,object_key`)
	if err != nil {
		return err
	}
	var objects []avatarObject
	for rows.Next() {
		var o avatarObject
		if err = rows.Scan(&o.ID, &o.Key); err != nil {
			rows.Close()
			return err
		}
		objects = append(objects, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, o := range objects {
		if err = s.avatars.Delete(ctx, o.Key); err != nil {
			return err
		}
		if _, err = s.repo.pool.Exec(ctx, `UPDATE account_avatar_objects SET state='deleted' WHERE id=$1 AND state='deleting'`, o.ID); err != nil {
			return err
		}
	}
	return nil
}
