package access

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool       *pgxpool.Pool
	logger     *slog.Logger
	logMu      sync.Mutex
	lastDenial time.Time
}

func New(pool *pgxpool.Pool, logger *slog.Logger) *Service {
	return &Service{pool: pool, logger: logger}
}

// Transact owns one bounded transaction; callbacks never commit independently.
func (s *Service) Transact(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(ctx, "SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='10s'"); e != nil {
		return e
	}
	if e = fn(tx); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func Lock(ctx context.Context, tx pgx.Tx, exclusive bool) (string, error) {
	mode := " FOR SHARE"
	if exclusive {
		mode = " FOR UPDATE"
	}
	var revision, checksum string
	e := tx.QueryRow(ctx, "SELECT revision::text,catalog_checksum FROM authorization_state WHERE id=1"+mode).Scan(&revision, &checksum)
	if e != nil {
		return "", e
	}
	if checksum != Checksum() {
		return "", ErrCatalog
	}
	return revision, nil
}
func effective(ctx context.Context, tx pgx.Tx, id string, configured bool) ([]Code, error) {
	rows, e := tx.Query(ctx, `SELECT DISTINCT rp.permission_code FROM accounts a JOIN account_roles ar ON ar.account_id=a.id JOIN roles r ON r.id=ar.role_id JOIN role_permissions rp ON rp.role_id=r.id JOIN permission_catalog p ON p.code=rp.permission_code WHERE a.id=$1 AND r.deleted_at IS NULL AND p.lifecycle='active' AND ($2 OR (a.status='active' AND r.status='active')) ORDER BY rp.permission_code`, id, configured)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	codes := []Code{}
	for rows.Next() {
		var c Code
		if e = rows.Scan(&c); e != nil {
			return nil, e
		}
		if known(c) {
			codes = append(codes, c)
		}
	}
	return codes, rows.Err()
}
func RequireTx(ctx context.Context, tx pgx.Tx, id string, op Operation) error {
	policy, ok := PolicyFor(op)
	if !ok || (policy.Class != "permission" && policy.Class != "permission_any") || len(policy.Permissions) == 0 {
		return ErrDenied
	}
	have, e := effective(ctx, tx, id, false)
	if e != nil {
		return e
	}
	if policy.Class == "permission_any" {
		for _, p := range policy.Permissions {
			if slices.Contains(have, p) {
				return nil
			}
		}
		return ErrDenied
	}
	for _, p := range policy.Permissions {
		if !slices.Contains(have, p) {
			return ErrDenied
		}
	}
	return nil
}

// Require acquires the shared authorization lock before callers lock accounts/credentials.
func Require(ctx context.Context, tx pgx.Tx, id string, op Operation) error {
	if _, e := Lock(ctx, tx, false); e != nil {
		return e
	}
	return RequireTx(ctx, tx, id, op)
}
func HasTx(ctx context.Context, tx pgx.Tx, id string, code Code) (bool, error) {
	v, e := effective(ctx, tx, id, false)
	return slices.Contains(v, code), e
}
func Bump(ctx context.Context, tx pgx.Tx) error {
	_, e := tx.Exec(ctx, "UPDATE authorization_state SET revision=revision+1 WHERE id=1")
	return e
}
func (s *Service) Authorize(ctx context.Context, id string, op Operation) (string, error) {
	var rev string
	e := s.Transact(ctx, func(tx pgx.Tx) error {
		var e error
		rev, e = Lock(ctx, tx, false)
		if e != nil {
			return e
		}
		return RequireTx(ctx, tx, id, op)
	})
	if denied(e) {
		s.LogDenial(op, id, e)
	}
	return rev, e
}
func (s *Service) LogDenial(op Operation, id string, e error) {
	if !denied(e) {
		return
	}
	s.logMu.Lock()
	defer s.logMu.Unlock()
	if time.Since(s.lastDenial) < time.Second {
		return
	}
	s.lastDenial = time.Now()
	s.logger.Warn("authorization denied", "operation", op, "actor_id", id, "reason", e.Error())
}
func snapshotTx(ctx context.Context, tx pgx.Tx, id, revision string) (Snapshot, error) {
	v := Snapshot{Revision: revision, Permissions: []Code{}, Roles: []Role{}, Sources: map[Code][]string{}}
	rows, e := tx.Query(ctx, "SELECT r.id::text FROM account_roles ar JOIN roles r ON r.id=ar.role_id WHERE ar.account_id=$1 AND r.deleted_at IS NULL ORDER BY r.id", id)
	if e != nil {
		return v, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return v, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return v, e
	}
	v.Permissions, e = effective(ctx, tx, id, false)
	if e != nil {
		return v, e
	}
	for _, rid := range ids {
		r, e := roleTx(ctx, tx, rid)
		if e != nil {
			return v, e
		}
		v.Roles = append(v.Roles, r)
		if r.Status == "active" {
			for _, p := range r.Permissions {
				if slices.Contains(v.Permissions, p) {
					v.Sources[p] = append(v.Sources[p], r.ID)
				}
			}
		}
	}
	return v, nil
}
func (s *Service) Snapshot(ctx context.Context, id string) (Snapshot, error) {
	var v Snapshot
	e := s.Transact(ctx, func(tx pgx.Tx) error {
		rev, e := Lock(ctx, tx, false)
		if e != nil {
			return e
		}
		v, e = snapshotTx(ctx, tx, id, rev)
		return e
	})
	return v, e
}
func EnsureManager(ctx context.Context, tx pgx.Tx) error {
	var initialized bool
	if e := tx.QueryRow(ctx, "SELECT completed_at IS NOT NULL FROM system_initialization WHERE id=1").Scan(&initialized); e != nil {
		return e
	}
	if !initialized {
		return nil
	}
	required := []string{}
	for _, p := range ManagementCodes() {
		required = append(required, string(p))
	}
	var exists bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accounts a WHERE a.status='active' AND NOT EXISTS(SELECT 1 FROM unnest($1::text[]) AS required(code) WHERE NOT EXISTS(SELECT 1 FROM account_roles ar JOIN roles r ON r.id=ar.role_id JOIN role_permissions rp ON rp.role_id=r.id JOIN permission_catalog p ON p.code=rp.permission_code WHERE ar.account_id=a.id AND r.status='active' AND r.deleted_at IS NULL AND p.lifecycle='active' AND rp.permission_code=required.code)))`, required).Scan(&exists)
	if e != nil {
		return e
	}
	if !exists {
		return ErrLastManager
	}
	return nil
}
func InvitationHolders(ctx context.Context, tx pgx.Tx) ([]string, error) {
	rows, e := tx.Query(ctx, `SELECT DISTINCT a.id::text FROM accounts a JOIN account_roles ar ON ar.account_id=a.id JOIN roles r ON r.id=ar.role_id JOIN role_permissions rp ON rp.role_id=r.id JOIN permission_catalog p ON p.code=rp.permission_code WHERE a.status='active' AND r.status='active' AND r.deleted_at IS NULL AND p.lifecycle='active' AND rp.permission_code=$1`, InvitationCreate)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		v = append(v, id)
	}
	return v, rows.Err()
}
func RevokeLostInvitations(ctx context.Context, tx pgx.Tx, before []string) error {
	after, e := InvitationHolders(ctx, tx)
	if e != nil {
		return e
	}
	for _, id := range before {
		if !slices.Contains(after, id) {
			if _, e = tx.Exec(ctx, "UPDATE account_invitations SET revoked_at=now() WHERE inviter_id=$1 AND used_at IS NULL AND revoked_at IS NULL", id); e != nil {
				return e
			}
		}
	}
	return nil
}
func checkVersion(have, want string) error {
	n, e := strconv.ParseInt(want, 10, 64)
	if e != nil || n < 1 || strconv.FormatInt(n, 10) != want {
		return Validation("expected_version", "请提交有效的版本号。")
	}
	if have != want {
		return ErrConflict
	}
	return nil
}
func notFound(e error) error {
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return e
}

// Transaction reads are for cooperating modules that already hold the authorization lock.
func PermissionsTx(ctx context.Context, tx pgx.Tx, id string) ([]Code, error) {
	return effective(ctx, tx, id, false)
}
func SnapshotTx(ctx context.Context, tx pgx.Tx, id, revision string) (Snapshot, error) {
	return snapshotTx(ctx, tx, id, revision)
}
