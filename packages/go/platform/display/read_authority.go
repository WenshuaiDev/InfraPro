package display

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

func newReadCall(call Call) bool {
	if slices.Contains([]string{"classification", "views", "queries", "devices", "map", "statistics"}, strings.Split(call.Method, ".")[0]) || slices.Contains([]string{"resources.types", "resources.query", "resources.tree", "resources.hierarchy", "resources.detail", "sources.bindings", "sources.bindingHistory", "spatial.assets", "spatial.versions", "spatial.componentLinks", "spatial.locations", "spatial.settings"}, call.Method) {
		return true
	}
	var raw struct {
		Target map[string]json.RawMessage `json:"target"`
	}
	if json.Unmarshal(call.Params, &raw) != nil {
		return false
	}
	_, view := raw.Target["view"]
	_, selection := raw.Target["selection"]
	return view || selection
}
func (s *Service) readEvidenceTx(ctx context.Context, tx pgx.Tx, v Session) (displayread.Evidence, error) {
	current, e := scanSession(tx.QueryRow(ctx, sessionSelect+` WHERE s.id=$1`, v.ID))
	if e != nil {
		return displayread.Evidence{}, e
	}
	g, _, e := s.checkSessionTx(ctx, tx, current, false)
	if e != nil {
		return displayread.Evidence{}, e
	}
	vector, e := s.sourceVectorTx(ctx, tx, current, []string{})
	if e != nil {
		return displayread.Evidence{}, e
	}
	var classification, derived, deviceGrants, spatialGrants string
	e = tx.QueryRow(ctx, `SELECT classification.revision::text,
        (SELECT coalesce(md5(string_agg(channel_id::text||':'||head_revision::text, ',' ORDER BY channel_id)), '0')
           FROM derived_channel_heads WHERE tenant_id=$1),
        coalesce((SELECT revision::text FROM project_device_access_states WHERE tenant_id=$1 AND project_id=$2), '0'),
        coalesce((SELECT revision::text FROM project_spatial_states WHERE tenant_id=$1 AND project_id=$2), '0')
      FROM device_classification_state classification WHERE classification.id=1`, current.Tenant, current.Project).
		Scan(&classification, &derived, &deviceGrants, &spatialGrants)
	if e != nil {
		return displayread.Evidence{}, e
	}
	vector["classification"], vector["derived"] = classification, derived
	vector["device_grants"], vector["spatial_grants"] = deviceGrants, spatialGrants
	authority, safety, e := readAuthorityStateTx(ctx, tx, current, g, vector)
	return displayread.Evidence{Authorization: authority, Vector: vector, Safety: safety, LeaseUntil: current.LeaseUntil}, e
}

// A saved read and every peer trust boundary use the same authority hash and
// the same safety revisions. The owner also retains the full source vector for
// query consistency and invalidation.
func readAuthorityStateTx(ctx context.Context, tx pgx.Tx, current Session, g Grant, vector map[string]string) (string, string, error) {
	rows, e := tx.Query(ctx, `SELECT domain,revision::text,safety_revision::text FROM display_read_revisions WHERE tenant_id=$1 ORDER BY domain`, current.Tenant)
	if e != nil {
		return "", "", e
	}
	defer rows.Close()
	safety := map[string]string{}
	for rows.Next() {
		var domain, revision, revoked string
		if e = rows.Scan(&domain, &revision, &revoked); e != nil {
			return "", "", e
		}
		vector[domain] = revision
		safety[domain] = revoked
	}
	if e = rows.Err(); e != nil {
		return "", "", e
	}
	authority := hash([]any{current.SubjectID, current.Environment, current.ConfigID, scopeFingerprint(g), vector["authorization"], vector["data_grants"], vector["resource_grants"], vector["device_grants"]})
	return authority, hash(safety), nil
}
func (s *Service) ReadAuthority(v Session) displayread.Authority {
	a := displayread.Authority{Owner: displayread.Owner{Tenant: v.Tenant, Project: v.Project, Instance: v.InstanceID, Subject: v.SubjectID, Session: v.ID, Config: v.ConfigID, Environment: v.Environment}, Subject: s.subject(v)}
	a.Subject.Windows = func(ctx context.Context, tx pgx.Tx, ids []string) (map[string][]access.ReadWindow, error) {
		return s.readWindowsTx(ctx, tx, v, ids, false)
	}
	a.Subject.HistoryWindows = func(ctx context.Context, tx pgx.Tx, ids []string) (map[string][]access.ReadWindow, error) {
		return s.readWindowsTx(ctx, tx, v, ids, true)
	}
	a.Check = func(ctx context.Context, tx pgx.Tx) (displayread.Evidence, error) {
		return s.readEvidenceTx(ctx, tx, v)
	}
	a.Require = func(ctx context.Context, tx pgx.Tx, method string) error {
		capName, ok := methodCapabilities[method]
		if !ok {
			return invalid("/method", "未知读取方法。")
		}
		g, _, e := s.checkSessionTx(ctx, tx, v, false)
		if e != nil {
			return e
		}
		p, e := packageTx(ctx, tx, v.InstanceID, v.PackageID)
		if e != nil {
			return e
		}
		if !slices.Contains(g.Capabilities, capName) || p.Manifest.Required[capName] != 1 && p.Manifest.Optional[capName] != 1 {
			return hidden()
		}
		if s.descriptors[capName].Availability != "available" || s.providers[capName] == nil {
			return problem("capability_unavailable", "读取能力当前不可用。", 409)
		}
		person := g.IssuedBy
		if v.AccountID != "" {
			person = v.AccountID
		}
		auth, e := access.ProjectSnapshotTx(ctx, tx, person, v.Tenant, v.Project, "")
		if e != nil {
			return e
		}
		for _, permission := range s.descriptors[capName].Permissions {
			if !slices.Contains(auth.Permissions, permission) {
				return hidden()
			}
		}
		return nil
	}
	a.Resolve = func(ctx context.Context, tx pgx.Tx, target displayread.Target, domain string, requested, minimum []string) (displayread.Scope, error) {
		out := displayread.Scope{Domain: domain, Members: []displayread.Member{}}
		if target.View == "" || target.Selection != "" {
			return out, invalid("/target/view", "需要命名读取视图。")
		}
		if e := a.Require(ctx, tx, "views.describe"); e != nil {
			return out, e
		}
		g, c, e := s.checkSessionTx(ctx, tx, v, false)
		if e != nil {
			return out, e
		}
		d, ok := c.ReadViews[target.View]
		if !ok || d.Domain != domain {
			return out, problem("scope_unavailable", "读取视图不存在或领域不匹配。", 409)
		}
		parent, ok := findReadScope(g, d.ScopeID)
		if !ok || parent.Domain != domain {
			return out, problem("scope_unavailable", "读取范围已移除。", 409)
		}
		available := []string{}
		for _, f := range d.FieldGroups {
			if slices.Contains(parent.FieldGroups, f) {
				available = append(available, f)
			}
		}
		if displayread.StructureVersion(d.StructureModel) > displayread.StructureVersion(parent.StructureModel) {
			return out, displayread.StructureUpgradeRequired()
		}
		out.StructureModel = displayread.StructureVersion(d.StructureModel)
		out.FieldGroups, e = displayread.Fields(available, requested, minimum)
		if e != nil {
			return out, e
		}
		issuer := access.Actor{ID: g.IssuedBy}
		members, e := s.effectiveReadScopeTx(ctx, tx, issuer, v.Scope, v.InstanceID, d)
		if e != nil {
			return out, e
		}
		allowed, e := s.effectiveReadScopeTx(ctx, tx, issuer, v.Scope, v.InstanceID, parent)
		if e != nil {
			return out, e
		}
		members = displayread.IntersectMembers(members, allowed)
		if v.AccountID != "" && v.AccountID != g.IssuedBy {
			current, e := s.resolveReadMembersTx(ctx, tx, access.Actor{ID: v.AccountID}, v.Scope, d)
			if e != nil {
				return out, e
			}
			members = displayread.IntersectMembers(members, current)
		}
		if target.IDs != nil {
			wanted := map[string]bool{}
			for _, id := range target.IDs {
				wanted[id] = true
			}
			found := map[string]bool{}
			out.Members = []displayread.Member{}
			for _, m := range members {
				if wanted[m.ID] {
					out.Members = append(out.Members, m)
					found[m.ID] = true
				}
			}
			if len(found) != len(wanted) {
				return out, hidden()
			}
		} else {
			out.Members = members
		}
		if domain == "channels" {
			x, e := displayread.ParseWindow(d.TimeWindow, "/time_window")
			if e != nil {
				return out, e
			}
			y, e := displayread.ParseWindow(parent.TimeWindow, "/time_window")
			if e != nil {
				return out, e
			}
			w, ok := displayread.Intersect(x, y)
			out.Windows = map[string][]access.ReadWindow{}
			if !ok {
				out.Members = []displayread.Member{}
			} else {
				for _, m := range out.Members {
					out.Windows[m.ID] = []access.ReadWindow{w}
				}
			}
		}
		out.Hash = hash([]any{d, parent, out.Members, out.FieldGroups, out.Windows})
		return out, nil
	}
	a.Views = func(ctx context.Context, tx pgx.Tx, name string) (any, error) {
		g, c, e := s.checkSessionTx(ctx, tx, v, false)
		if e != nil {
			return nil, e
		}
		p, e := packageTx(ctx, tx, v.InstanceID, v.PackageID)
		if e != nil {
			return nil, e
		}
		person := g.IssuedBy
		if v.AccountID != "" {
			person = v.AccountID
		}
		items, e := s.validateReadViewsTx(ctx, tx, v.Scope, Instance{ID: v.InstanceID}, p, c, g, access.Actor{ID: person}, false)
		if e != nil {
			return nil, e
		}
		if name != "" {
			for _, item := range items {
				if item.Name == name {
					return item, nil
				}
			}
			return nil, problem("scope_unavailable", "读取视图不存在。", 409)
		}
		return map[string]any{"items": items}, nil
	}
	return a
}

// Qualification of recursive inputs uses current delegated views, but never
// exposes those inputs as members of a caller's output selection.
func (s *Service) readWindowsTx(ctx context.Context, tx pgx.Tx, v Session, ids []string, history bool) (map[string][]access.ReadWindow, error) {
	// Input proof trees revisit the same channels many times. All delegation
	// changes take the exclusive authorization lock; reuse this read projection
	// only within its shared-lock transaction. The next page/request rechecks it.
	if _, err := access.Lock(ctx, tx, false); err != nil {
		return nil, err
	}
	normalized := slices.Clone(ids)
	slices.Sort(normalized)
	normalized = slices.Compact(normalized)
	type key struct {
		service           *Service
		session, channels string
		history           bool
	}
	windows, err := access.MemoTx(tx, key{s, v.ID, hash(normalized), history}, func() (map[string][]access.ReadWindow, error) {
		return s.loadReadWindowsTx(ctx, tx, v, normalized, history)
	})
	if err != nil {
		return nil, err
	}
	// Domain readers may intersect their returned windows. Never expose the
	// memo's map, slices or endpoint pointers to those callers.
	out := make(map[string][]access.ReadWindow, len(windows))
	for id, values := range windows {
		out[id] = slices.Clone(values)
		for i, window := range values {
			if window.From != nil {
				from := *window.From
				out[id][i].From = &from
			}
			if window.To != nil {
				to := *window.To
				out[id][i].To = &to
			}
		}
	}
	return out, nil
}

func (s *Service) loadReadWindowsTx(ctx context.Context, tx pgx.Tx, v Session, ids []string, history bool) (map[string][]access.ReadWindow, error) {
	out := map[string][]access.ReadWindow{}
	g, c, e := s.checkSessionTx(ctx, tx, v, false)
	if e != nil {
		return nil, e
	}
	// Recursive qualification asks for particular inputs. Narrow predicates
	// before qualifying them, so an unrelated derived view is not recursively
	// revalidated for every raw input. Fixed-set and current grants still apply.
	narrow := func(d displayread.Definition) (displayread.Definition, bool) {
		selected := slices.Clone(ids)
		if len(d.Selector.IDs) > 0 {
			selected = slices.DeleteFunc(selected, func(id string) bool { return !slices.Contains(d.Selector.IDs, id) })
		}
		d.Selector.IDs = selected
		return d, len(selected) > 0
	}
	for _, d := range c.ReadViews {
		if d.Domain != "channels" {
			continue
		}
		parent, ok := findReadScope(g, d.ScopeID)
		if !ok {
			continue
		}
		if history && (!slices.Contains(d.FieldGroups, "history") || !slices.Contains(parent.FieldGroups, "history")) {
			continue
		}
		var selected bool
		d, selected = narrow(d)
		if !selected {
			continue
		}
		parent, selected = narrow(parent)
		if !selected {
			continue
		}
		members, e := s.effectiveReadScopeTx(ctx, tx, access.Actor{ID: g.IssuedBy}, v.Scope, v.InstanceID, d)
		if e != nil {
			return nil, e
		}
		allowed, e := s.effectiveReadScopeTx(ctx, tx, access.Actor{ID: g.IssuedBy}, v.Scope, v.InstanceID, parent)
		if e != nil {
			return nil, e
		}
		members = displayread.IntersectMembers(members, allowed)
		if v.AccountID != "" && v.AccountID != g.IssuedBy {
			current, e := s.resolveReadMembersTx(ctx, tx, access.Actor{ID: v.AccountID}, v.Scope, d)
			if e != nil {
				return nil, e
			}
			members = displayread.IntersectMembers(members, current)
		}
		x, e := displayread.ParseWindow(d.TimeWindow, "/time_window")
		if e != nil {
			return nil, e
		}
		y, e := displayread.ParseWindow(parent.TimeWindow, "/time_window")
		if e != nil {
			return nil, e
		}
		w, ok := displayread.Intersect(x, y)
		if !ok {
			continue
		}
		for _, m := range members {
			if slices.Contains(ids, m.ID) {
				out[m.ID] = append(out[m.ID], w)
			}
		}
	}
	// Mixed manifests retain their legacy binding contract. These windows
	// qualify recursive inputs only; they do not add output view members or
	// confer the new history capability.
	if !history {
		allowed, e := grantIDsTx(ctx, tx, v.Scope, g, "channels")
		if e != nil {
			return nil, e
		}
		ceiling, e := readWindow(g.Scope.From, g.Scope.To)
		if e != nil {
			return nil, e
		}
		for _, binding := range c.Bindings {
			if binding.Kind != "channels" {
				continue
			}
			members, e := effectiveBindingIDsTx(ctx, tx, v.Scope, binding, &g)
			if e != nil {
				return nil, e
			}
			window, e := readWindow(binding.From, binding.To)
			if e != nil {
				return nil, e
			}
			w, ok := displayread.Intersect(access.ReadWindow{From: window.From, To: window.To}, access.ReadWindow{From: ceiling.From, To: ceiling.To})
			if !ok {
				continue
			}
			for _, id := range ids {
				if slices.Contains(members, id) && slices.Contains(allowed, id) {
					out[id] = append(out[id], w)
				}
			}
		}
	}
	for _, id := range ids {
		if len(out[id]) == 0 {
			return nil, hidden()
		}
		out[id] = displayread.Union(out[id])
	}
	return out, nil
}
