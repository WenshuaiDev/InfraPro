package sources

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	r "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/jackc/pgx/v5"
	"strconv"
)

type Service struct {
	access    *access.Service
	devices   *devices.Service
	resources *r.Service
}

func New(a *access.Service, d *devices.Service, resources *r.Service) *Service {
	return &Service{access: a, devices: d, resources: resources}
}
func resourceScope(s Scope) r.Scope {
	if s.Entry == "project" {
		return r.ProjectScope(s.Tenant, s.Project)
	}
	return r.TenantScope(s.Tenant)
}
func (s *Service) transact(ctx context.Context, a access.Actor, scope Scope, kind, action string, write bool, fn func(pgx.Tx, k.Authorization) error) error {
	if scope.Entry == "platform" {
		return k.NotFound("source")
	}
	e := k.Transact(ctx, s.access, a, scope, scope.Operation(kind, action), k.Locks{TenantWrite: write}, fn)
	var re *r.Error
	if errors.As(e, &re) {
		out := k.Problem(re.Code, re.Message, re.Status)
		for _, i := range re.Issues {
			out.Issues = append(out.Issues, Issue{Path: i.Path, Code: i.Code, Message: i.Message})
		}
		return out
	}
	return e
}
func (s *Service) channelTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, target Target, write bool) (r.DeviceTarget, c.Semantics, error) {
	var semantic c.Semantics
	out, e := s.resources.DeviceTargetTx(ctx, tx, a, resourceScope(scope), r.Target{Kind: r.Channel, Facility: target.FacilityID, Point: target.PointID, ID: target.ChannelID})
	if e != nil {
		return out, semantic, e
	}
	if write && scope.Entry == "project" && !out.SourceManage {
		return out, semantic, access.ErrDenied
	}
	v := out.Resource
	semantic = c.Semantics{MetricVersionID: v.MetricVersionID, UnitVersionID: v.UnitVersionID, TimestampMeaning: v.TimestampMeaning, AggregationDescription: v.AggregationDescription, ObservationDirection: v.ObservationDirection, ReferenceBasis: v.ReferenceBasis}
	semantic, _, e = c.ResolveSemanticsTx(ctx, tx, semantic, false)
	return out, semantic, e
}
func currentBindingTx(ctx context.Context, tx pgx.Tx, scope Scope, target Target) (*Binding, string, error) {
	revision := "0"
	e := tx.QueryRow(ctx, `SELECT revision::text FROM monitoring_source_heads WHERE tenant_id=$1 AND channel_id=$2`, scope.Tenant, target.ChannelID).Scan(&revision)
	if e != nil && e != pgx.ErrNoRows {
		return nil, "", e
	}
	var raw []byte
	e = tx.QueryRow(ctx, `SELECT to_jsonb(b)||jsonb_build_object('revision',revision::text) FROM monitoring_source_bindings b WHERE tenant_id=$1 AND channel_id=$2 AND ended_at IS NULL`, scope.Tenant, target.ChannelID).Scan(&raw)
	if e == pgx.ErrNoRows {
		return nil, revision, nil
	}
	if e != nil {
		return nil, "", e
	}
	var b Binding
	e = json.Unmarshal(raw, &b)
	return &b, revision, e
}
func bindingTx(ctx context.Context, tx pgx.Tx, scope Scope, target Target, id string) (Binding, error) {
	var out Binding
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT to_jsonb(b)||jsonb_build_object('revision',revision::text) FROM monitoring_source_bindings b WHERE tenant_id=$1 AND facility_id=$2 AND point_id=$3 AND channel_id=$4 AND id=$5`, scope.Tenant, target.FacilityID, target.PointID, target.ChannelID, id).Scan(&raw)
	if e == pgx.ErrNoRows {
		return out, k.NotFound("source")
	}
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(raw, &out)
	return out, e
}
func sourceWritable(auth k.Authorization) bool {
	if auth.Entry == "tenant" {
		return auth.Has(access.TenantMonitoringSourceManage) && auth.Has(access.TenantDeviceRead) && auth.Has(access.TenantEngineeringResourceRead)
	}
	return auth.ProjectStatus != "archived" && auth.Has(access.ProjectMonitoringSourceManage) && auth.Has(access.ProjectDeviceRead) && auth.Has(access.ProjectDeviceOutputUse) && auth.Has(access.ProjectEngineeringResourceRead)
}
func hidden(err error) bool {
	var e *k.Error
	var ae *access.Error
	return errors.As(err, &e) && (e.Status == 403 || e.Status == 404) || errors.As(err, &ae) && (ae.Status == 403 || ae.Status == 404)
}
func (s *Service) projectBinding(ctx context.Context, tx pgx.Tx, a access.Actor, auth k.Authorization, b *Binding, history bool) (Projection, error) {
	out := Projection{Configured: b != nil, Visibility: "full"}
	if b == nil {
		return out, nil
	}
	permission := access.TenantDeviceRead
	if auth.Entry == "project" {
		permission = access.ProjectDeviceRead
	}
	if !auth.Has(permission) {
		out.Visibility = "restricted"
		return out, nil
	}
	if history {
		permission = access.TenantDeviceHistoryRead
		if auth.Entry == "project" {
			permission = access.ProjectDeviceHistoryRead
		}
		if !auth.Has(permission) {
			out.Visibility = "restricted"
			return out, nil
		}
	}
	o, e := s.devices.ResolveOutputTx(ctx, tx, a, auth.Scope, b.OutputID, false)
	if hidden(e) {
		out.Visibility = "restricted"
		return out, nil
	}
	if e != nil {
		return out, e
	}
	out.ID = b.ID
	out.Revision = b.Revision
	out.Output = &o
	out.StartedAt = &b.StartedAt
	out.EndedAt = b.EndedAt
	if history {
		out.OutputSnapshot = b.OutputSnapshot
		out.ChannelSnapshot = b.ChannelSnapshot
		if auth.Entry == "tenant" || b.CreatedProjectID != nil && *b.CreatedProjectID == auth.Project {
			out.Reason = b.CreatedReason
		}
	}
	return out, nil
}
func (s *Service) Current(ctx context.Context, a access.Actor, scope Scope, target Target) (Current, error) {
	var out Current
	e := s.transact(ctx, a, scope, "source", "detail", false, func(tx pgx.Tx, auth k.Authorization) error {
		channel, semantic, e := s.channelTx(ctx, tx, a, scope, target, false)
		if e != nil {
			return e
		}
		binding, revision, e := currentBindingTx(ctx, tx, scope, target)
		if e != nil {
			return e
		}
		out = Current{SourceRevision: revision, ChannelRevision: strconv.Itoa(channel.Resource.Revision), ResourceAccessRevision: auth.ResourceAccessRevision, DeviceAccessRevision: auth.DeviceAccessRevision, LinkRevision: channel.LinkRevision, TargetRetired: channel.Retired, Semantics: semantic}
		out.Source, e = s.projectBinding(ctx, tx, a, auth, binding, false)
		if e != nil {
			return e
		}
		can := sourceWritable(auth) && (auth.Entry == "tenant" || channel.SourceManage)
		out.Actions.Bind = can && binding == nil && !channel.Retired
		if can && binding != nil {
			_, e := s.devices.ResolveOutputTx(ctx, tx, a, scope, binding.OutputID, true)
			if e != nil && !hidden(e) {
				return e
			}
			out.Actions.Unbind = e == nil
			out.Actions.Switch = e == nil && !channel.Retired
		}
		return nil
	})
	return out, e
}

func (s *Service) projectBindings(ctx context.Context, tx pgx.Tx, a access.Actor, auth k.Authorization, bindings []Binding, history bool) ([]Projection, error) {
	out := make([]Projection, 0, len(bindings))
	read, readHistory := access.TenantDeviceRead, access.TenantDeviceHistoryRead
	if auth.Entry == "project" {
		read, readHistory = access.ProjectDeviceRead, access.ProjectDeviceHistoryRead
	}
	outputs := map[string]devices.Output{}
	if auth.Has(read) && (!history || auth.Has(readHistory)) {
		ids := make([]string, 0, len(bindings))
		for _, b := range bindings {
			ids = append(ids, b.OutputID)
		}
		var e error
		outputs, e = s.devices.OutputsByIDTx(ctx, tx, a, auth.Scope, ids, false)
		if e != nil {
			return nil, e
		}
	}
	for _, b := range bindings {
		v := Projection{Configured: true, Visibility: "restricted"}
		if o, ok := outputs[b.OutputID]; ok {
			v = Projection{Configured: true, Visibility: "full", ID: b.ID, Revision: b.Revision, Output: &o, StartedAt: &b.StartedAt, EndedAt: b.EndedAt}
			if history {
				v.OutputSnapshot = b.OutputSnapshot
				v.ChannelSnapshot = b.ChannelSnapshot
				if auth.Entry == "tenant" || b.CreatedProjectID != nil && *b.CreatedProjectID == auth.Project {
					v.Reason = b.CreatedReason
				}
			}
		}
		out = append(out, v)
	}
	return out, nil
}
