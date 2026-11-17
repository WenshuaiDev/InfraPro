package display

import (
	"context"
	"slices"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

func (s *Service) WithReadResolver(domain string, r displayread.Resolver) *Service {
	if !displayread.DomainValid(domain) || r == nil {
		panic("invalid display read resolver")
	}
	s.readResolvers[domain] = r
	return s
}
func (s *Service) WithReadQualification(domain string, q displayread.ScopeQualifier) *Service {
	if s.readQualifiers == nil {
		s.readQualifiers = map[string]displayread.ScopeQualifier{}
	}
	s.readQualifiers[domain] = q
	return s
}
func (s *Service) resolveReadMembersTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, d displayread.Definition) ([]displayread.Member, error) {
	// Stored delegation identifies an account, not a login session. Owning
	// modules still require an active identity check for recursive input reads.
	// Recheck current project membership here; each domain then independently
	// verifies its operation permission and object/window grants.
	if a.Check == nil && a.ReadSubject() == nil {
		account := a.ID
		a.Check = func(c context.Context, t pgx.Tx, recent bool) error {
			if recent {
				return access.ErrDenied
			}
			current, e := access.ProjectSnapshotTx(c, t, account, scope.Tenant, scope.Project, "")
			if e != nil {
				return e
			}
			if len(current.InactiveReasons) != 0 {
				return access.ErrDenied
			}
			return nil
		}
	}
	r := s.readResolvers[d.Domain]
	if r == nil {
		return nil, problem("capability_unavailable", "读取领域尚未接入。", 409)
	}
	selector := d.Selector
	selector.StructureModel = d.StructureModel
	if d.Domain == "resources" && d.StructureModel != 2 && len(selector.Kinds) == 0 {
		selector.Kinds = []string{"facility", "component", "point", "channel"}
	}
	members, e := r.Members(ctx, tx, a, scope.Tenant, scope.Project, selector)
	if e != nil {
		return nil, e
	}
	if qualify := s.readQualifiers[d.Domain]; qualify != nil {
		if e = qualify(ctx, tx, a, scope.Tenant, scope.Project, d, members); e != nil {
			return nil, e
		}
	}
	return members, nil
}
func (s *Service) freezeReadScopesTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, v Instance, g *Grant, old Grant) error {
	if e := displayread.ValidateFormat(g.Format, len(g.ReadScopes) > 0, "/scope_format"); e != nil {
		return e
	}
	if g.Format == 0 {
		g.Format = 1
	}
	if len(g.ReadScopes) > 64 {
		return invalid("/read_scopes", "最多定义 64 个读取范围。")
	}
	revision := int64(1)
	if old.Revision != "" {
		n, e := strconv.ParseInt(old.Revision, 10, 64)
		if e != nil {
			return e
		}
		revision = n + 1
	}
	seen := map[string]bool{}
	for i := range g.ReadScopes {
		d := &g.ReadScopes[i]
		path := "/read_scopes/" + strconv.Itoa(i)
		// Returned set references may be echoed by editors, but are never trusted.
		d.SetID, d.Digest = "", ""
		if e := displayread.ValidateDefinition(d, false, path); e != nil {
			return e
		}
		if seen[d.ID] {
			return invalid(path+"/id", "读取范围标识重复。")
		}
		seen[d.ID] = true
		members, e := s.resolveReadMembersTx(ctx, tx, a, scope, *d)
		if e != nil {
			return e
		}
		if e = explicitReadMembers(*d, members); e != nil {
			return e
		}
		if e = displayread.FreezeTx(ctx, tx, displayread.SetOwner{Tenant: scope.Tenant, Project: scope.Project, Instance: v.ID, Kind: "grant", GrantRevision: revision, Name: d.ID}, d, members); e != nil {
			return e
		}
	}
	return nil
}
func explicitReadMembers(d displayread.Definition, members []displayread.Member) error {
	set := map[string]bool{}
	for _, m := range members {
		set[m.Kind+":"+m.ID] = true
		if d.Domain != "devices" || m.Kind == "device" {
			set[m.ID] = true
		}
	}
	for _, id := range d.Selector.IDs {
		if !set[id] {
			return hidden()
		}
	}
	for _, id := range d.Selector.OutputIDs {
		if !set["output:"+id] {
			return hidden()
		}
	}
	return nil
}
func findReadScope(g Grant, id string) (displayread.Definition, bool) {
	for _, d := range g.ReadScopes {
		if d.ID == id {
			return d, true
		}
	}
	return displayread.Definition{}, false
}
func (s *Service) effectiveReadScopeTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, instance string, d displayread.Definition) ([]displayread.Member, error) {
	members, e := s.resolveReadMembersTx(ctx, tx, a, scope, d)
	if e != nil {
		return nil, e
	}
	if !d.IncludeFuture {
		fixed, e := displayread.MembersTx(ctx, tx, scope.Tenant, scope.Project, instance, d)
		if e != nil {
			return nil, e
		}
		members = displayread.IntersectMembers(members, fixed)
	}
	return members, nil
}
func (s *Service) freezeReadViewsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, v Instance, p Package, in *Command, configID string) error {
	if e := displayread.ValidateFormat(in.ConfigFormat, len(in.ReadViews) > 0, "/config_format"); e != nil {
		return e
	}
	if in.ConfigFormat == 0 {
		in.ConfigFormat = 1
	}
	if p.Manifest.Version != 2 && in.ConfigFormat == 2 {
		return invalid("/config_format", "Manifest 1 不能使用新读取视图。")
	}
	if len(in.ReadViews) == 0 {
		return nil
	}
	g, e := grantTx(ctx, tx, v.ID)
	if e != nil {
		return e
	}
	for name, value := range in.ReadViews {
		d := value
		path := "/read_views/" + name
		d.SetID, d.Digest = "", ""
		if e = displayread.ValidateDefinition(&d, true, path); e != nil {
			return e
		}
		schema, ok := p.Manifest.ReadViews[name]
		if !ok || schema.Domain != d.Domain || displayread.StructureVersion(schema.StructureModel) != displayread.StructureVersion(d.StructureModel) || d.IncludeFuture && !schema.AllowDynamic {
			return invalid(path, "视图与程序声明不符。")
		}
		parent, ok := findReadScope(g, d.ScopeID)
		if !ok || !displayread.DefinitionSubset(d, parent) {
			return invalid(path, "视图的字段、时段或未来成员谓词超出授权。")
		}
		members, e := s.resolveReadMembersTx(ctx, tx, a, scope, d)
		if e != nil {
			return e
		}
		if e = explicitReadMembers(d, members); e != nil {
			return e
		}
		allowed, e := s.effectiveReadScopeTx(ctx, tx, a, scope, v.ID, parent)
		if e != nil {
			return e
		}
		if len(displayread.IntersectMembers(members, allowed)) != len(members) {
			return invalid(path, "配置成员超出授权范围。")
		}
		if e = displayread.FreezeTx(ctx, tx, displayread.SetOwner{Tenant: scope.Tenant, Project: scope.Project, Instance: v.ID, Kind: "config", Config: configID, Name: name}, &d, members); e != nil {
			return e
		}
		in.ReadViews[name] = d
	}
	return nil
}

type ReadViewDescription struct {
	StructureModel  int                         `json:"structure_model"`
	DefaultVersions *displayread.DefaultSummary `json:"default_versions,omitempty"`
	Name            string                      `json:"name"`
	Domain          string                      `json:"domain"`
	State           string                      `json:"state"`
	IncludeFuture   bool                        `json:"include_future"`
	FieldGroups     []string                    `json:"field_groups"`
	TimeWindow      *displayread.Window         `json:"time_window,omitempty"`
	MemberCount     string                      `json:"member_count"`
	ScopeHash       string                      `json:"scope_hash"`
	Budgets         map[string]int              `json:"budgets"`
}

func (s *Service) validateReadViewsTx(ctx context.Context, tx pgx.Tx, scope Scope, v Instance, p Package, c Config, g Grant, actor access.Actor, publish bool) ([]ReadViewDescription, error) {
	if e := displayread.ValidateFormat(c.Format, len(c.ReadViews) > 0, "/config_format"); e != nil {
		return nil, e
	}
	if e := displayread.ValidateFormat(g.Format, len(g.ReadScopes) > 0, "/scope_format"); e != nil {
		return nil, e
	}
	out := []ReadViewDescription{}
	for name, d := range c.ReadViews {
		schema, ok := p.Manifest.ReadViews[name]
		if !ok || schema.Domain != d.Domain || displayread.StructureVersion(schema.StructureModel) != displayread.StructureVersion(d.StructureModel) {
			return nil, invalid("/read_views/"+name, "未声明的读取视图。")
		}
		parent, ok := findReadScope(g, d.ScopeID)
		if !ok {
			return nil, problem("scope_unavailable", "读取授权范围已移除。", 409)
		}
		if publish && (!displayread.DefinitionSubset(d, parent) || d.IncludeFuture && !schema.AllowDynamic) {
			return nil, invalid("/read_views/"+name, "发布范围超出当前授权。")
		}
		members, e := s.effectiveReadScopeTx(ctx, tx, actor, scope, v.ID, d)
		if e != nil {
			return nil, e
		}
		allowed, e := s.effectiveReadScopeTx(ctx, tx, actor, scope, v.ID, parent)
		if e != nil {
			return nil, e
		}
		members = displayread.IntersectMembers(members, allowed)
		if publish && schema.Required && len(members) == 0 {
			return nil, invalid("/read_views/"+name, "必需视图不能为空。")
		}
		fields := []string{}
		for _, f := range d.FieldGroups {
			if slices.Contains(parent.FieldGroups, f) {
				fields = append(fields, f)
			}
		}
		state := "ready"
		if len(members) == 0 {
			state = "empty"
		}
		var defaults *displayread.DefaultSummary
		if reader := s.readDefaults[d.Domain]; reader != nil && slices.Contains(fields, "spatial.metadata") {
			value, err := reader(ctx, tx, actor, scope.Tenant, scope.Project, members)
			if err != nil {
				return nil, err
			}
			defaults = &value
		}
		out = append(out, ReadViewDescription{StructureModel: displayread.StructureVersion(d.StructureModel), DefaultVersions: defaults, Name: name, Domain: d.Domain, State: state, IncludeFuture: d.IncludeFuture && parent.IncludeFuture, FieldGroups: fields, TimeWindow: d.TimeWindow, MemberCount: displayread.Count(members), ScopeHash: hash([]any{d, parent, members}), Budgets: map[string]int{"max_view_members": displayread.MaxMembers, "max_explicit_ids": displayread.MaxIDs, "max_page_size": displayread.MaxPageSize, "max_map_page_size": displayread.MaxMapPageSize, "max_details": displayread.MaxDetails, "max_trend_channels": displayread.MaxTrendChannels, "async_deadline_seconds": 120}})
	}
	if publish {
		for name, schema := range p.Manifest.ReadViews {
			if schema.Required {
				if _, ok := c.ReadViews[name]; !ok {
					return nil, invalid("/read_views/"+name, "缺少必需读取视图。")
				}
			}
		}
	}
	slices.SortFunc(out, func(a, b ReadViewDescription) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return out, nil
}

func readScopesNarrow(old, next Grant) bool {
	for _, d := range next.ReadScopes {
		before, ok := findReadScope(old, d.ID)
		if !ok || !displayread.DefinitionSubset(d, before) || !displayread.SelectorSubset(d.Selector, before.Selector) {
			return false
		}
	}
	return true
}
