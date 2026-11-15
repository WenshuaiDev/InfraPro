package devices

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

func (s *Service) AuditMetadataTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, kind, id string) (string, error) {
	if kind == "device_output" {
		out, e := s.ResolveOutputTx(ctx, tx, a, scope, id, false)
		return out.Name, e
	}
	if kind == "project_device_grant_set" {
		if _, e := k.AuthorizeTx(ctx, tx, a, scope, scope.Operation("access", "detail")); e != nil {
			return "", e
		}
		return "设备授权", nil
	}
	if kind == "device_installation" {
		var device string
		if e := tx.QueryRow(ctx, `SELECT device_id::text FROM device_installations WHERE tenant_id=$1 AND id::text=$2`, scope.Tenant, id).Scan(&device); e != nil {
			return "", e
		}
		id = device
	} else if kind != "device" {
		return "", access.ErrDenied
	}
	auth, e := k.AuthorizeTx(ctx, tx, a, scope, scope.Operation("device", "detail"))
	if e != nil {
		return "", e
	}
	if e = projectReadable(ctx, tx, auth, "device", id, false); e != nil {
		return "", e
	}
	v, e := loadDevice(ctx, tx, scope.Tenant, id, false)
	return v.Name, e
}

// Resolve only the current, independently authorized parent; a transport never
// guesses ownership from event labels or an untrusted route parameter.
func (s *Service) AuditLocationTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, kind, id string) (map[string]string, error) {
	if _, err := s.AuditMetadataTx(ctx, tx, a, scope, kind, id); err != nil {
		return nil, err
	}
	var device string
	var err error
	switch kind {
	case "device_output":
		err = tx.QueryRow(ctx, `SELECT device_id::text FROM device_outputs WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id).Scan(&device)
	case "device_installation":
		err = tx.QueryRow(ctx, `SELECT device_id::text FROM device_installations WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id).Scan(&device)
	default:
		return nil, access.ErrDenied
	}
	return map[string]string{"device": device}, err
}
