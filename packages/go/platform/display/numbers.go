package display

import (
	"encoding/json"
	"fmt"
	"math/big"
)

func validNumber(s Schema, v any) bool {
	switch v.(type) {
	case json.Number, float64, int, int64:
	default:
		return false
	}
	n, ok := new(big.Rat).SetString(fmt.Sprint(v))
	if !ok || s.Type == "integer" && !n.IsInt() {
		return false
	}
	if s.Minimum != nil {
		min, ok := new(big.Rat).SetString(s.Minimum.String())
		if !ok || n.Cmp(min) < 0 {
			return false
		}
	}
	if s.Maximum != nil {
		max, ok := new(big.Rat).SetString(s.Maximum.String())
		if !ok || n.Cmp(max) > 0 {
			return false
		}
	}
	return true
}
