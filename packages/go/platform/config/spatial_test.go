package config

import "testing"

func TestSpatialBasemapConfiguration(t *testing.T) {
	valid := `[{"id":"survey","name":"Survey","url":"/tiles/{z}/{x}/{y}.png","attribution":"Synthetic","min_zoom":0,"max_zoom":18}]`
	if got, err := spatialBasemaps(valid); err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	for _, raw := range []string{
		valid + `[]`,
		`[{"id":"local","name":"x","url":"/tiles/{z}/{x}/{y}","attribution":"x"}]`,
		`[{"id":"x","name":"x","url":"http://example.test/{z}/{x}/{y}","attribution":"x"}]`,
		`[{"id":"x","name":"x","url":"//example.test/{z}/{x}/{y}","attribution":"x"}]`,
		`[{"id":"x","name":"x","url":"https://user:password@example.test/{z}/{x}/{y}","attribution":"x"}]`,
		`[{"id":"x","name":"x","url":"/tiles/{z}/{x}/{y}","attribution":"x","max_zoom":23}]`,
	} {
		if _, err := spatialBasemaps(raw); err == nil {
			t.Fatalf("invalid basemap accepted: %s", raw)
		}
	}
}
