package spatial

import "math"

const maxMercatorLatitude = 85.0511287798066

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func vector(v []float64, n int, normalized bool) bool {
	if len(v) != n {
		return false
	}
	for _, x := range v {
		if !finite(x) || math.Abs(x) > 1e12 || normalized && (x < 0 || x > 1) {
			return false
		}
	}
	return true
}
func validateBBox(b []float64) error {
	if !vector(b, 4, false) || b[0] < -180 || b[0] > 180 || b[2] < -180 || b[2] > 180 || b[1] < -90 || b[3] > 90 || b[1] > b[3] {
		return invalid("/bbox", "地理范围无效；跨日期变更线可使用西界大于东界。")
	}
	return nil
}
func InBBox(lon, lat float64, b []float64) bool {
	if len(b) == 0 {
		return true
	}
	longitude := lon >= b[0] && lon <= b[2]
	if b[0] > b[2] {
		longitude = lon >= b[0] || lon <= b[2]
	}
	return longitude && lat >= b[1] && lat <= b[3]
}

// Geographic explicitly supports only the two declared horizontal systems.
func Geographic(crs string, x, y float64) (float64, float64, error) {
	if !finite(x) || !finite(y) {
		return 0, 0, invalid("/position", "坐标必须有限。")
	}
	switch crs {
	case "EPSG:4326":
		if x < -180 || x > 180 || math.Abs(y) > maxMercatorLatitude {
			return 0, 0, invalid("/position", "坐标超出当前地图可显示范围。")
		}
		return x, y, nil
	case "EPSG:3857":
		lon := x / 6378137 * 180 / math.Pi
		lat := (2*math.Atan(math.Exp(y/6378137)) - math.Pi/2) * 180 / math.Pi
		return Geographic("EPSG:4326", lon, lat)
	default:
		return 0, 0, invalid("/position/crs", "此坐标系尚不支持地理显示；原始位置保留。")
	}
}
