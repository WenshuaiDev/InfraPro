package monitoring

import (
	"cmp"
	"encoding/json"
	"slices"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring/internal/observation"
)

// Window represents a half-open UTC nanosecond range. nil endpoints are truly
// unbounded, avoiding a maximum-timestamp-plus-one overflow sentinel.
type Window struct{ From, To *int64 }

func (r *Range) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if e := json.Unmarshal(raw, &fields); e != nil {
		return e
	}
	if len(fields) != 2 || fields["from"] == nil || fields["to"] == nil {
		return invalid("range", "请显式填写 from/to；无界端点使用 null。")
	}
	type plain Range
	var value plain
	if e := json.Unmarshal(raw, &value); e != nil {
		return e
	}
	*r = Range(value)
	return nil
}

func (r Range) window() (Window, error) {
	out := Window{}
	for _, entry := range []struct {
		text *string
		dest **int64
		name string
	}{{r.From, &out.From, "range.from"}, {r.To, &out.To, "range.to"}} {
		if entry.text != nil {
			tm, e := observation.ParseTime(*entry.text)
			if e != nil {
				return out, invalid(entry.name, "需要存储范围内的带偏移纳秒时刻。")
			}
			n := tm.UnixNano()
			*entry.dest = &n
		}
	}
	if out.From != nil && out.To != nil && *out.From >= *out.To {
		return out, invalid("range", "起点必须早于终点。")
	}
	return out, nil
}
func (w Window) rangeDTO() Range {
	out := Range{}
	if w.From != nil {
		s := time.Unix(0, *w.From).UTC().Format(time.RFC3339Nano)
		out.From = &s
	}
	if w.To != nil {
		s := time.Unix(0, *w.To).UTC().Format(time.RFC3339Nano)
		out.To = &s
	}
	return out
}
func (w Window) pg() string {
	lo, hi := "", ""
	if w.From != nil {
		lo = strconv.FormatInt(*w.From, 10)
	}
	if w.To != nil {
		hi = strconv.FormatInt(*w.To, 10)
	}
	return "[" + lo + "," + hi + ")"
}
func (w Window) Contains(at int64, start *int64) bool {
	first := at
	if start != nil {
		first = *start
		if first >= at {
			return false
		}
	}
	return (w.From == nil || first >= *w.From) && (w.To == nil || at < *w.To)
}
func (w Window) Covers(other Window) bool {
	return (w.From == nil || (other.From != nil && *w.From <= *other.From)) && (w.To == nil || (other.To != nil && *w.To >= *other.To))
}
func (w Window) Intersect(other Window) (Window, bool) {
	out := w
	if out.From == nil || (other.From != nil && *other.From > *out.From) {
		out.From = other.From
	}
	if out.To == nil || (other.To != nil && *other.To < *out.To) {
		out.To = other.To
	}
	return out, out.From == nil || out.To == nil || *out.From < *out.To
}
func UnionWindows(windows []Window) []Window {
	values := slices.Clone(windows)
	slices.SortFunc(values, func(a, b Window) int {
		if a.From == nil {
			if b.From == nil {
				return 0
			}
			return -1
		}
		if b.From == nil {
			return 1
		}
		return cmp.Compare(*a.From, *b.From)
	})
	out := []Window{}
	for _, w := range values {
		if w.From != nil && w.To != nil && *w.From >= *w.To {
			continue
		}
		if len(out) == 0 {
			out = append(out, w)
			continue
		}
		last := &out[len(out)-1]
		if last.To == nil {
			continue
		}
		if w.From == nil || *w.From <= *last.To {
			if w.To == nil || *w.To > *last.To {
				last.To = w.To
			}
		} else {
			out = append(out, w)
		}
	}
	return out
}
