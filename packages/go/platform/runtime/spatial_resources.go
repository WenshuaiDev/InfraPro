package runtime

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"github.com/jackc/pgx/v5"
)

type spatialObjects struct{ resources *resources.Service }

func spatialResourceError(e error) error {
	var re *resources.Error
	if errors.As(e, &re) {
		return k.Problem(re.Code, re.Message, re.Status)
	}
	return e
}

func spatialObject(r resources.Resource, retired bool) spatial.Object {
	facility := r.FacilityID
	if r.Kind == resources.Facility {
		facility = r.ID
	}
	out := spatial.Object{ObjectRef: spatial.ObjectRef{Kind: string(r.Kind), ID: r.ID}, FacilityID: facility, Code: r.Code, Name: r.Name, Status: r.Status, Retired: retired, Position: k.JSON(r.Position), BasisHash: k.Hash([]any{r.FacilityID, r.ComponentID, r.ParentComponentID, r.Position})}
	if r.Kind == resources.Channel {
		out.OwnerKind = r.OwnerKind
		switch r.OwnerKind {
		case "facility":
			out.OwnerID = r.FacilityID
		case "component":
			if r.ComponentID != nil {
				out.OwnerID = *r.ComponentID
			}
		default:
			out.OwnerKind = "point"
			out.OwnerID = r.PointID
		}
	}
	if r.Position == nil {
		out.PositionNotice = "未提供工程位置"
		return out
	}
	var crs string
	var x, y *float64
	if p := r.Position.Geographic; p != nil {
		crs = p.CRS
		x = p.Longitude
		y = p.Latitude
	} else if p := r.Position.Projected; p != nil && p.Unit == "m" {
		crs = p.CRS
		x = p.X
		y = p.Y
	}
	if x == nil || y == nil {
		out.PositionNotice = "仅有桩号或局部位置，暂无可显示的地理坐标"
		return out
	}
	lon, lat, e := spatial.Geographic(crs, *x, *y)
	if e != nil {
		out.PositionNotice = e.Error()
		return out
	}
	out.Longitude = &lon
	out.Latitude = &lat
	return out
}
func (r spatialObjects) ObjectTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope spatial.Scope, ref spatial.ObjectRef) (spatial.Object, error) {
	proof, e := r.resources.SpatialTargetTx(ctx, tx, a, resourceScope(scope), resources.Target{Kind: resources.Kind(ref.Kind), ID: ref.ID})
	if e != nil {
		return spatial.Object{}, spatialResourceError(e)
	}
	return spatialObject(proof.Resource, proof.Retired), nil
}
func (r spatialObjects) Objects(ctx context.Context, a access.Actor, scope spatial.Scope, f spatial.Filter) (spatial.List[spatial.Object], error) {
	out := spatial.List[spatial.Object]{Items: []spatial.Object{}}
	kind := resources.Kind(f.Kind)
	if kind == "" {
		kind = resources.Facility
	}
	page, e := r.resources.SpatialTargets(ctx, a, resourceScope(scope), kind, f.FacilityID, resources.Filter{Query: f.Query, Status: f.State, Cursor: f.Cursor, Limit: f.Limit}, f.BBox)
	if e != nil {
		return out, spatialResourceError(e)
	}
	for _, item := range page.Items {
		o := spatialObject(item.Resource, item.Status != "normal")
		if len(f.BBox) > 0 && (o.Longitude == nil || !spatial.InBBox(*o.Longitude, *o.Latitude, f.BBox)) {
			continue
		}
		out.Items = append(out.Items, o)
	}
	out.NextCursor = page.NextCursor
	return out, nil
}

type spatialReferences struct{ service *spatial.Service }

func (r spatialReferences) ResourceImpactTx(ctx context.Context, tx pgx.Tx, scope resources.Scope, target resources.Target) (resources.ExternalImpact, error) {
	ids := []string{}
	rows, e := tx.Query(ctx, `WITH RECURSIVE affected AS(SELECT kind,id FROM engineering_resource_nodes WHERE tenant_id=$1 AND kind=$2 AND id=$3 UNION SELECT n.kind,n.id FROM engineering_resource_nodes n JOIN affected a ON n.parent_kind=a.kind AND n.parent_id=a.id WHERE n.tenant_id=$1) SELECT id::text FROM affected`, scope.Tenant, target.Kind, target.ID)
	if e != nil {
		return resources.ExternalImpact{}, e
	}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return resources.ExternalImpact{}, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return resources.ExternalImpact{}, e
	}
	hash, used, e := r.service.ReferencesTx(ctx, tx, scope.Tenant, ids)
	return resources.ExternalImpact{Hash: hash, Referenced: used}, e
}
