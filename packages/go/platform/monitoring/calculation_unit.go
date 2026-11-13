package monitoring

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Display only: the persisted quantity expression remains the semantic truth.
// Do not infer a unit conversion or a physical meaning from its display name.
func calculationUnit(symbol string, raw json.RawMessage) (string, error) {
	var q struct {
		BasePower int    `json:"base_power"`
		TimeUnit  string `json:"time_unit"`
		TimePower int    `json:"time_power"`
	}
	if e := json.Unmarshal(raw, &q); e != nil {
		return "", e
	}
	if q.BasePower < 0 || q.BasePower > 64 || q.TimePower > 0 || q.TimePower < -32 {
		return "", fmt.Errorf("invalid persisted quantity exponent")
	}
	power := func(unit string, n int) string {
		if n == 1 {
			return unit
		}
		if n == 2 {
			return unit + "²"
		}
		if n == 3 {
			return unit + "³"
		}
		return unit + "^" + strconv.Itoa(n)
	}
	base := "1"
	if q.BasePower > 0 {
		if symbol == "" {
			return "", fmt.Errorf("missing persisted base unit symbol")
		}
		base = power(symbol, q.BasePower)
	}
	if q.TimePower < 0 {
		unit := map[string]string{"second": "s", "minute": "min", "hour": "h", "day": "d"}[q.TimeUnit]
		if unit == "" {
			return "", fmt.Errorf("unsupported persisted time unit")
		}
		base += "/" + power(unit, -q.TimePower)
	}
	return base, nil
}
