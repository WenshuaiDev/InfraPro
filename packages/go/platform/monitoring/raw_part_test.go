package monitoring

import (
	"testing"
)

func TestRawSnapshotPartRetainsOnlySelectedIntegrityProofs(t *testing.T) {
	lo, hi := int64(100), int64(200)
	one, two := "12da9077-155f-45f0-b0de-f0c2b46b2ad0", "5e701d9d-434e-4562-8171-8e13ad7b0d17"
	needs := []RawNeed{{Channel: one, From: &lo, To: &hi}, {Channel: two, From: &lo, To: &hi}}
	original := rawEvidence{Needs: needs, Candidates: needs, Integrity: map[string]string{one: "1", two: "8"}}
	part, e := selectRawPart(original, needs[:1])
	if e != nil {
		t.Fatal(e)
	}
	if len(part.Needs) != 1 || len(part.Candidates) != 1 || len(part.Integrity) != 1 || part.Integrity[one] != "1" {
		t.Fatal(part)
	}
	if len(original.Integrity) != 2 || original.Integrity[two] != "8" || len(original.Needs) != 2 {
		t.Fatal("part mutation changed the original proof")
	}
	outside := int64(99)
	if _, e = selectRawPart(original, []RawNeed{{Channel: one, From: &outside, To: &hi}}); e == nil {
		t.Fatal("part escaped immutable snapshot range")
	}
}
