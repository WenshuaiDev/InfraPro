package spatial

import (
	"encoding/json"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
)

func DisplayFacilityMap(items []displayread.FacilityMapItem, q displayread.MapFacilitiesRequest) displayread.Materialized {
	result := displayread.Materialized{Rows: []json.RawMessage{}, Metadata: map[string]any{}}
	counts := map[string]int{"located": 0, "missing": 0, "unsupported": 0}
	hits := 0
	for _, item := range items {
		item.PositionState = "missing"
		item.Notice = "仅有桩号、局部位置或尚未提供地理坐标。"
		var crs string
		var x, y *float64
		if p := item.Position; p != nil {
			if p.Geographic != nil {
				crs, x, y = p.Geographic.CRS, p.Geographic.Longitude, p.Geographic.Latitude
			} else if p.Projected != nil {
				crs, x, y = p.Projected.CRS, p.Projected.X, p.Projected.Y
				if p.Projected.Unit != "m" {
					crs = "unsupported-unit"
				}
			}
		}
		if x != nil && y != nil {
			lon, lat, e := Geographic(crs, *x, *y)
			if e != nil {
				item.PositionState = "unsupported"
				item.Notice = "坐标系或单位暂不支持地理转换。"
			} else {
				item.PositionState = "located"
				item.Notice = "依据设施自身工程位置转换。"
				item.Longitude, item.Latitude = &lon, &lat
				item.CoordinateCRS = "EPSG:4326"
			}
		}
		counts[item.PositionState]++
		if q.BBox != nil && (item.Longitude == nil || !q.BBox.Contains(*item.Longitude, *item.Latitude)) {
			continue
		}
		if q.PositionState != "" && q.PositionState != "all" && q.PositionState != item.PositionState {
			continue
		}
		hits++
		result.Rows = append(result.Rows, displayread.Bytes(item))
	}
	states := map[string]string{}
	for key, n := range counts {
		states[key] = strconv.Itoa(n)
	}
	result.Metadata["position_counts"] = states
	result.Metadata["matched_count"] = strconv.Itoa(hits)
	return result
}
