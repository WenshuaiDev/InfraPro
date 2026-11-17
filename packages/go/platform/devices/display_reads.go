package devices

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

func (s *Service) DisplayDevicesTx(ctx context.Context, tx pgx.Tx, p displayread.Plan, filter displayread.DeviceFilters, search string) ([]Device, error) {
	o := p.Authority.Owner
	a := p.Authority.Subject.ReadActor()
	scope := p.Scopes["target"]
	for _, id := range filter.FacilityIDs {
		if !slices.Contains(p.Scopes["resource_target"].IDs("facility"), id) {
			return nil, displayread.Hidden()
		}
	}
	if _, e := access.RequireActorProjectTx(ctx, tx, a, access.DeviceOperation("project", "device", "list"), o.Tenant, o.Project); e != nil {
		return nil, e
	}
	rows, e := tx.Query(ctx, `SELECT `+deviceJSON+deviceFrom+` WHERE d.tenant_id=$1 AND d.id=ANY($3::uuid[]) AND d.deleted_at IS NULL
 AND EXISTS(SELECT 1 FROM project_device_grants g WHERE g.tenant_id=d.tenant_id AND g.project_id=$2 AND g.device_id=d.id AND g.kind='device' AND g.ended_at IS NULL)
 AND ($4='' OR strpos(lower(d.name),lower($4))>0 OR strpos(lower(d.code),lower($4))>0)
 AND (COALESCE(cardinality($5::uuid[]),0)=0 OR p.manufacturer_id=ANY($5)) AND (COALESCE(cardinality($6::uuid[]),0)=0 OR d.product_id=ANY($6))
 AND (COALESCE(cardinality($7::uuid[]),0)=0 OR d.product_version_id=ANY($7)) AND (COALESCE(cardinality($8::text[]),0)=0 OR d.status=ANY($8))
 AND ($9='' OR ($9='installed')=EXISTS(SELECT 1 FROM device_installations i WHERE i.tenant_id=d.tenant_id AND i.device_id=d.id AND i.uninstalled_at IS NULL)) ORDER BY d.code,d.id`, o.Tenant, o.Project, scope.IDs("device"), search, filter.ManufacturerIDs, filter.ProductIDs, filter.ProductVersionIDs, filter.Statuses, filter.InstallationState)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []Device{}
	for rows.Next() {
		var raw []byte
		var v Device
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &v); e != nil {
			return nil, e
		}
		items = append(items, v)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	rows.Close()
	items, e = displayClassificationsTx(ctx, tx, items, filter)
	if e != nil {
		return nil, e
	}
	if len(filter.FacilityIDs) > 0 {
		relations, e := s.DisplayRelationsTx(ctx, tx, p, filter.Relation)
		if e != nil {
			return nil, e
		}
		items = slices.DeleteFunc(items, func(d Device) bool {
			for _, f := range relations[d.ID] {
				if slices.Contains(filter.FacilityIDs, f) {
					return false
				}
			}
			return true
		})
	}
	return items, nil
}
func DisplayDevice(d Device, fields []string) displayread.DeviceDetail {
	out := displayread.DeviceDetail{ID: d.ID, Code: d.Code, Name: d.Name, Status: d.Status, Revision: d.Revision, Visibility: "full", Product: displayread.ProductVersionRef{ID: d.ProductVersionID, ProductID: d.ProductID, Code: d.Product.Code, Name: d.Product.Name, Number: strconv.Itoa(d.Product.Number), AdoptedModel: d.Product.Model}, Manufacturer: displayread.NamedRef{ID: d.Product.ManufacturerID, Name: d.Product.ManufacturerName}, AdoptedModel: d.Product.Model}
	if slices.Contains(fields, "device.classification") && d.Classification != nil {
		var v displayread.Classification
		_ = json.Unmarshal(displayread.Bytes(d.Classification), &v)
		out.Classification = &v
	}
	if slices.Contains(fields, "device.serial_number") {
		out.SerialNumber = &d.SerialNumber
	}
	if slices.Contains(fields, "device.description") {
		out.Description = &d.Description
	}
	return out
}
func (s *Service) DisplayReadTx(ctx context.Context, tx pgx.Tx, p displayread.Plan) (displayread.Materialized, error) {
	out := displayread.Materialized{Rows: []json.RawMessage{}}
	scope := p.Scopes["target"]
	if q, ok := p.Request.(*displayread.DeviceOutputsRequest); ok {
		values, e := s.DisplayOutputsTx(ctx, tx, p.Authority.Subject.ReadActor(), p.Authority.Owner.Tenant, p.Authority.Owner.Project, scope, q.ID, false)
		if e != nil {
			return out, e
		}
		for _, v := range values {
			out.Rows = append(out.Rows, displayread.Bytes(v))
		}
		return out, nil
	}
	if q, ok := p.Request.(*displayread.DeviceHistoryRequest); ok {
		return s.displayHistoryTx(ctx, tx, p, *q)
	}
	var filters displayread.DeviceFilters
	var search, id string
	switch q := p.Request.(type) {
	case *displayread.DevicesRequest:
		filters, search = q.Filters, q.Search
	case *displayread.DeviceGetRequest:
		id = q.ID
	}
	values, e := s.DisplayDevicesTx(ctx, tx, p, filters, search)
	if e != nil {
		return out, e
	}
	var installations map[string][]displayread.DeviceInstallation
	if slices.Contains(scope.FieldGroups, "device.installations") {
		installations, e = s.DisplayInstallationsTx(ctx, tx, p, scope.IDs("device"), false)
		if e != nil {
			return out, e
		}
	}
	found := false
	for _, v := range values {
		item := DisplayDevice(v, scope.FieldGroups)
		if items := installations[v.ID]; len(items) > 0 {
			item.CurrentInstallation = &items[0]
		}
		if id != "" {
			if id == v.ID {
				out.Single = item
				found = true
			}
		} else {
			out.Rows = append(out.Rows, displayread.Bytes(item))
		}
	}
	if id != "" && !found {
		return out, displayread.Hidden()
	}
	return out, nil
}
func (s *Service) DisplayOutputsTx(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, project string, scope displayread.Scope, device string, history bool) ([]displayread.DeviceOutput, error) {
	if _, e := access.RequireActorProjectTx(ctx, tx, a, access.DeviceOperation("project", "output", "detail"), tenant, project); e != nil {
		return nil, e
	}
	ids := scope.IDs("output")
	outputs, e := s.OutputsByIDTx(ctx, tx, a, ProjectScope(tenant, project), ids, false)
	if e != nil {
		return nil, e
	}
	items := []displayread.DeviceOutput{}
	full := scope.IDs("device")
	contextDevice := false
	for _, o := range outputs {
		if device != "" && o.DeviceID != device {
			continue
		}
		contextDevice = true
		if !history && o.EndedAt != nil {
			continue
		}
		v := displayread.DeviceOutput{ID: o.ID, DeviceID: o.DeviceID, SlotKey: o.SlotKey, Generation: strconv.Itoa(o.Generation), Name: o.Name, Revision: o.Revision, StartedAt: o.StartedAt, EndedAt: o.EndedAt, ProductVersionID: o.ProductVersionID, State: "active", OutputSemantics: displaySemantics(o.Semantics), Device: displayread.NamedRef{ID: o.Device.ID, Code: o.Device.Code, Name: o.Device.Name, Visibility: "context"}, Definitions: []displayread.DefinitionVersion{}}
		if slices.Contains(full, o.DeviceID) {
			v.Device.Visibility = "full"
		}
		if o.EndedAt != nil {
			v.State = "ended"
		} else if o.DeviceStatus != "enabled" {
			v.State = "attention"
		}
		for _, d := range o.Definitions {
			v.Definitions = append(v.Definitions, displayread.DefinitionVersion{ID: d.ID, DefinitionID: d.DefinitionID, Kind: string(d.Kind), Number: strconv.Itoa(d.Number), Content: k.JSON(d.Content)})
		}
		items = append(items, v)
	}
	if device != "" && !contextDevice && !slices.Contains(full, device) {
		return nil, displayread.Hidden()
	}
	slices.SortFunc(items, func(a, b displayread.DeviceOutput) int {
		if n := strings.Compare(a.SlotKey, b.SlotKey); n != 0 {
			return n
		}
		if n := b.StartedAt.Compare(a.StartedAt); n != 0 {
			return n
		}
		return strings.Compare(a.ID, b.ID)
	})
	return items, nil
}
func (s *Service) DisplayInstallationsTx(ctx context.Context, tx pgx.Tx, p displayread.Plan, devices []string, history bool) (map[string][]displayread.DeviceInstallation, error) {
	o := p.Authority.Owner
	a := p.Authority.Subject.ReadActor()
	out := map[string][]displayread.DeviceInstallation{}
	rows, e := tx.Query(ctx, `SELECT to_jsonb(i)||jsonb_build_object('revision',revision::text) FROM device_installations i WHERE tenant_id=$1 AND device_id=ANY($2::uuid[]) AND ($3 OR uninstalled_at IS NULL) ORDER BY installed_at DESC,id DESC`, o.Tenant, devices, history)
	if e != nil {
		return nil, e
	}
	items := []Installation{}
	for rows.Next() {
		var raw []byte
		var v Installation
		if e = rows.Scan(&raw); e != nil {
			rows.Close()
			return nil, e
		}
		if e = json.Unmarshal(raw, &v); e != nil {
			rows.Close()
			return nil, e
		}
		items = append(items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	if s.resources != nil && len(p.Scopes["resource_target"].Members) > 0 {
		items, e = s.resources.ProjectInstallationsTx(ctx, tx, a, ProjectScope(o.Tenant, o.Project), items)
		if e != nil {
			return nil, e
		}
	}
	allowed := p.Scopes["resource_target"].Members
	for _, v := range items {
		item := displayread.DeviceInstallation{ID: v.ID, DeviceID: v.DeviceID, Revision: v.Revision, Visibility: "restricted", InstalledAt: v.InstalledAt, UninstalledAt: v.UninstalledAt}
		id := ""
		switch v.TargetKind {
		case "facility":
			if v.FacilityID != nil {
				id = *v.FacilityID
			}
		case "component":
			if v.ComponentID != nil {
				id = *v.ComponentID
			}
		case "point":
			if v.PointID != nil {
				id = *v.PointID
			}
		case "monitoring_section":
			if v.MonitoringSectionID != nil {
				id = *v.MonitoringSectionID
			}
		case "borehole":
			if v.BoreholeID != nil {
				id = *v.BoreholeID
			}
		}
		if v.TargetKind == "location" {
			item.Visibility = "full"
			item.Position = v.Position
			item.LocationDescription = &v.LocationDescription
		} else if slices.Contains(allowed, displayread.Member{Kind: v.TargetKind, ID: id}) && v.Visibility == "full" {
			item.Visibility = "full"
			item.Target = &displayread.Ref{Kind: v.TargetKind, ID: id}
			item.TargetName = v.TargetName
			item.TargetStatus = v.TargetStatus
			item.Position = v.Position
		}
		out[v.DeviceID] = append(out[v.DeviceID], item)
	}
	return out, nil
}
