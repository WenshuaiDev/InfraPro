package display

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"slices"
	"time"
)

type Preparation struct {
	ReadViews             []ReadViewDescription `json:"read_views,omitempty"`
	Hash                  string                `json:"hash"`
	Package               Package               `json:"package"`
	Config                Config                `json:"config"`
	GrantRevision         string                `json:"grant_revision"`
	BindingSets           map[string][]string   `json:"binding_sets"`
	AuthorizationRevision string                `json:"authorization_revision"`
}

func (s *Service) prepareTx(ctx context.Context, tx pgx.Tx, scope Scope, v Instance, configID string) (Preparation, error) {
	out := Preparation{BindingSets: map[string][]string{}}
	st, e := settingsTx(ctx, tx, scope)
	if e != nil {
		return out, e
	}
	if !st.Enabled || v.State != "enabled" {
		return out, problem("display_disabled", "项目或实例未启用。", 409)
	}
	out.Config, e = configTx(ctx, tx, v.ID, configID)
	if e != nil {
		return out, e
	}
	out.Package, e = packageTx(ctx, tx, v.ID, out.Config.PackageID)
	if e != nil {
		return out, e
	}
	g, e := grantTx(ctx, tx, v.ID)
	if e != nil || g.Revoked || g.ValidUntil.Before(time.Now()) {
		return out, problem("grant_required", "需要当前有效的大屏读取授权。", 409)
	}
	out.GrantRevision = g.Revision
	for name, version := range out.Package.Manifest.Required {
		cap := s.descriptors[name]
		if cap.Availability != "available" || s.providers[name] == nil {
			return out, problem("capability_unavailable", "必需能力当前不可用："+name, 409)
		}
		if version != cap.Version {
			return out, problem("version_incompatible", "必需能力版本不兼容。", 409)
		}
		if !slices.Contains(g.Capabilities, name) {
			return out, problem("grant_required", "必需能力尚未获准："+name, 409)
		}
	}
	if e = ValidateConfig(out.Package.Manifest, out.Config.Parameters); e != nil {
		return out, e
	}
	if e = s.validateBindingsTx(ctx, tx, scope, out.Package.Manifest, out.Config.Bindings, &g, true); e != nil {
		return out, e
	}
	// Delegation cannot outlive the issuer's current project authority. Logging
	// out is deliberately unrelated to this check.
	issuer, e := access.ProjectSnapshotTx(ctx, tx, g.IssuedBy, scope.Tenant, scope.Project, "")
	if e != nil {
		return out, hidden()
	}
	if !slices.Contains(issuer.Permissions, access.ProjectDisplayGrant) {
		return out, hidden()
	}
	for _, p := range s.grantPermissions(g) {
		if !slices.Contains(issuer.Permissions, p) {
			return out, hidden()
		}
	}
	out.ReadViews, e = s.validateReadViewsTx(ctx, tx, scope, v, out.Package, out.Config, g, access.Actor{ID: g.IssuedBy}, true)
	if e != nil {
		return out, e
	}
	for name, b := range out.Config.Bindings {
		ids, e := effectiveBindingIDsTx(ctx, tx, scope, b, &g)
		if e != nil {
			return out, e
		}
		out.BindingSets[name] = ids
	}
	if s.qualifyBindings != nil {
		// This proof-only subject cannot create runtime-owned snapshots: it never
		// leaves this transaction and receives no session, cookie or data payload.
		sub := &access.ReadSubject{ID: v.ID, Tenant: scope.Tenant, Project: scope.Project}
		sub.Check = func(context.Context, pgx.Tx) error { return nil }
		sub.Authorize = func(c context.Context, t pgx.Tx, op access.Operation) (access.ProjectAuthorization, error) {
			auth, e := access.RequireProjectTx(c, t, g.IssuedBy, op, scope.Tenant, scope.Project)
			allowed := s.grantPermissions(g)
			auth.Permissions = slices.DeleteFunc(slices.Clone(auth.Permissions), func(p access.Code) bool { return !slices.Contains(allowed, p) })
			return auth, e
		}
		sub.Windows = func(c context.Context, t pgx.Tx, channels []string) (map[string][]access.ReadWindow, error) {
			ids, e := grantIDsTx(c, t, scope, g, "channels")
			if e != nil {
				return nil, e
			}
			w, e := readWindow(g.Scope.From, g.Scope.To)
			if e != nil {
				return nil, e
			}
			out := map[string][]access.ReadWindow{}
			for _, id := range channels {
				if !slices.Contains(ids, id) {
					return nil, hidden()
				}
				out[id] = []access.ReadWindow{{From: w.From, To: w.To}}
			}
			return out, nil
		}
		resolved := map[string]Binding{}
		for name, b := range out.Config.Bindings {
			b.IDs = out.BindingSets[name]
			b.Dynamic = nil
			resolved[name] = b
		}
		if e = s.qualifyBindings(ctx, tx, sub, resolved); e != nil {
			return out, e
		}
	}
	rev, e := access.Lock(ctx, tx, false)
	if e != nil {
		return out, e
	}
	var resource, data string
	e = tx.QueryRow(ctx, `SELECT r.revision::text,COALESCE(m.revision,0)::text FROM project_resource_policies r LEFT JOIN monitoring_access_states m USING(tenant_id,project_id) WHERE r.tenant_id=$1 AND r.project_id=$2`, scope.Tenant, scope.Project).Scan(&resource, &data)
	if e != nil {
		return out, e
	}
	out.AuthorizationRevision = rev + ":" + resource + ":" + data
	var readRevision string
	if e = tx.QueryRow(ctx, `SELECT md5(COALESCE((SELECT string_agg(domain||':'||revision::text,',' ORDER BY domain) FROM display_read_revisions WHERE tenant_id=$1),'')||':'||COALESCE((SELECT revision::text FROM project_device_access_states WHERE tenant_id=$1 AND project_id=$2),'0')||':'||COALESCE((SELECT revision::text FROM project_spatial_states WHERE tenant_id=$1 AND project_id=$2),'0'))`, scope.Tenant, scope.Project).Scan(&readRevision); e != nil {
		return out, e
	}
	out.Hash = hash([]any{v.Revision, st.Revision, out.Package.Hash, out.Config, g, out.BindingSets, out.ReadViews, out.AuthorizationRevision, readRevision})
	return out, nil
}
func (s *Service) Prepare(ctx context.Context, a access.Actor, scope Scope, id string, in Command) (Preparation, error) {
	var out Preparation
	e := s.transact(ctx, a, scope, "prepare", func(tx pgx.Tx, auth access.ProjectAuthorization) error {
		v, e := instanceTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if in.Revision != v.Revision {
			return conflict()
		}
		config := in.ConfigID
		if in.ReleaseID != "" {
			if !access.ValidID(in.ReleaseID) {
				return hidden()
			}
			e = tx.QueryRow(ctx, `SELECT config_id::text FROM display_releases WHERE instance_id=$1 AND id=$2 AND NOT withdrawn`, id, in.ReleaseID).Scan(&config)
			if e != nil {
				return hidden()
			}
		}
		out, e = s.prepareTx(ctx, tx, scope, v, config)
		if e != nil {
			return e
		}
		g, e := grantTx(ctx, tx, id)
		if e != nil {
			return e
		}
		// Publish proof covers the delegated release. Preview counts additionally
		// intersect the current reviewing person's business authority.
		out.ReadViews, e = s.validateReadViewsTx(ctx, tx, scope, v, out.Package, out.Config, g, a, false)
		return e
	})
	return out, e
}
func (s *Service) publishTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, v Instance, action string, in Command, out map[string]any) error {
	config := in.ConfigID
	if action == "rollback" {
		if !access.ValidID(in.ReleaseID) {
			return hidden()
		}
		if e := tx.QueryRow(ctx, `SELECT config_id::text FROM display_releases WHERE instance_id=$1 AND id=$2 AND NOT withdrawn`, v.ID, in.ReleaseID).Scan(&config); e != nil {
			return hidden()
		}
	}
	prep, e := s.prepareTx(ctx, tx, scope, v, config)
	if e != nil {
		return e
	}
	if prep.Hash != in.Preparation {
		return conflict()
	}
	var id string
	e = tx.QueryRow(ctx, `INSERT INTO display_releases(tenant_id,project_id,instance_id,package_id,config_id,requirements,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`, scope.Tenant, scope.Project, v.ID, prep.Package.ID, prep.Config.ID, jsonBytes(map[string]any{"capabilities": prep.Package.Manifest.Required, "bindings": prep.BindingSets, "read_views": prep.Config.ReadViews, "config_format": prep.Config.Format, "preparation": prep.Hash}), a.ID).Scan(&id)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE display_instances SET current_release_id=$2 WHERE id=$1`, v.ID, id)
	out["release_id"] = id
	return e
}
