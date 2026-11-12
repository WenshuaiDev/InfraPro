package monitoring

import (
	"context"
	"slices"
	"testing"
)

func TestIdentityVerificationAcrossAdoptions(t *testing.T) {
	original := rawObservation{Channel: "channel", Observation: "observation", Adoption: "first", Batch: "original", At: 1, Value: "10000000000000000000000000.000000000001", First: true}
	readopt := original
	readopt.Adoption, readopt.Batch, readopt.Mapping, readopt.First = "second", "new batch", 2, false
	for _, test := range []struct {
		name   string
		rows   []rawObservation
		active []bool
		code   string
	}{
		{"readopt preserves identity", []rawObservation{readopt, original}, []bool{true, false}, ""},
		{"two live adoptions", []rawObservation{readopt, original}, []bool{true, true}, "duplicate_active_observation"},
		{"missing original", []rawObservation{readopt}, []bool{true}, "first_adoption_identity_mismatch"},
		{"repeated adoption", []rawObservation{original, original}, []bool{false, false}, "duplicate_adoption_identity"},
		{"changed retained value", []rawObservation{original, func() rawObservation { r := readopt; r.Value = "2"; return r }()}, []bool{false, true}, "observation_identity_payload_mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			v, e := newIdentityVerifier(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			defer v.close()
			for i, row := range test.rows {
				if e = v.add(row, test.active[i]); e != nil {
					t.Fatal(e)
				}
			}
			var codes []string
			if e = v.verify(context.Background(), func(_, code string) { codes = append(codes, code) }); e != nil {
				t.Fatal(e)
			}
			if test.code == "" && len(codes) != 0 || test.code != "" && !slices.Contains(codes, test.code) {
				t.Fatal(codes)
			}
		})
	}
}
