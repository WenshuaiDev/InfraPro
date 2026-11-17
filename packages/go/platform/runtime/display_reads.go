package runtime

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"github.com/jackc/pgx/v5"
)

func installDisplayReads(b Business) {
	b.DisplayReads.Register("classification.categories", b.Classification.DisplayCategoriesTx)
	b.DisplayReads.Register("statistics.deviceCategories", b.Devices.DisplayCategoryStatisticsTx)
	b.DisplayReads.Register("statistics.structure", func(ctx context.Context, tx pgx.Tx, p displayread.Plan) (displayread.Materialized, error) {
		if scope, ok := p.Scopes["channel_target"]; ok {
			if _, _, e := b.Monitoring.DisplayChannelsTx(ctx, tx, p, scope.IDs("channel"), displayread.ChannelFilters{}, ""); e != nil {
				return displayread.Materialized{}, e
			}
		}
		return b.Resources.DisplayStructureStatisticsTx(ctx, tx, p)
	})
	b.Displays.WithReadDefaults("spatial", b.Spatial.DisplayDefaultsTx)
	b.Displays.WithReadOptions("resources", b.Resources.DisplayOptionsTx).WithReadOptions("channels", b.Monitoring.DisplayOptionsTx).WithReadOptions("devices", b.Devices.DisplayOptionsTx).WithReadOptions("spatial", b.Spatial.DisplayOptionsTx)
	b.Monitoring.WithDisplayQualification(b.DerivedMonitoring.QualifyDisplayInputsTx)
	b.Displays.WithReadQualification("channels", b.Monitoring.QualifyDisplayMembersTx)
	b.DisplayReads.WithDrilldown(func(ctx context.Context, tx pgx.Tx, p displayread.Plan, proof json.RawMessage, q *displayread.DrilldownRequest) (displayread.Scope, json.RawMessage, error) {
		switch p.Method {
		case "statistics.deviceCategories":
			return b.Devices.DisplayCategoryDrilldownTx(ctx, tx, p, proof, q)
		case "statistics.structure":
			return b.Resources.DisplayStructureDrilldownTx(ctx, tx, p, proof, q)
		default:
			return b.Monitoring.DisplayDrilldownTx(ctx, tx, p, proof, q)
		}
	}, func(ctx context.Context, tx pgx.Tx, p displayread.Plan, proof json.RawMessage) (displayread.Materialized, error) {
		switch p.Method {
		case "devices.list":
			return b.Devices.DisplayCategorySelectionTx(ctx, tx, p, proof)
		case "resources.query":
			return b.Resources.DisplayStructureSelectionTx(ctx, tx, p, proof)
		default:
			return b.Monitoring.DisplaySelectionTx(ctx, tx, p, proof)
		}
	})
	b.DisplayReads.WithWorkLifecycle(b.Monitoring.DisplayBindWorkTx, b.Monitoring.DisplayReleaseWorkTx)
	for _, method := range []string{"channels.list", "channels.get"} {
		b.DisplayReads.Register(method, b.Monitoring.DisplayDirectoryTx)
		b.DisplayReads.RegisterValidator(method, b.Monitoring.DisplayCheckTx)
	}
	for _, method := range []string{"statistics.monitoring", "monitoring.latest", "monitoring.details", "monitoring.trends", "monitoring.statistics", "monitoring.overview"} {
		b.DisplayReads.RegisterWork(method, func(ctx context.Context, tx pgx.Tx, p displayread.Plan) (displayread.Materialized, error) {
			var items []displayread.ResourceSummary
			if scope, ok := p.Scopes["resource_target"]; ok {
				var e error
				o := p.Authority.Owner
				items, e = b.Resources.DisplayDirectoryTx(ctx, tx, p.Authority.Subject.ReadActor(), o.Tenant, o.Project, scope, false)
				if e != nil {
					return displayread.Materialized{}, e
				}
			}
			if q, ok := p.Request.(*displayread.MonitoringStatisticsRequest); ok && displayread.NewStructureKind(q.GroupBy) {
				refs := []displayread.Ref{}
				seen := map[string]bool{}
				for _, scope := range p.Scopes {
					if scope.Domain == "channels" {
						for _, id := range scope.IDs("channel") {
							if !seen[id] {
								seen[id] = true
								refs = append(refs, displayread.Ref{Kind: "channel", ID: id})
							}
						}
					}
				}
				groups, e := b.Resources.DisplayStructureMembershipTx(ctx, tx, p, p.Scopes["resource_target"], refs, q.GroupBy, q.Membership)
				if e != nil {
					return displayread.Materialized{}, e
				}
				return b.Monitoring.DisplayPrepareTx(ctx, tx, p, items, groups)
			}
			return b.Monitoring.DisplayPrepareTx(ctx, tx, p, items)
		}, b.Monitoring.DisplayExecute, b.Monitoring.DisplayCheckTx)
	}
	for _, method := range []string{"map.basemaps", "map.readTile", "map.features", "spatial.assets", "spatial.versions", "spatial.locations", "spatial.componentLinks", "spatial.settings", "spatial.manifest", "spatial.objects", "spatial.placements", "spatial.readFile"} {
		b.DisplayReads.Register(method, func(ctx context.Context, tx pgx.Tx, p displayread.Plan) (displayread.Materialized, error) {
			var items []displayread.ResourceSummary
			resourceScope, ok := p.Scopes["resource_target"]
			if p.Method == "spatial.locations" {
				resourceScope, ok = p.Scopes["target"]
			}
			if ok {
				var e error
				o := p.Authority.Owner
				items, e = b.Resources.DisplayDirectoryTx(ctx, tx, p.Authority.Subject.ReadActor(), o.Tenant, o.Project, resourceScope, false)
				if e != nil {
					return displayread.Materialized{}, e
				}
			}
			return b.Spatial.DisplayReadTx(ctx, tx, p, items)
		})
	}
	b.Devices.WithDisplaySourceRelations(b.Sources.DisplayDeviceRelationsTx)
	for _, method := range []string{"devices.list", "devices.get", "devices.outputs", "devices.history"} {
		b.DisplayReads.Register(method, b.Devices.DisplayReadTx)
	}
	for _, method := range []string{"sources.bindings", "sources.bindingHistory"} {
		b.DisplayReads.Register(method, b.Sources.DisplayReadTx)
	}
	b.DisplayReads.Register("statistics.devices", func(ctx context.Context, tx pgx.Tx, p displayread.Plan) (displayread.Materialized, error) {
		var items []displayread.ResourceSummary
		if scope, ok := p.Scopes["resource_target"]; ok {
			var e error
			o := p.Authority.Owner
			items, e = b.Resources.DisplayDirectoryTx(ctx, tx, p.Authority.Subject.ReadActor(), o.Tenant, o.Project, scope, false)
			if e != nil {
				return displayread.Materialized{}, e
			}
		}
		q := p.Request.(*displayread.DeviceStatisticsRequest)
		if displayread.NewStructureKind(q.GroupBy) {
			scope := p.Scopes["resource_target"]
			if scope.StructureModel != 2 {
				return displayread.Materialized{}, displayread.StructureUpgradeRequired()
			}
			owners := map[string][]displayread.Ref{}
			if q.Relation == "source" {
				var e error
				owners, e = b.Sources.DisplayDeviceStructureTargetsTx(ctx, tx, p)
				if e != nil {
					return displayread.Materialized{}, e
				}
			} else {
				installed, e := b.Devices.DisplayInstallationsTx(ctx, tx, p, p.Scopes["target"].IDs("device"), false)
				if e != nil {
					return displayread.Materialized{}, e
				}
				for device, values := range installed {
					for _, v := range values {
						if v.Target != nil {
							owners[device] = append(owners[device], *v.Target)
						} else if v.Visibility == "restricted" {
							owners[device] = append(owners[device], displayread.Ref{Kind: "restricted"})
						}
					}
				}
			}
			refs := []displayread.Ref{}
			seen := map[string]bool{}
			for _, values := range owners {
				for _, v := range values {
					key := displayread.RefKey(v)
					if v.Kind != "restricted" && !seen[key] {
						refs = append(refs, v)
						seen[key] = true
					}
				}
			}
			memberships, e := b.Resources.DisplayStructureMembershipTx(ctx, tx, p, scope, refs, q.GroupBy, q.Membership)
			if e != nil {
				return displayread.Materialized{}, e
			}
			grouped := map[string][]displayread.StructureGroupRef{}
			for device, values := range owners {
				keys := map[string]bool{}
				for _, v := range values {
					groups := memberships[displayread.RefKey(v)]
					if v.Kind == "restricted" {
						groups = []displayread.StructureGroupRef{{Key: "restricted_context", Label: "受限工程归属", Visibility: "restricted"}}
					}
					for _, g := range groups {
						if !keys[g.Key] {
							grouped[device] = append(grouped[device], g)
							keys[g.Key] = true
						}
					}
				}
			}
			return b.Devices.DisplayStatisticsTx(ctx, tx, p, items, grouped)
		}
		return b.Devices.DisplayStatisticsTx(ctx, tx, p, items)
	})
	for _, method := range []string{"resources.types", "resources.query", "resources.tree", "resources.hierarchy", "resources.detail"} {
		b.DisplayReads.Register(method, b.Resources.DisplayReadTx)
	}
	b.DisplayReads.Register("statistics.resources", func(ctx context.Context, tx pgx.Tx, p displayread.Plan) (displayread.Materialized, error) {
		if scope, ok := p.Scopes["channel_target"]; ok {
			if _, _, e := b.Monitoring.DisplayChannelsTx(ctx, tx, p, scope.IDs("channel"), displayread.ChannelFilters{}, ""); e != nil {
				return displayread.Materialized{}, e
			}
		}
		return b.Resources.DisplayStatisticsTx(ctx, tx, p)
	})
	b.DisplayReads.Register("map.facilities", func(ctx context.Context, tx pgx.Tx, p displayread.Plan) (displayread.Materialized, error) {
		items, e := b.Resources.DisplayMapItemsTx(ctx, tx, p)
		if e != nil {
			return displayread.Materialized{}, e
		}
		return spatial.DisplayFacilityMap(items, *p.Request.(*displayread.MapFacilitiesRequest)), nil
	})
	b.Displays.WithReads(b.DisplayReads)
}
