package monitoring

import (
	"context"
	"encoding/json"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

type CalculationHead struct {
	Root            string `json:"current_root"`
	Revision        string `json:"head_revision"`
	ConfigRevision  string `json:"config_revision"`
	ControlRevision string `json:"control_revision"`
	ControlState    string `json:"control_state"`
	Health          string `json:"health"`
}
type CalculationBoundary struct {
	At                          int64
	Channel, Result, Generation string
}
type CalculationQuery struct {
	// PreparedProofs are server-generated qualification tickets for the exact
	// fixed heads/ranges. Readers must recheck their current DB authority and
	// source state; this is only a way to avoid rediscovering immutable needs.
	PreparedProofs []json.RawMessage
	Summary        func(CalculationSummary) error
	Capability     string
	ValidOnly      bool
	EndpointsOnly  bool
	Heads          map[string]CalculationHead
	Windows        map[string][]Window
	Range          *Window
	Qualities      []string
	Boundary       *CalculationBoundary
	Limit          int
	Reverse        bool
	History        bool
}
type CalculatedObservation struct {
	SourceKind string   `json:"source_kind"`
	Channel    string   `json:"channel_id"`
	Result     string   `json:"result_id"`
	Generation string   `json:"generation_id"`
	Config     string   `json:"config_id"`
	Run        string   `json:"run_id"`
	At         string   `json:"output_at"`
	AtNS       int64    `json:"-"`
	Start      *string  `json:"interval_start"`
	End        *string  `json:"interval_end"`
	Value      *string  `json:"value"`
	Scale      int      `json:"scale"`
	Rounding   string   `json:"rounding"`
	Rounded    bool     `json:"rounded"`
	Quality    string   `json:"data_quality"`
	Status     string   `json:"calculation_status"`
	Reasons    []string `json:"reason_flags"`
	Count      string   `json:"sample_count"`
	Excluded   string   `json:"excluded_count"`
	InputFirst *string  `json:"input_first"`
	InputLast  *string  `json:"input_last"`
	Freshness  string   `json:"freshness"`
	Lineage    string   `json:"lineage_id"`
}
type CalculationCoverage struct {
	Proof      json.RawMessage `json:"-"`
	Channel    string          `json:"channel_id"`
	Range      Range           `json:"range"`
	State      string          `json:"state"`
	Generation string          `json:"generation_id,omitempty"`
}
type CalculationPage struct {
	Items    []CalculatedObservation
	Coverage []CalculationCoverage
}
type CalculationStatus struct {
	LastValidOutput *CalculatedObservation `json:"last_valid_output"`
	ComputedThrough *string                `json:"computed_through"`
	InputThrough    *string                `json:"input_through"`
	PendingRanges   []Range                `json:"pending_ranges"`
	ControlState    string                 `json:"control_state"`
	Health          string                 `json:"health"`
}

type CalculationReader interface {
	CurrentCalculationStatus(context.Context, access.Actor, Scope, CalculationQuery, []CalculationCoverage) (CalculationStatus, error)
	FreezeCalculatedTx(context.Context, pgx.Tx, string, []string) (map[string]CalculationHead, error)
	ReadCalculated(context.Context, access.Actor, Scope, CalculationQuery) (CalculationPage, error)
	ScanCalculated(context.Context, access.Actor, Scope, CalculationQuery, uint64, func(CalculatedObservation) error) ([]CalculationCoverage, error)
	CheckCalculated(context.Context, access.Actor, Scope, CalculationQuery) ([]CalculationCoverage, error)
	CheckCalculationProofsTx(context.Context, pgx.Tx, access.Actor, Scope, []json.RawMessage, string) error
}

func (s *Service) WithCalculationReader(reader CalculationReader) *Service {
	s.calculationReader = reader
	return s
}

func (s *Service) AuthorizeCalculationOutputTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, channel string, required []Window, history bool, capabilities ...string) error {
	return s.AuthorizeCalculationOutputsTx(ctx, tx, a, scope, map[string][]Window{channel: required}, history, capabilities...)
}

// AuthorizeCalculationOutputsTx shares the current actor/catalog and grant
// reads across result generations. Callers pass exact required ranges.
func (s *Service) AuthorizeCalculationOutputsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, required map[string][]Window, history bool, capabilities ...string) error {
	if len(required) == 0 {
		return nil
	}
	auth, e := s.rawAuthorizationTx(ctx, tx, a, scope)
	if e != nil {
		return e
	}
	channels := make([]string, 0, len(required))
	for channel := range required {
		channels = append(channels, channel)
	}
	allowed, e := authorizedWindowsTx(ctx, tx, auth, channels, "read")
	if e != nil {
		return e
	}
	if history {
		historical, e := authorizedWindowsTx(ctx, tx, auth, channels, "history")
		if e != nil {
			return e
		}
		allowed = intersectWindows(allowed, historical)
	}
	for _, capability := range capabilities {
		if capability == "" || capability == "read" {
			continue
		}
		if capability != "export" {
			return access.ErrDenied
		}
		extra, e := authorizedWindowsTx(ctx, tx, auth, channels, capability)
		if e != nil {
			return e
		}
		allowed = intersectWindows(allowed, extra)
	}
	for channel, windows := range required {
		for _, part := range UnionWindows(windows) {
			covered := false
			for _, w := range allowed[channel] {
				if w.Covers(part) {
					covered = true
					break
				}
			}
			if !covered {
				return notFound()
			}
		}
	}
	return nil
}
func (o Observation) MarshalJSON() ([]byte, error) {
	if o.Calculated != nil {
		return json.Marshal(o.Calculated)
	}
	type raw Observation
	value := raw(o)
	value.SourceKind = "direct"
	return json.Marshal(value)
}
func (o *Observation) UnmarshalJSON(raw []byte) error {
	var kind struct {
		SourceKind string `json:"source_kind"`
	}
	if e := json.Unmarshal(raw, &kind); e != nil {
		return e
	}
	if kind.SourceKind == "derived" {
		var point CalculatedObservation
		if e := json.Unmarshal(raw, &point); e != nil {
			return e
		}
		at, e := time.Parse(time.RFC3339Nano, point.At)
		if e != nil {
			return e
		}
		point.AtNS = at.UnixNano()
		*o = *calculatedDTO(&point)
		return nil
	}
	type direct Observation
	return json.Unmarshal(raw, (*direct)(o))
}
