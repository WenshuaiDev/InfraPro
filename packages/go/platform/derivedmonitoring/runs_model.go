package derivedmonitoring

import (
	"math"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

type NanosecondRange struct {
	From string  `json:"from_ns"`
	To   *string `json:"to_ns"`
}

func (r NanosecondRange) rangeValue() (planning.Range, error) {
	from, e := strconv.ParseInt(r.From, 10, 64)
	if e != nil || strconv.FormatInt(from, 10) != r.From || from < monitoringvalue.MinTime.UnixNano() {
		return planning.Range{}, invalid("/output_range/from_ns", "需要存储时间域内的纳秒十进制字符串。")
	}
	out := planning.Range{From: from}
	if r.To != nil {
		to, e := strconv.ParseInt(*r.To, 10, 64)
		if e != nil || strconv.FormatInt(to, 10) != *r.To || to <= from {
			return out, invalid("/output_range/to_ns", "结束边界必须晚于起点；时间域上界使用 null。")
		}
		out.To = &to
	}
	return out, nil
}
func rangeSQL(r planning.Range) string {
	upper := ""
	if r.To != nil {
		upper = strconv.FormatInt(*r.To, 10)
	}
	return "[" + strconv.FormatInt(r.From, 10) + "," + upper + ")"
}
func rangeDTO(from int64, to *int64) NanosecondRange {
	out := NanosecondRange{From: strconv.FormatInt(from, 10)}
	if to != nil {
		v := strconv.FormatInt(*to, 10)
		out.To = &v
	}
	return out
}
func runOptions(r planning.Range, source *planning.Range, at int64) compute.RunOptions {
	end := int64(math.MaxInt64)
	if r.To != nil {
		end = *r.To
	}
	out := compute.RunOptions{OutputRange: compute.TimeRange{From: r.From, To: end}, SourceRange: compute.TimeRange{From: r.From, To: end}, IncludeMax: r.To == nil, NowNS: at}
	if source != nil {
		out.SourceRange.From = source.From
		if source.To != nil {
			out.SourceRange.To = *source.To
		}
	}
	return out
}

type TrialInput struct {
	WriteMeta
	ConfigID    string          `json:"config_id"`
	ConfigHash  string          `json:"config_hash"`
	OutputRange NanosecondRange `json:"output_range"`
}
type Run struct {
	ID          string                    `json:"id"`
	ChannelID   string                    `json:"channel_id,omitempty"`
	ConfigID    string                    `json:"config_id,omitempty"`
	Mode        string                    `json:"mode"`
	OutputRange NanosecondRange           `json:"output_range"`
	PlanHash    string                    `json:"compiled_plan_hash"`
	CreatedAt   time.Time                 `json:"created_at"`
	Job         monitoring.Job            `json:"job"`
	Trial       *TrialResult              `json:"trial,omitempty"`
	Replay      *ReplayResult             `json:"historical_replay,omitempty"`
	Analysis    *AnalysisRunInfo          `json:"analysis,omitempty"`
	Comparison  *AnalysisComparisonResult `json:"comparison,omitempty"`
	Replayed    bool                      `json:"replayed"`
}
type RunPage struct {
	Items      []Run  `json:"items"`
	NextCursor string `json:"next_cursor"`
}
type RunFilter struct {
	ChannelID, State, Cursor string
	Limit                    int
}
type TrialMatch struct {
	Reference string `json:"reference"`
	ID        string `json:"id"`
	At        string `json:"at_ns"`
	Offset    string `json:"offset_ns"`
}
type TrialValue struct {
	PreviewIssue string           `json:"preview_issue,omitempty"`
	Output       string           `json:"output_key"`
	At           string           `json:"at_ns"`
	Interval     *NanosecondRange `json:"interval"`
	Value        *string          `json:"value"`
	Scale        int              `json:"scale"`
	Rounded      bool             `json:"rounded"`
	Status       string           `json:"calculation_status"`
	Quality      string           `json:"quality"`
	Reasons      []string         `json:"reason_flags"`
	Count        string           `json:"count"`
	Excluded     string           `json:"excluded_count"`
	Matches      []TrialMatch     `json:"matches"`
}
type TrialNode struct {
	NodeID   string            `json:"node_id"`
	Samples  string            `json:"sample_count"`
	States   map[string]string `json:"status_counts"`
	Examples []TrialValue      `json:"examples"`
}
type TrialResult struct {
	Sources              []inputSourceProof                 `json:"upstream_inputs"`
	DerivedInputExamples []monitoring.CalculatedObservation `json:"derived_input_examples"`
	ReceiptID            string                             `json:"trial_receipt_id"`
	ExpiresAt            time.Time                          `json:"expires_at"`
	PlanHash             string                             `json:"compiled_plan_hash"`
	InputSnapshot        monitoring.RawSnapshot             `json:"input_snapshot"`
	InputRanges          []monitoring.RawNeed               `json:"input_ranges"`
	Baselines            []BaselineProof                    `json:"baselines"`
	InputCounts          monitoring.RawReadCount            `json:"input_counts"`
	InputExamples        []monitoring.RawSample             `json:"input_examples"`
	OutputRange          NanosecondRange                    `json:"output_range"`
	OutputCount          string                             `json:"output_count"`
	States               map[string]string                  `json:"status_counts"`
	Nodes                []TrialNode                        `json:"nodes"`
	Examples             []TrialValue                       `json:"examples"`
	WaitingForInput      bool                               `json:"waiting_for_input"`
	ExamplesTruncated    bool                               `json:"examples_truncated"`
	Stale                bool                               `json:"stale"`
}
