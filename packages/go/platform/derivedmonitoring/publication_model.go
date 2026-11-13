package derivedmonitoring

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

type resultManifest struct {
	Artifact    string                 `json:"artifact_id,omitempty"`
	Outputs     []Output               `json:"outputs,omitempty"`
	SourceRange *NanosecondRange       `json:"source_range,omitempty"`
	Schema      string                 `json:"schema"`
	Generation  string                 `json:"generation_id"`
	Run         string                 `json:"run_id"`
	Channel     string                 `json:"channel_id"`
	Config      string                 `json:"config_id"`
	Plan        string                 `json:"compiled_plan_hash"`
	Range       NanosecondRange        `json:"output_range"`
	Input       monitoring.RawSnapshot `json:"input_snapshot"`
	Baselines   []BaselineProof        `json:"baselines"`
	Sources     []inputSourceProof     `json:"upstream_inputs,omitempty"`
	Lineage     string                 `json:"lineage_root"`
	Blocks      []resultstore.Block    `json:"blocks"`
	Rows        uint64                 `json:"row_count,string"`
	States      map[string]string      `json:"status_counts"`
}
type resultPart struct {
	Schema   string              `json:"schema"`
	Plan     string              `json:"plan_hash"`
	Snapshot string              `json:"snapshot_hash"`
	Range    NanosecondRange     `json:"output_range"`
	Split    *string             `json:"split_at_ns"`
	Blocks   []resultstore.Block `json:"blocks"`
	Rows     uint64              `json:"row_count,string"`
	States   map[string]string   `json:"status_counts"`
}
type publicationTarget struct{ Generation, RequiredPosition, GraphRevision string }
