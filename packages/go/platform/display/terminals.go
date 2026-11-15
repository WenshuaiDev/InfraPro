package display

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"io/fs"
	"strconv"
	"time"
)

func (s *Service) terminalCommandTx(ctx context.Context, tx pgx.Tx, scope Scope, v Instance, auth access.ProjectAuthorization, action, target string, in Command, out map[string]any) error {
	if action == "terminal.create" {
		if e := validName(in.Name); e != nil {
			return e
		}
		if in.ValidUntil.Before(time.Now()) || in.ValidUntil.After(time.Now().Add(366*24*time.Hour)) {
			return invalid("/valid_until", "终端委托需在一年内到期。")
		}
		code := secret()
		var id string
		e := tx.QueryRow(ctx, `INSERT INTO display_terminals(tenant_id,project_id,instance_id,name,pair_hash,pair_expires_at,valid_until,environment) VALUES($1,$2,$3,$4,$5,clock_timestamp()+interval '10 minutes',$6,(SELECT generation FROM display_environment WHERE id=1)) RETURNING id::text`, scope.Tenant, scope.Project, v.ID, in.Name, hashBytes([]byte(code)), in.ValidUntil).Scan(&id)
		if e != nil {
			return e
		}
		out["terminal_id"] = id
		out["pair_code"] = code
		return nil
	}
	if !access.ValidID(target) {
		return hidden()
	}
	var terminalState string
	if e := tx.QueryRow(ctx, `SELECT state FROM display_terminals WHERE tenant_id=$1 AND project_id=$2 AND instance_id=$3 AND id=$4 FOR UPDATE`, scope.Tenant, scope.Project, v.ID, target).Scan(&terminalState); e != nil {
		return hidden()
	}
	if auth.ProjectStatus == "archived" && in.State != "revoked" {
		return access.ErrDenied
	}
	state := in.State
	code := ""
	newInstance := v.ID
	switch state {
	case "revoked":
	case "reset":
		state = "unpaired"
		code = secret()
	case "assign":
		if !access.ValidID(in.InstanceID) {
			return hidden()
		}
		next, e := instanceTx(ctx, tx, scope, in.InstanceID)
		if e != nil {
			return e
		}
		if next.State != "enabled" {
			return conflict()
		}
		newInstance = next.ID
		state = "unpaired"
		code = secret()
	default:
		return invalid("/state", "终端支持 reset、assign 或 revoked。")
	}
	_, e := tx.Exec(ctx, `UPDATE display_terminals SET state=$2,generation=generation+1,token_hash=NULL,pair_hash=$3,pair_expires_at=clock_timestamp()+interval '10 minutes',instance_id=$4,actual_release_id=NULL,environment=(SELECT generation FROM display_environment WHERE id=1) WHERE id=$1`, target, state, nullable(hashBytes([]byte(code))), newInstance)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE display_sessions SET state='revoked',lease_until=clock_timestamp() WHERE subject_id IN (SELECT id FROM display_subjects WHERE terminal_id=$1)`, target)
	if e != nil {
		return e
	}
	if state == "revoked" && terminalState != "revoked" {
		if e := eventTx(ctx, tx, scope, v.ID, auth.AccountID, "terminal.revoked", map[string]string{"terminal_id": target}); e != nil {
			return e
		}
	}
	out["terminal_id"] = target
	if code != "" {
		out["pair_code"] = code
	}
	return nil
}
func (s *Service) PairTerminal(ctx context.Context, code string) (string, error) {
	if len(code) < 32 || len(code) > 200 {
		return "", hidden()
	}
	token := secret()
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		var id, tenant, project, instance string
		e := tx.QueryRow(ctx, `SELECT id::text,tenant_id::text,project_id::text,instance_id::text FROM display_terminals WHERE pair_hash=$1 AND state='unpaired' AND pair_expires_at>clock_timestamp() AND valid_until>clock_timestamp() AND environment=(SELECT generation FROM display_environment WHERE id=1) FOR UPDATE`, hashBytes([]byte(code))).Scan(&id, &tenant, &project, &instance)
		if e != nil {
			return hidden()
		}
		v, e := instanceTx(ctx, tx, Scope{tenant, project}, instance)
		if e != nil {
			return e
		}
		var status string
		if e = tx.QueryRow(ctx, `SELECT status FROM projects WHERE id=$1`, project).Scan(&status); e != nil {
			return e
		}
		if status != "active" {
			return access.ErrDenied
		}
		var config string
		e = tx.QueryRow(ctx, `SELECT config_id::text FROM display_releases WHERE id=$1 AND NOT withdrawn`, nullable(v.Current)).Scan(&config)
		if e != nil {
			return hidden()
		}
		if _, e = s.prepareTx(ctx, tx, v.Scope, v, config); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE display_terminals SET state='active',pair_hash=NULL,pair_expires_at=NULL,token_hash=$2 WHERE id=$1`, id, hashBytes([]byte(token)))
		return e
	})
	return token, e
}
func (s *Service) StartTerminal(ctx context.Context, credential string) (string, string, string, Session, error) {
	token, asset, renewal := secret(), secret(), secret()
	var session Session
	if len(credential) < 32 || len(credential) > 200 {
		return "", "", "", session, hidden()
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		var id, tenant, project, instance, generation string
		e := tx.QueryRow(ctx, `SELECT id::text,tenant_id::text,project_id::text,instance_id::text,generation::text FROM display_terminals WHERE token_hash=$1 AND state='active' AND valid_until>clock_timestamp() AND environment=(SELECT generation FROM display_environment WHERE id=1) FOR UPDATE`, hashBytes([]byte(credential))).Scan(&id, &tenant, &project, &instance, &generation)
		if e != nil {
			return hidden()
		}
		scope := Scope{tenant, project}
		v, e := instanceTx(ctx, tx, scope, instance)
		if e != nil {
			return e
		}
		var config string
		e = tx.QueryRow(ctx, `SELECT config_id::text FROM display_releases WHERE id=$1 AND NOT withdrawn`, nullable(v.Current)).Scan(&config)
		if e != nil {
			return hidden()
		}
		prep, e := s.prepareTx(ctx, tx, scope, v, config)
		if e != nil {
			return e
		}
		session, e = s.createSessionTx(ctx, tx, scope, v, prep, "", "", id, generation, v.Current, false)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE display_sessions SET token_hash=$2,asset_hash=$3 WHERE id=$1`, session.ID, hashBytes([]byte(token)), hashBytes([]byte(asset)))
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE display_terminals SET token_hash=$2,last_seen_at=clock_timestamp() WHERE id=$1`, id, hashBytes([]byte(renewal)))
		return e
	})
	return token, asset, renewal, session, e
}
func (s *Service) Switch(ctx context.Context, v Session) (string, string, Session, error) {
	var out Session
	token, asset := secret(), secret()
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if _, _, e := s.checkSessionTx(ctx, tx, v, false); e != nil {
			return e
		}
		instance, e := instanceTx(ctx, tx, v.Scope, v.InstanceID)
		if e != nil {
			return e
		}
		if v.Preview || instance.Current == v.ReleaseID {
			return conflict()
		}
		var config string
		if e = tx.QueryRow(ctx, `SELECT config_id::text FROM display_releases WHERE id=$1 AND NOT withdrawn`, nullable(instance.Current)).Scan(&config); e != nil {
			return hidden()
		}
		prep, e := s.prepareTx(ctx, tx, v.Scope, instance, config)
		if e != nil {
			return e
		}
		out, e = s.createSessionTx(ctx, tx, v.Scope, instance, prep, v.AccountID, v.PersonSession, v.TerminalID, v.TerminalGeneration, instance.Current, false)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE display_sessions SET token_hash=$2,asset_hash=$3 WHERE id=$1`, out.ID, hashBytes([]byte(token)), hashBytes([]byte(asset)))
		return e
	})
	return token, asset, out, e
}
func (s *Service) Asset(ctx context.Context, permit, path string) (File, []byte, error) {
	var file File
	var v Session
	var key string
	if len(permit) > 200 || !SafePath(path) {
		return file, nil, hidden()
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		var e error
		v, e = scanSession(tx.QueryRow(ctx, sessionSelect+` WHERE s.asset_hash=$1`, hashBytes([]byte(permit))))
		if e != nil {
			return e
		}
		if _, _, e = s.checkSessionTx(ctx, tx, v, false); e != nil {
			return e
		}
		p, e := packageTx(ctx, tx, v.InstanceID, v.PackageID)
		if e != nil {
			return e
		}
		requested := path
		if p.Manifest.Routing.Mode == "history" {
			for _, fallback := range p.Manifest.Routing.Fallback {
				if path == fallback {
					requested = p.Manifest.Entry
				}
			}
		}
		e = tx.QueryRow(ctx, `SELECT path,size_bytes,media_type,sha256,object_key FROM display_package_files WHERE package_id=$1 AND path=$2`, v.PackageID, requested).Scan(&file.Path, &file.Size, &file.MediaType, &file.SHA256, &key)
		if e == pgx.ErrNoRows {
			return hidden()
		}
		return e
	})
	if e != nil {
		return file, nil, e
	}
	if s.blobs == nil {
		return file, nil, problem("dependency_unavailable", "程序包存储不可用。", 503)
	}
	r, e := s.blobs.Get(ctx, key)
	if e != nil {
		if errors.Is(e, fs.ErrNotExist) {
			s.markPackageMissing(ctx, v, "file_missing")
		}
		return file, nil, problem("package_unavailable", "程序包存储不可用或文件缺失。", 503)
	}
	defer r.Close()
	b, e := readBounded(r, file.Size)
	if e != nil || hashBytes(b) != file.SHA256 {
		if e == nil {
			s.markPackageMissing(ctx, v, "hash_mismatch")
		}
		return file, nil, problem("package_unavailable", "程序包完整性校验失败。", 503)
	}
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		current, e := scanSession(tx.QueryRow(ctx, sessionSelect+` WHERE s.id=$1`, v.ID))
		if e != nil {
			return e
		}
		_, _, e = s.checkSessionTx(ctx, tx, current, false)
		return e
	})
	return file, b, e
}
func (s *Service) IssueAsset(ctx context.Context, v Session) (string, error) {
	asset := secret()
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if _, _, e := s.checkSessionTx(ctx, tx, v, false); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE display_sessions SET asset_hash=$2 WHERE id=$1`, v.ID, hashBytes([]byte(asset)))
		return e
	})
	return asset, e
}

var _ = strconv.Itoa

func (s *Service) markPackageMissing(ctx context.Context, v Session, code string) {
	_ = s.access.Transact(ctx, func(tx pgx.Tx) error {
		return PackageAvailabilityTx(ctx, tx, v.PackageID, true)
	})
	s.RecordDiagnostic(ctx, v, "package_unavailable", code)
}
