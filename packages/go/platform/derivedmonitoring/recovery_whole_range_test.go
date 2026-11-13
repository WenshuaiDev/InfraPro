package derivedmonitoring

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"testing"
)

func TestWholeRangeLabelIsOnlyValidAtItsSealedBoundary(t *testing.T) {
	start, end := int64(100), int64(200)
	interval := rangeDTO(start, &end)
	manifest := resultManifest{Range: interval, SourceRange: &interval}
	row := resultstore.Row{At: end, Start: &start, End: &end}
	if !publishedRowTime(manifest, row, true) || publishedRowTime(manifest, row, false) {
		t.Fatal("whole-range end label was confused with ordinary half-open output")
	}
	wrong := start + 1
	row.Start = &wrong
	if publishedRowTime(manifest, row, true) {
		t.Fatal("incorrect whole-range provenance interval accepted")
	}
	proof := analysisArtifactProof{WholeRange: true, Version: analysisArtifactRecord{AnalysisArtifactVersion: AnalysisArtifactVersion{OutputRange: interval}}}
	if !analysisIncludesWholeEnd(proof, &start, &end) || analysisIncludesWholeEnd(proof, &wrong, &end) {
		t.Fatal("whole-range selection changed the original aggregation interval")
	}
}
