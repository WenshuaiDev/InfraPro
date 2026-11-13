// Package resultstore owns immutable physical calculation payloads. Only a
// PostgreSQL publication may select its sealed blocks for a current/history read.
package resultstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/google/uuid"
)

const LayoutVersion = "derived-results/1"
const MaxBlockRows = 1_000_000
const MaxBlockBytes = 128 << 20

var statuses = []string{"valid", "baseline_invalid", "noncausal_baseline", "configuration_boundary", "invalid_input", "missing_input", "insufficient_samples", "gap_exceeded", "age_exceeded", "division_by_zero", "numeric_out_of_range", "numeric_underflow", "precision_unresolved"}

type Row struct {
	Tenant      string   `json:"tenant_id"`
	Series      string   `json:"series_id"`
	Channel     *string  `json:"channel_id"`
	Artifact    *string  `json:"artifact_id"`
	Result      string   `json:"result_id"`
	Generation  string   `json:"generation_id"`
	Run         string   `json:"run_id"`
	Config      string   `json:"config_id"`
	Output      string   `json:"output_key"`
	At          int64    `json:"output_at_ns,string"`
	Start       *int64   `json:"interval_start_ns,string"`
	End         *int64   `json:"interval_end_ns,string"`
	Coefficient *string  `json:"coefficient"`
	Scale       uint8    `json:"scale"`
	Rounded     bool     `json:"rounded"`
	Quality     string   `json:"quality"`
	Status      string   `json:"calculation_status"`
	Reasons     []string `json:"reason_flags"`
	Count       uint64   `json:"count,string"`
	Excluded    uint64   `json:"excluded_count,string"`
	InputFirst  *int64   `json:"input_first_ns,string"`
	InputLast   *int64   `json:"input_last_ns,string"`
	Lineage     string   `json:"lineage_ref"`
}

func ResultID(series, output string, at int64) (string, error) {
	namespace, e := uuid.Parse(series)
	if e != nil || output == "" {
		return "", fmt.Errorf("invalid_result_identity")
	}
	return uuid.NewSHA1(namespace, []byte(output+":"+strconv.FormatInt(at, 10))).String(), nil
}
func (r Row) Validate() error {
	if len(r.Output) == 0 || len(r.Output) > 128 || len(r.Reasons) > 64 {
		return fmt.Errorf("invalid_result_contract")
	}
	for _, reason := range r.Reasons {
		if len(reason) == 0 || len(reason) > 128 || strings.ContainsAny(reason, "\x00\r\n") {
			return fmt.Errorf("invalid_result_reason")
		}
	}
	for _, id := range []string{r.Tenant, r.Series, r.Result, r.Generation, r.Run, r.Config} {
		parsed, e := uuid.Parse(id)
		if e != nil || parsed.String() != id {
			return fmt.Errorf("invalid_result_identity")
		}
	}
	if (r.Channel == nil) == (r.Artifact == nil) || r.Channel != nil && *r.Channel != r.Series {
		return fmt.Errorf("invalid_result_owner")
	}
	if r.Artifact != nil {
		id, e := uuid.Parse(*r.Artifact)
		if e != nil || id.String() != *r.Artifact {
			return fmt.Errorf("invalid_artifact_identity")
		}
	}
	expected, e := ResultID(r.Series, r.Output, r.At)
	if e != nil || expected != r.Result {
		return fmt.Errorf("invalid_logical_result_id")
	}
	if r.Scale > 60 || !slices.Contains(statuses, r.Status) || !slices.Contains([]string{"good", "unmarked", "suspect", "bad"}, r.Quality) {
		return fmt.Errorf("invalid_result_contract")
	}
	if (r.Status == "valid") != (r.Coefficient != nil) {
		return fmt.Errorf("invalid_result_nullability")
	}
	if r.Coefficient != nil {
		n, ok := new(big.Int).SetString(*r.Coefficient, 10)
		if !ok || n.String() != *r.Coefficient || new(big.Int).Abs(n).Cmp(monitoringvalue.Power10(76)) >= 0 {
			return fmt.Errorf("invalid_result_coefficient")
		}
	}
	if e = monitoringvalue.CheckTime(time.Unix(0, r.At)); e != nil {
		return e
	}
	if (r.Start == nil) != (r.End == nil) || r.Start != nil && (*r.Start >= *r.End || *r.End != r.At) {
		return fmt.Errorf("invalid_result_interval")
	}
	for _, at := range []*int64{r.Start, r.End, r.InputFirst, r.InputLast} {
		if at != nil {
			if e = monitoringvalue.CheckTime(time.Unix(0, *at)); e != nil {
				return e
			}
		}
	}
	if (r.InputFirst == nil) != (r.InputLast == nil) || r.InputFirst != nil && *r.InputFirst > *r.InputLast {
		return fmt.Errorf("invalid_result_input_bounds")
	}
	if len(r.Lineage) != 64 {
		return fmt.Errorf("invalid_lineage_reference")
	}
	if _, e = hex.DecodeString(r.Lineage); e != nil {
		return fmt.Errorf("invalid_lineage_reference")
	}
	return nil
}
func (r Row) Hash() string {
	r.Reasons = append([]string{}, r.Reasons...)
	raw, _ := json.Marshal(r)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func (r Row) Decimal() *string {
	if r.Coefficient == nil {
		return nil
	}
	n, ok := new(big.Int).SetString(*r.Coefficient, 10)
	if !ok {
		return nil
	}
	v := monitoringvalue.Format(n, int(r.Scale))
	return &v
}

// Endpoints are sealed with the immutable manifest and verified against every
// payload row before publication. Old blocks without this index remain readable.
type Endpoints struct {
	Last      Row            `json:"last"`
	LastValid map[string]Row `json:"last_valid_by_quality"`
}

func (v *Endpoints) Add(row Row) {
	row.Reasons = append([]string{}, row.Reasons...)
	v.Last = row
	if v.LastValid == nil {
		v.LastValid = map[string]Row{}
	}
	if row.Status == "valid" {
		v.LastValid[row.Quality] = row
	}
}

type Block struct {
	Summary    monitoring.CalculationSummary `json:"summary,omitempty"`
	Endpoints  *Endpoints                    `json:"endpoints,omitempty"`
	Output     string                        `json:"output_key,omitempty"`
	First      *int64                        `json:"first_output_ns,omitempty,string"`
	Last       *int64                        `json:"last_output_ns,omitempty,string"`
	Tenant     string                        `json:"tenant_id"`
	Chunk      string                        `json:"chunk_id"`
	Attempt    string                        `json:"attempt_id"`
	Physical   string                        `json:"physical_shard"`
	Rows       uint64                        `json:"row_count,string"`
	Bytes      uint64                        `json:"payload_bytes,string"`
	Hash       string                        `json:"payload_hash"`
	ObjectKey  string                        `json:"object_key"`
	ObjectHash string                        `json:"object_hash"`
	Layout     string                        `json:"layout_version"`
}

func (r Row) Observation() monitoring.CalculatedObservation {
	text := func(at *int64) *string {
		if at == nil {
			return nil
		}
		v := time.Unix(0, *at).UTC().Format(time.RFC3339Nano)
		return &v
	}
	return monitoring.CalculatedObservation{SourceKind: "derived", Channel: r.Series, Result: r.Result, Generation: r.Generation, Config: r.Config, Run: r.Run, At: time.Unix(0, r.At).UTC().Format(time.RFC3339Nano), AtNS: r.At, Start: text(r.Start), End: text(r.End), Value: r.Decimal(), Scale: int(r.Scale), Rounding: "half_even", Rounded: r.Rounded, Quality: r.Quality, Status: r.Status, Reasons: append([]string{}, r.Reasons...), Count: strconv.FormatUint(r.Count, 10), Excluded: strconv.FormatUint(r.Excluded, 10), InputFirst: text(r.InputFirst), InputLast: text(r.InputLast), Freshness: "current", Lineage: r.Lineage}
}
