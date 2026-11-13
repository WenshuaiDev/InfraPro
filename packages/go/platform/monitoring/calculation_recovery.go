package monitoring

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
)

// CalculationRecovery is the physical recovery seam. It conveys no user
// authority and cannot start calculations, grant rights or publish results.
// Checkpoint verifies the same repeatable-read control snapshot as pg_dump.
type CalculationRecovery interface {
	Descriptor() CalculationRecoveryDescriptor
	Initialize(context.Context) error
	Check(context.Context) error
	Checkpoint(context.Context, pgx.Tx) error
}
type CalculationRecoveryDescriptor struct {
	Layout        string          `json:"layout_version"`
	Engine        string          `json:"engine_version"`
	Compiler      string          `json:"compiler_version"`
	DSL           string          `json:"dsl_version"`
	Tables        []string        `json:"tables"`
	FixtureSHA256 string          `json:"fixture_sha256,omitempty"`
	Fixtures      json.RawMessage `json:"numeric_fixtures,omitempty"`
}

func compatibleCalculationRecovery(stored, current CalculationRecoveryDescriptor) bool {
	if stored.Layout != current.Layout || stored.Engine != current.Engine || stored.Compiler != current.Compiler || stored.DSL != current.DSL || !slices.Equal(stored.Tables, current.Tables) {
		return false
	}
	if stored.FixtureSHA256 == "" {
		return len(stored.Fixtures) == 0
	}
	var compact bytes.Buffer
	if e := json.Compact(&compact, stored.Fixtures); e != nil {
		return false
	}
	hash := sha256.Sum256(compact.Bytes())
	return hex.EncodeToString(hash[:]) == stored.FixtureSHA256 && stored.FixtureSHA256 == current.FixtureSHA256
}

func requireCalculationRecovery(ctx context.Context, tx pgx.Tx, recovery CalculationRecovery) error {
	var exists bool
	if e := tx.QueryRow(ctx, `SELECT to_regclass('derived_channels') IS NOT NULL`).Scan(&exists); e != nil {
		return e
	}
	if !exists {
		return nil
	}
	var used bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_channels) OR EXISTS(SELECT 1 FROM derived_metric_versions) OR EXISTS(SELECT 1 FROM analysis_config_versions)`).Scan(&used); e != nil {
		return e
	}
	if used && recovery == nil {
		return fmt.Errorf("calculation recovery provider is required for this control database")
	}
	return nil
}
