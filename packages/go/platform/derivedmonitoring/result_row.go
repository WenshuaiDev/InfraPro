package derivedmonitoring

import (
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"math/big"
	"strings"
)

func channelResultRow(run runRecord, generation, lineage string, result compute.Result) (resultstore.Row, error) {
	id, e := resultstore.ResultID(run.resultSeries(), result.OutputKey, result.At)
	if e != nil {
		return resultstore.Row{}, e
	}
	row := resultstore.Row{Tenant: run.Scope.Tenant, Series: run.ChannelID, Channel: &run.ChannelID, Result: id, Generation: generation, Run: run.ID, Config: run.ConfigID, Output: result.OutputKey, At: result.At, Scale: uint8(result.Scale), Rounded: result.Rounded, Quality: result.Quality, Status: result.Status, Reasons: result.Reasons, Count: result.Count, Excluded: result.Excluded, InputFirst: result.InputFrom, InputLast: result.InputTo, Lineage: lineage}
	if run.Analysis != nil {
		row.Series = run.Analysis.ArtifactID
		row.Channel = nil
		row.Artifact = &run.Analysis.ArtifactID
	}
	if result.Interval != nil {
		row.Start = &result.Interval.From
		row.End = &result.Interval.To
	}
	if result.Value != nil {
		coefficient, ok := new(big.Int).SetString(strings.Replace(*result.Value, ".", "", 1), 10)
		if !ok {
			return row, fmt.Errorf("invalid_result_encoding")
		}
		value := coefficient.String()
		row.Coefficient = &value
	}
	return row, nil
}
func (run runRecord) resultSeries() string {
	if run.Analysis != nil {
		return run.Analysis.ArtifactID
	}
	return run.ChannelID
}
