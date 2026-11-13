package monitoringvalue

import (
	"errors"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata" // Named-zone contracts also apply in minimal worker containers.
)

var (
	MinTime = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	MaxTime = time.Unix(0, 1<<63-1).UTC()
	rfcTime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.[0-9]{1,9})?(?:Z|[+-](?:[01][0-9]|2[0-3]):[0-5][0-9])$`)
)

func CheckTime(t time.Time) error {
	if t.Before(MinTime) || t.After(MaxTime) {
		return errors.New("time_out_of_range")
	}
	return nil
}

func ParseTime(text string) (time.Time, error) {
	if !rfcTime.MatchString(text) {
		return time.Time{}, errors.New("explicit_offset_and_nanosecond_precision_required")
	}
	t, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}, errors.New("invalid_time")
	}
	if err := CheckTime(t); err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// ParseLocal requires a single explicit layout and a named zone. It checks all
// offset candidates around the civil date, rejecting DST folds and gaps.
func ParseLocal(text, layout, zone string) (time.Time, error) {
	if strings.Contains(layout, "Z") || strings.Contains(layout, "-07") || strings.Contains(layout, "MST") {
		return time.Time{}, errors.New("use_explicit_offset_parser")
	}
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" || layout == "" {
		return time.Time{}, errors.New("explicit_time_layout_and_zone_required")
	}
	civil, err := time.Parse(layout, text)
	if err != nil || civil.Format(layout) != text {
		return time.Time{}, errors.New("invalid_local_time")
	}
	// layout must describe a wall time, not silently override a source offset.
	_, offset := civil.Zone()
	if offset != 0 || civil.Location() != time.UTC {
		return time.Time{}, errors.New("use_explicit_offset_parser")
	}
	seen := map[int]bool{}
	candidates := map[int64]time.Time{}
	for h := -48; h <= 48; h++ {
		_, off := civil.Add(time.Duration(h) * time.Hour).In(loc).Zone()
		if seen[off] {
			continue
		}
		seen[off] = true
		t := civil.Add(-time.Duration(off) * time.Second)
		if t.In(loc).Format(layout) == text {
			if err := CheckTime(t); err != nil {
				return time.Time{}, err
			}
			candidates[t.UnixNano()] = t.UTC()
		}
	}
	if len(candidates) != 1 {
		return time.Time{}, errors.New("ambiguous_or_nonexistent_local_time_requires_offset")
	}
	for _, t := range candidates {
		return t, nil
	}
	panic("unreachable")
}
