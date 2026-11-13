package compute

import "errors"

func unitEqual(a, b Quantity) bool {
	return a.BaseUnitVersionID == b.BaseUnitVersionID && a.BasePower == b.BasePower && a.TimeUnit == b.TimeUnit && a.TimePower == b.TimePower
}
func scalable(q Quantity) bool      { return q.Mode != "level" || q.LevelScale == "ratio" }
func dimensionless(q Quantity) bool { return q.BasePower == 0 && q.TimePower == 0 }
func scalar() Quantity {
	return Quantity{Mode: "ratio", LevelScale: "ratio", DimensionlessKind: "ratio"}
}
func validateQuantity(q Quantity) error {
	if q.Mode != "level" && q.Mode != "difference" && q.Mode != "ratio" && q.Mode != "count" {
		return errors.New("quantity_mode required")
	}
	if q.LevelScale != "unknown" && q.LevelScale != "ratio" && q.LevelScale != "affine" {
		return errors.New("level_scale required")
	}
	if q.Mode != "level" && q.LevelScale != "ratio" {
		return errors.New("difference/ratio/count must use ratio scale")
	}
	if q.BasePower < 0 || q.BasePower > 64 || q.TimePower > 0 || q.TimePower < -32 {
		return errors.New("unit exponent out of range")
	}
	if (q.BasePower == 0) != (q.BaseUnitVersionID == "") {
		return errors.New("base unit and exponent required together")
	}
	if (q.TimePower == 0) != (q.TimeUnit == "") {
		return errors.New("time unit and exponent required together")
	}
	if q.TimeUnit != "" && q.TimeUnit != "second" && q.TimeUnit != "minute" && q.TimeUnit != "hour" && q.TimeUnit != "day" {
		return errors.New("unsupported time unit")
	}
	if q.Mode == "ratio" || q.Mode == "count" {
		if !dimensionless(q) || q.DimensionlessKind != q.Mode {
			return errors.New("dimensionless semantics mismatch")
		}
	} else if q.DimensionlessKind != "" {
		return errors.New("dimensionless_kind not allowed")
	}
	return nil
}
func quantityBinary(op string, a, b Quantity) (Quantity, error) {
	bad := func() (Quantity, error) { return Quantity{}, errors.New("incompatible units or quantity semantics") }
	switch op {
	case "add", "subtract", "min", "max":
		if !unitEqual(a, b) {
			return bad()
		}
		if op == "min" || op == "max" {
			if a != b {
				return bad()
			}
			return a, nil
		}
		if a.Mode == "level" && b.Mode == "level" {
			if a.LevelScale != b.LevelScale {
				return bad()
			}
			if op == "subtract" {
				a.Mode = "difference"
				a.LevelScale = "ratio"
				return a, nil
			}
			if a.LevelScale != "ratio" {
				return bad()
			}
			return a, nil
		}
		if a.Mode == "level" && b.Mode == "difference" {
			return a, nil
		}
		if op == "add" && a.Mode == "difference" && b.Mode == "level" {
			return b, nil
		}
		if a.Mode != b.Mode || a.LevelScale != b.LevelScale {
			return bad()
		}
		return a, nil
	case "multiply":
		if !scalable(a) || !scalable(b) {
			return bad()
		}
		if dimensionless(a) {
			return b, nil
		}
		if dimensionless(b) {
			return a, nil
		}
		return bad()
	case "divide":
		if !scalable(a) || !scalable(b) {
			return bad()
		}
		if unitEqual(a, b) && a.Mode == b.Mode {
			return scalar(), nil
		}
		if dimensionless(b) {
			return a, nil
		}
		return bad()
	}
	return bad()
}
func quantityUnary(op string, q Quantity, params Params) (Quantity, error) {
	switch op {
	case "abs":
		if !scalable(q) {
			return Quantity{}, errors.New("absolute value requires a ratio scale")
		}
	case "baseline_ratio":
		if !scalable(q) {
			return Quantity{}, errors.New("baseline ratio requires a ratio scale")
		}
		return scalar(), nil
	case "window_count":
		return Quantity{Mode: "count", LevelScale: "ratio", DimensionlessKind: "count"}, nil
	case "window_sum":
		if !scalable(q) {
			return Quantity{}, errors.New("sum requires an additive quantity")
		}
	case "difference", "rate", "baseline_delta", "window_range", "window_stddev", "window_variance":
		if q.Mode == "level" {
			q.Mode = "difference"
			q.LevelScale = "ratio"
		}
		if op == "window_variance" {
			q.BasePower *= 2
			q.TimePower *= 2
		}
		if op == "rate" {
			if q.TimePower != 0 && q.TimeUnit != params.RateTimeUnit {
				return Quantity{}, errors.New("mixed rate time units are not implicit conversions")
			}
			q.TimeUnit = params.RateTimeUnit
			q.TimePower--
			if q.Mode == "ratio" || q.Mode == "count" {
				q.Mode = "difference"
				q.DimensionlessKind = ""
			}
		}
	}
	return q, validateQuantity(q)
}
