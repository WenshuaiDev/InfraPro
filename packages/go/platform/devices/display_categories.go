package devices

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/classification"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
	"slices"
	"strconv"
	"strings"
)

type categoryDeviceSnapshot struct {
	Device         Device                   `json:"device"`
	Classification classification.Effective `json:"classification"`
	Groups         []string                 `json:"groups"`
}

func (s *Service) DisplayCategoryDrilldownTx(ctx context.Context, tx pgx.Tx, p dr.Plan, raw json.RawMessage, q *dr.DrilldownRequest) (dr.Scope, json.RawMessage, error) {
	if q.Basis != "device_count" || q.Purpose != "devices" {
		return dr.Scope{}, nil, dr.Invalid("/basis", "分类下钻须选择设备台数和设备目录。")
	}
	var proof categoryStatisticsProof
	if e := json.Unmarshal(raw, &proof); e != nil {
		return dr.Scope{}, nil, e
	}
	selected := categoryStatisticsProof{ReleaseID: proof.ReleaseID, Devices: []categoryDeviceSnapshot{}}
	scope := dr.Scope{StructureModel: p.Scopes["target"].StructureModel, Domain: "devices", Members: []dr.Member{}}
	for _, d := range proof.Devices {
		if q.GroupKey == "" || slices.Contains(d.Groups, q.GroupKey) {
			selected.Devices = append(selected.Devices, d)
			scope.Members = append(scope.Members, dr.Member{Kind: "device", ID: d.Device.ID})
		}
	}
	scope.Hash = dr.Hash([]any{p.Query.ID, q, scope.Members})
	return scope, dr.Bytes(selected), nil
}

func (s *Service) DisplayCategorySelectionTx(ctx context.Context, tx pgx.Tx, p dr.Plan, raw json.RawMessage) (dr.Materialized, error) {
	out := dr.Materialized{Rows: []json.RawMessage{}}
	var proof categoryStatisticsProof
	if e := json.Unmarshal(raw, &proof); e != nil {
		return out, e
	}
	q := p.Request.(*dr.DevicesRequest)
	scope := p.Scopes["target"]
	if q.Filters.DictionaryReleaseID != "" && q.Filters.DictionaryReleaseID != proof.ReleaseID {
		return out, dr.Invalid("/filters/dictionary_release_id", "下钻沿用父查询固定字典版本。")
	}
	for _, field := range scope.FieldGroups {
		if !slices.Contains([]string{"device.metadata", "device.serial_number", "device.description", "device.classification"}, field) {
			return out, dr.Invalid("/field_groups", "分类下钻只提供固定设备档案和分类；其他关联请独立查询。")
		}
	}
	// Current eligibility is independently checked; displayed facts stay pinned.
	current, e := s.DisplayDevicesTx(ctx, tx, p, dr.DeviceFilters{}, "")
	if e != nil {
		return out, e
	}
	if len(current) != len(scope.Members) {
		return out, dr.Hidden()
	}
	allowed := scope.IDs("device")
	for _, snapshot := range proof.Devices {
		d := snapshot.Device
		if !slices.Contains(allowed, d.ID) || len(q.Filters.ManufacturerIDs) > 0 && !slices.Contains(q.Filters.ManufacturerIDs, d.Product.ManufacturerID) || len(q.Filters.ProductIDs) > 0 && !slices.Contains(q.Filters.ProductIDs, d.ProductID) || len(q.Filters.ProductVersionIDs) > 0 && !slices.Contains(q.Filters.ProductVersionIDs, d.ProductVersionID) || len(q.Filters.Statuses) > 0 && !slices.Contains(q.Filters.Statuses, d.Status) {
			continue
		}
		if q.Search != "" && !strings.Contains(strings.ToLower(d.Code+" "+d.Name), strings.ToLower(q.Search)) {
			continue
		}
		d.Classification = &snapshot.Classification
		if !displayCategoryMatches(d.Classification, q.Filters) {
			continue
		}
		out.Rows = append(out.Rows, dr.Bytes(DisplayDevice(d, scope.FieldGroups)))
	}
	out.Metadata = map[string]any{"dictionary_release_id": proof.ReleaseID, "population_basis": "fixed_parent_query"}
	out.Proof = raw
	return out, nil
}

type categoryStatisticsProof struct {
	ReleaseID string                   `json:"dictionary_release_id"`
	Devices   []categoryDeviceSnapshot `json:"devices"`
}

func categoryProjection(v classification.Node) dr.Category {
	return dr.Category{ID: v.ID, Code: v.Code, Name: v.Name, ParentID: v.ParentID, Description: v.Description, Order: v.Order, Selectable: v.Selectable, Status: v.Status, Path: v.Path, NamePath: v.NamePath, Sensor: v.Sensor}
}
func (s *Service) DisplayCategoryStatisticsTx(ctx context.Context, tx pgx.Tx, p dr.Plan) (dr.Materialized, error) {
	q := p.Request.(*dr.DeviceCategoryStatisticsRequest)
	out := dr.Materialized{Rows: []json.RawMessage{}}
	filters := q.Filters
	if q.ReleaseID != "" {
		if filters.DictionaryReleaseID != "" && filters.DictionaryReleaseID != q.ReleaseID {
			return out, dr.Invalid("/dictionary_release_id", "分类统计和筛选须使用同一字典版本。")
		}
		filters.DictionaryReleaseID = q.ReleaseID
	}
	devices, e := s.DisplayDevicesTx(ctx, tx, p, filters, "")
	if e != nil {
		return out, e
	}
	if q.Population == "monitoring_bound" {
		relations, e := s.DisplayRelationsTx(ctx, tx, p, "source")
		if e != nil {
			return out, e
		}
		devices = slices.DeleteFunc(devices, func(d Device) bool { return len(relations[d.ID]) == 0 })
	}
	var revision, releaseID string
	if e = tx.QueryRow(ctx, `SELECT revision::text,COALESCE(current_release_id::text,'') FROM device_classification_state WHERE id=1`).Scan(&revision, &releaseID); e != nil {
		return out, e
	}
	if q.ReleaseID != "" {
		releaseID = q.ReleaseID
	} else if filters.DictionaryReleaseID != "" {
		releaseID = filters.DictionaryReleaseID
	}
	nodes := map[string]classification.Node{}
	if releaseID != "" {
		release, e := classification.PublishedTx(ctx, tx, releaseID)
		if e != nil {
			return out, e
		}
		for _, n := range release.Content.Nodes {
			nodes[n.ID] = n
		}
	}
	for _, id := range q.FrontierIDs {
		n, ok := nodes[id]
		if !ok {
			return out, dr.Invalid("/frontier_ids", "Unknown category in fixed release.")
		}
		for _, ancestor := range n.Path {
			if ancestor != id && slices.Contains(q.FrontierIDs, ancestor) {
				return out, dr.Invalid("/frontier_ids", "Frontier categories must be mutually exclusive.")
			}
		}
	}
	direct, included, counts := map[string]int{}, map[string]int{}, map[string]int{}
	sensors, nonSensors, unclassified, unavailable := 0, 0, 0, 0
	sensorCategories := map[string]bool{}
	proof := categoryStatisticsProof{ReleaseID: releaseID, Devices: []categoryDeviceSnapshot{}}
	for _, d := range devices {
		effective := classification.Effective{State: "unclassified", Path: []string{}, NamePath: []string{}}
		if d.Classification != nil {
			effective = *d.Classification
		}
		effective.DictionaryReleaseID = releaseID
		keys := []string{}
		if effective.CategoryID == "" {
			unclassified++
			keys = []string{"unclassified"}
			effective.State = "unclassified"
		} else if n, ok := nodes[effective.CategoryID]; !ok {
			unavailable++
			keys = []string{"unavailable"}
			effective.State = "unavailable"
			effective.Path = []string{}
			effective.NamePath = []string{}
			effective.Name = ""
			effective.Sensor = nil
		} else {
			effective.State = "classified"
			effective.Name = n.Name
			effective.Path = n.Path
			effective.NamePath = n.NamePath
			effective.Sensor = &n.Sensor
			effective.CategoryStatus = n.Status
			if n.Sensor {
				sensors++
				sensorCategories[n.ID] = true
			} else {
				nonSensors++
			}
			direct[n.ID]++
			for _, id := range n.Path {
				included[id]++
			}
			switch q.Mode {
			case "direct":
				keys = []string{n.ID}
			case "inclusive":
				keys = slices.Clone(n.Path)
			case "frontier":
				for _, id := range q.FrontierIDs {
					if slices.Contains(n.Path, id) {
						keys = append(keys, id)
					}
				}
				if len(keys) == 0 {
					keys = []string{"outside_frontier"}
				}
			}
		}
		for _, key := range keys {
			counts[key]++
		}
		proof.Devices = append(proof.Devices, categoryDeviceSnapshot{Device: d, Classification: effective, Groups: keys})
	}
	keys := []string{}
	for key := range counts {
		keys = append(keys, key)
	}
	if q.Mode == "frontier" {
		for _, id := range q.FrontierIDs {
			if !slices.Contains(keys, id) {
				keys = append(keys, id)
			}
		}
	}
	slices.Sort(keys)
	for _, key := range keys {
		g := dr.CategoryGroup{Key: key, Label: key, Visibility: "full", DirectCount: strconv.Itoa(direct[key]), IncludedCount: strconv.Itoa(included[key]), Count: strconv.Itoa(counts[key])}
		if n, ok := nodes[key]; ok {
			v := categoryProjection(n)
			g.Category = &v
			g.Label = n.Name
		} else {
			g.Visibility = key
			g.DirectCount = g.Count
			g.IncludedCount = g.Count
		}
		out.Rows = append(out.Rows, dr.Bytes(g))
	}
	additivity := "disjoint"
	if q.Mode == "inclusive" {
		additivity = "overlapping"
	}
	out.Metadata = map[string]any{"definitions_version": "2", "population": q.Population, "group_by": "device_category", "mode": q.Mode, "additivity": additivity, "dictionary_release_id": releaseID, "classification_revision": revision, "classification_digest": dr.Hash(proof), "totals": dr.CategoryCounts{UniqueDeviceCount: strconv.Itoa(len(devices)), SensorDeviceCount: strconv.Itoa(sensors), UsedSensorCategoryCount: strconv.Itoa(len(sensorCategories)), NonSensorDeviceCount: strconv.Itoa(nonSensors), UnclassifiedDeviceCount: strconv.Itoa(unclassified), UnavailableDeviceCount: strconv.Itoa(unavailable)}, "count_basis": map[string]string{"deduplicate_by": "device_id", "classification": "fixed_dictionary_and_current_classification_records", "monitoring_bound": "current_authorized_source_bindings", "inclusive_counts": "never_sum_parent_and_child"}}
	out.Proof = dr.Bytes(proof)
	if len(devices) == 0 {
		out.State = "empty"
	}
	return out, nil
}
