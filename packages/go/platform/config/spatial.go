package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
)

type SpatialBasemap struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Attribution string `json:"attribution"`
	MinZoom     int    `json:"min_zoom"`
	MaxZoom     int    `json:"max_zoom"`
}

func spatialBasemaps(raw string) ([]SpatialBasemap, error) {
	out := []SpatialBasemap{}
	if raw == "" {
		return out, nil
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > 16384 || decoder.Decode(&out) != nil || len(out) > 8 || decoder.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("spatial_basemaps must be a JSON array of at most eight XYZ sources")
	}
	seen := map[string]bool{"blank": true, "local": true}
	for _, b := range out {
		u, e := url.Parse(strings.NewReplacer("{z}", "0", "{x}", "0", "{y}", "0").Replace(b.URL))
		if e != nil || u.User != nil || u.Fragment != "" || !(u.Scheme == "https" && u.Host != "" || u.Scheme == "" && u.Host == "" && strings.HasPrefix(b.URL, "/") && !strings.HasPrefix(b.URL, "//")) || !strings.Contains(b.URL, "{z}") || !strings.Contains(b.URL, "{x}") || !strings.Contains(b.URL, "{y}") || strings.TrimSpace(b.Name) == "" || strings.TrimSpace(b.Attribution) == "" || !regexp.MustCompile(`^[a-z][a-z0-9_-]{0,39}$`).MatchString(b.ID) || seen[b.ID] || b.MinZoom < 0 || b.MaxZoom > 22 || b.MaxZoom < b.MinZoom {
			return nil, fmt.Errorf("invalid spatial XYZ source; use a unique ID, HTTPS or same-origin path, attribution and zoom 0–22")
		}
		seen[b.ID] = true
	}
	return out, nil
}
