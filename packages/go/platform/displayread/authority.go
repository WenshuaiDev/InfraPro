package displayread

import (
	"context"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

type Owner struct {
	Tenant      string `json:"tenant_id"`
	Project     string `json:"project_id"`
	Instance    string `json:"instance_id"`
	Subject     string `json:"subject_id"`
	Session     string `json:"session_id"`
	Config      string `json:"config_id"`
	Environment string `json:"environment"`
}
type Evidence struct {
	Authorization string            `json:"authorization_revision"`
	Vector        map[string]string `json:"source_vector"`
	Safety        string            `json:"safety"`
	LeaseUntil    time.Time         `json:"lease_until"`
}

// Authority is created only by the verified runtime. Its callbacks reload the
// current session and delegation; a saved result is never an authorization.
type Authority struct {
	Owner   Owner
	Subject *access.ReadSubject
	Check   func(context.Context, pgx.Tx) (Evidence, error)
	Require func(context.Context, pgx.Tx, string) error
	Resolve func(context.Context, pgx.Tx, Target, string, []string, []string) (Scope, error)
	Views   func(context.Context, pgx.Tx, string) (any, error)
}

func (a Authority) Validate(ctx context.Context, tx pgx.Tx, method string, old *Evidence) (Evidence, error) {
	if a.Check == nil || a.Require == nil || a.Resolve == nil || a.Subject == nil {
		return Evidence{}, Hidden()
	}
	if e := a.Require(ctx, tx, method); e != nil {
		return Evidence{}, e
	}
	v, e := a.Check(ctx, tx)
	if e != nil {
		return v, e
	}
	if old != nil && (old.Safety != v.Safety || old.Authorization != v.Authorization) {
		return v, Problem("reset_required", "读取依据或授权已失效，请重建查询。", 409)
	}
	return v, nil
}
