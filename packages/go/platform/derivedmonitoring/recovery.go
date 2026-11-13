package derivedmonitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Recovery struct{ store *resultstore.Store }

func NewRecovery(storage *monitoring.Storage) *Recovery {
	return &Recovery{resultstore.New(storage.CalculationTransport())}
}
func (r *Recovery) Descriptor() monitoring.CalculationRecoveryDescriptor {
	fixtures, hash := compute.RecoveryFixtures()
	return monitoring.CalculationRecoveryDescriptor{Layout: resultstore.LayoutVersion, Engine: compute.EngineVersion, Compiler: compute.CompilerVersion, DSL: compute.SchemaVersion, Tables: []string{"derived_results_local"}, FixtureSHA256: hash, Fixtures: fixtures}
}
func (r *Recovery) Initialize(ctx context.Context) error { return r.store.Initialize(ctx) }
func (r *Recovery) Check(ctx context.Context) error      { return r.store.Check(ctx) }

type ReconciliationFinding struct {
	Code   string `json:"code"`
	Tenant string `json:"tenant_id"`
	Target string `json:"target_id"`
}
type Reconciliation struct {
	State         string                  `json:"state"`
	Started       time.Time               `json:"started_at"`
	Completed     time.Time               `json:"completed_at"`
	Generations   uint64                  `json:"generations,string"`
	Artifacts     uint64                  `json:"artifact_versions,string"`
	Blocks        uint64                  `json:"blocks,string"`
	Rows          uint64                  `json:"rows,string"`
	Bytes         uint64                  `json:"payload_bytes,string"`
	CurrentBytes  uint64                  `json:"current_referenced_payload_bytes,string"`
	HistoryBytes  uint64                  `json:"historical_only_payload_bytes,string"`
	ArtifactBytes uint64                  `json:"artifact_payload_bytes,string"`
	CurrentRanges uint64                  `json:"current_ranges,string"`
	HistoryRanges uint64                  `json:"historical_ranges,string"`
	Snapshots     uint64                  `json:"raw_snapshots,string"`
	Findings      []ReconciliationFinding `json:"findings"`
	FindingCount  uint64                  `json:"finding_count,string"`
}

// Reconcile is read-only physical maintenance, not a user data query. It holds
// an explicit repeatable-read snapshot, like backup, and never changes current
// roots, permissions or publication receipts. Its report contains no values.
func (r *Recovery) Reconcile(ctx context.Context, pool *pgxpool.Pool, tenant string) (Reconciliation, error) {
	if tenant != "" && !access.ValidID(tenant) {
		return Reconciliation{}, fmt.Errorf("invalid tenant")
	}
	tx, e := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return Reconciliation{}, e
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	return r.reconcile(ctx, tx, tenant)
}
func (r *Recovery) Checkpoint(ctx context.Context, tx pgx.Tx) error {
	v, e := r.reconcile(ctx, tx, "")
	if e != nil {
		return e
	}
	if v.State != "passed" {
		return fmt.Errorf("%s (target %s)", v.Findings[0].Code, v.Findings[0].Target)
	}
	return nil
}

// The page contains identifiers only, not manifests or payloads. Each payload
// is released before the next row, so a large installation cannot materialize
// its whole evidence catalog in the verifier's heap.
func walkRecoveryIDs(ctx context.Context, tx pgx.Tx, table, idColumn, tenant string, visit func(string, string) error) error {
	var cursorTenant, cursorID string
	for {
		rows, e := tx.Query(ctx, `SELECT tenant_id::text,`+idColumn+`::text FROM `+table+` WHERE ($1::uuid IS NULL OR tenant_id=$1) AND ($2='' OR (tenant_id::text,`+idColumn+`::text)>($2,$3)) ORDER BY tenant_id::text,`+idColumn+`::text LIMIT 100`, nullable(tenant), cursorTenant, cursorID)
		if e != nil {
			return e
		}
		page := [][2]string{}
		for rows.Next() {
			var v [2]string
			if e = rows.Scan(&v[0], &v[1]); e != nil {
				break
			}
			page = append(page, v)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return e
		}
		for _, v := range page {
			if e = visit(v[0], v[1]); e != nil {
				return e
			}
		}
		if len(page) < 100 {
			return nil
		}
		cursorTenant, cursorID = page[len(page)-1][0], page[len(page)-1][1]
	}
}

func (r *Recovery) reconcile(ctx context.Context, tx pgx.Tx, tenant string) (Reconciliation, error) {
	out := Reconciliation{State: "passed", Started: time.Now().UTC(), Findings: []ReconciliationFinding{}}
	add := func(code, tenant, target string) {
		out.State = "failed"
		out.FindingCount++
		if len(out.Findings) < 100 {
			out.Findings = append(out.Findings, ReconciliationFinding{code, tenant, target})
		}
	}
	check := func(table, id string, fn func(string, string) error) error {
		return walkRecoveryIDs(ctx, tx, table, id, tenant, func(t, id string) error {
			if e := ctx.Err(); e != nil {
				return e
			}
			if e := fn(t, id); e != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				add(table+"_integrity", t, id)
			}
			return nil
		})
	}
	// Permanent rule versions must still compile with the version recorded at
	// publication. Deployment reductions do not reinterpret historical rules.
	if e := check("derived_metric_versions", "id", func(t, id string) error {
		var rule json.RawMessage
		var engine, compiler, schema, hash, output string
		if e := tx.QueryRow(ctx, `SELECT rule,engine_version,compiler_version,schema_version,content_hash,output_contract_hash FROM derived_metric_versions WHERE tenant_id=$1 AND id=$2`, t, id).Scan(&rule, &engine, &compiler, &schema, &hash, &output); e != nil {
			return e
		}
		p, e := compile(rule, "channel", nil)
		if e != nil {
			return e
		}
		if engine != compute.EngineVersion || compiler != compute.CompilerVersion || schema != compute.SchemaVersion || p.PlanHash != hash || p.OutputContractHash != output {
			return fmt.Errorf("unsupported_rule_version_or_hash")
		}
		return nil
	}); e != nil {
		return out, e
	}
	if e := check("derived_channel_configs", "id", func(t, id string) error {
		var rule, canonical json.RawMessage
		var values map[string]string
		var bindings map[string]InputBinding
		if e := tx.QueryRow(ctx, `SELECT rule,parameters,compiled_plan,input_bindings FROM derived_channel_configs WHERE tenant_id=$1 AND id=$2`, t, id).Scan(&rule, &values, &canonical, &bindings); e != nil {
			return e
		}
		p, e := compile(rule, "channel", values)
		if e != nil {
			return e
		}
		var stored, rebuilt any
		if e = json.Unmarshal(canonical, &stored); e != nil {
			return e
		}
		if e = json.Unmarshal(p.Canonical, &rebuilt); e != nil {
			return e
		}
		if fingerprint(stored) != fingerprint(rebuilt) {
			return fmt.Errorf("compiled_plan_mismatch")
		}
		return verifyRecoveryDependencies(ctx, tx, t, id, p, bindings)
	}); e != nil {
		return out, e
	}
	if e := check("derived_channels", "channel_id", func(t, id string) error {
		var complete bool
		e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_channels d JOIN engineering_channels c ON c.tenant_id=d.tenant_id AND c.id=d.channel_id JOIN derived_channel_heads h ON h.tenant_id=d.tenant_id AND h.channel_id=d.channel_id JOIN derived_graph_states g ON g.tenant_id=d.tenant_id WHERE d.tenant_id=$1 AND d.channel_id=$2 AND c.source_kind='derived')`, t, id).Scan(&complete)
		if e != nil {
			return e
		}
		if !complete {
			return fmt.Errorf("channel_control_graph_or_head_missing")
		}
		return nil
	}); e != nil {
		return out, e
	}
	if e := check("derived_graph_states", "tenant_id", func(t, _ string) error {
		graph, e := dependencyGraphTx(ctx, tx, t)
		if e != nil {
			return e
		}
		_, e = planning.Order(graph, 8)
		return e
	}); e != nil {
		return out, e
	}
	if e := check("analysis_config_versions", "id", func(t, id string) error {
		var body AnalysisBody
		var engine, compiler, hash string
		if e := tx.QueryRow(ctx, `SELECT body,engine_version,compiler_version,plan_hash FROM analysis_config_versions WHERE tenant_id=$1 AND id=$2`, t, id).Scan(&body, &engine, &compiler, &hash); e != nil {
			return e
		}
		p, e := compile(body.Rule, "analysis", body.Parameters)
		if e != nil {
			return e
		}
		if engine != compute.EngineVersion || compiler != compute.CompilerVersion || p.PlanHash != hash {
			return fmt.Errorf("analysis_plan_mismatch")
		}
		return nil
	}); e != nil {
		return out, e
	}
	if e := check("monitoring_raw_snapshots", "id", func(t, id string) error {
		out.Snapshots++
		if e := monitoring.VerifyRawCheckpointTx(ctx, tx, t, id); e != nil {
			return e
		}
		proof, exists, e := loadInputSourcesTx(ctx, tx, t, id)
		if e != nil {
			return e
		}
		if exists {
			return verifyRecoverySources(ctx, tx, t, proof.Sources, 0)
		}
		return nil
	}); e != nil {
		return out, e
	}
	if e := check("derived_calculation_config_keys", "id", func(t, id string) error {
		rows, e := tx.Query(ctx, `SELECT reference_key FROM derived_fixed_baselines WHERE tenant_id=$1 AND config_id=$2 ORDER BY reference_key`, t, id)
		if e != nil {
			return e
		}
		keys := []string{}
		for rows.Next() {
			var key string
			if e = rows.Scan(&key); e != nil {
				break
			}
			keys = append(keys, key)
			if len(keys) > 32 {
				e = fmt.Errorf("baseline_count_exceeded")
				break
			}
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return e
		}
		for _, key := range keys {
			if _, exists, e := loadBaselineTx(ctx, tx, t, id, key); e != nil {
				return e
			} else if !exists {
				return fmt.Errorf("baseline_missing")
			}
		}
		return nil
	}); e != nil {
		return out, e
	}
	if e := check("derived_write_attempts", "id", func(t, id string) error {
		var state string
		var block resultstore.Block
		var published bool
		if e := tx.QueryRow(ctx, `SELECT state,COALESCE(manifest,'{}'::jsonb),EXISTS(SELECT 1 FROM derived_generations g WHERE g.tenant_id=a.tenant_id AND g.run_id=a.run_id) OR EXISTS(SELECT 1 FROM analysis_artifact_versions v WHERE v.tenant_id=a.tenant_id AND v.run_id=a.run_id) FROM derived_write_attempts a WHERE tenant_id=$1 AND id=$2`, t, id).Scan(&state, &block, &published); e != nil {
			return e
		}
		var retired bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM monitoring_retention_receipts WHERE tenant_id=$1 AND kind='derived_attempt' AND target=$2)`, t, id).Scan(&retired); e != nil {
			return e
		}
		if state == "sealed" && !published && !retired {
			return r.store.Verify(ctx, block)
		}
		return nil
	}); e != nil {
		return out, e
	}
	for _, table := range []string{"derived_generations", "analysis_artifact_versions"} {
		if e := check(table, "id", func(t, id string) error {
			return r.verifyPublished(ctx, tx, t, id, table, &out)
		}); e != nil {
			return out, e
		}
	}
	if e := check("derived_channel_heads", "channel_id", func(t, id string) error {
		var current, history, summary, coverage string
		if e := tx.QueryRow(ctx, `SELECT COALESCE(current_root,''),COALESCE(history_root,''),COALESCE(summary_root,''),COALESCE(coverage_root,'') FROM derived_channel_heads WHERE tenant_id=$1 AND channel_id=$2`, t, id).Scan(&current, &history, &summary, &coverage); e != nil {
			return e
		}
		if coverage != current {
			return fmt.Errorf("coverage_root_mismatch")
		}
		if summary != "" {
			var m resultManifest
			if e := r.store.ReadDocument(ctx, t, "manifests", summary, &m); e != nil {
				return e
			}
			if m.Channel != id {
				return fmt.Errorf("summary_owner_mismatch")
			}
		}
		currentBlocks := map[string]bool{}
		collectCurrent := true
		verifier := r.store.RangeVerifier(t, func(v resultstore.RangeEntry) error {
			var valid bool
			if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_generations WHERE tenant_id=$1 AND id=$2 AND channel_id=$3 AND manifest_root=$4 AND output_range @> $5::int8range AND source_position=$6)`, t, v.Generation, id, v.Manifest, rangeSQL(v.Interval()), v.SourcePosition).Scan(&valid); e != nil {
				return e
			}
			if !valid {
				return fmt.Errorf("current_generation_mismatch")
			}
			if collectCurrent {
				var manifest resultManifest
				if e := r.store.ReadDocument(ctx, t, "manifests", v.Manifest, &manifest); e != nil {
					return e
				}
				interval := v.Interval()
				for _, block := range manifest.Blocks {
					if block.Last != nil && *block.Last < interval.From || block.First != nil && interval.To != nil && *block.First >= *interval.To {
						continue
					}
					if !currentBlocks[block.Attempt] {
						currentBlocks[block.Attempt] = true
						out.CurrentBytes += block.Bytes
					}
				}
			}
			return nil
		})
		count, e := verifier.Verify(ctx, current)
		if e != nil {
			return e
		}
		out.CurrentRanges += count
		collectCurrent = false
		for history != "" {
			var h struct {
				Schema          string          `json:"schema"`
				Previous        string          `json:"previous"`
				PreviousCurrent string          `json:"previous_current"`
				Current         string          `json:"current"`
				Generation      string          `json:"generation_id"`
				Manifest        string          `json:"manifest_root"`
				Range           NanosecondRange `json:"range"`
			}
			if e := r.store.ReadDocument(ctx, t, "history", history, &h); e != nil {
				return e
			}
			if h.Schema != "derived-history/1" || h.Current != current {
				return fmt.Errorf("history_chain_mismatch")
			}
			var valid bool
			if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_publication_receipts r JOIN derived_generations g ON g.tenant_id=r.tenant_id AND g.id=r.generation_id WHERE r.tenant_id=$1 AND g.channel_id=$2 AND r.generation_id=$3 AND r.current_root=$4 AND r.history_root=$5 AND g.manifest_root=$6)`, t, id, h.Generation, current, history, h.Manifest).Scan(&valid); e != nil {
				return e
			}
			if !valid {
				return fmt.Errorf("history_receipt_mismatch")
			}
			history, current = h.Previous, h.PreviousCurrent
			count, e := verifier.Verify(ctx, current)
			if e != nil {
				return e
			}
			out.HistoryRanges += count
		}
		return nil
	}); e != nil {
		return out, e
	}
	var bad int64
	if e := tx.QueryRow(ctx, `SELECT count(*) FROM monitoring_change_consumers c LEFT JOIN monitoring_tenant_states t USING(tenant_id) WHERE ($1::uuid IS NULL OR c.tenant_id=$1) AND (t.tenant_id IS NULL OR c.durable_cursor>t.commit_sequence)`, nullable(tenant)).Scan(&bad); e != nil {
		return out, e
	}
	if bad > 0 {
		add("consumer_cursor_outside_log", tenant, "")
	}
	if e := tx.QueryRow(ctx, `SELECT count(*) FROM derived_dirty_ranges d JOIN monitoring_tenant_states t USING(tenant_id) WHERE ($1::uuid IS NULL OR d.tenant_id=$1) AND d.required_position>t.commit_sequence`, nullable(tenant)).Scan(&bad); e != nil {
		return out, e
	}
	if bad > 0 {
		add("dirty_position_outside_log", tenant, "")
	}
	if out.CurrentBytes+out.ArtifactBytes > out.Bytes {
		add("payload_inventory_mismatch", tenant, "")
	} else {
		out.HistoryBytes = out.Bytes - out.CurrentBytes - out.ArtifactBytes
	}
	out.Completed = time.Now().UTC()
	return out, nil
}

func verifyRecoveryDependencies(ctx context.Context, tx pgx.Tx, tenant, config string, plan *compute.Plan, bindings map[string]InputBinding) error {
	rows, e := tx.Query(ctx, `SELECT d.slot_key,d.input_channel_id::text,d.input_kind,c.source_kind,c.deleted_at IS NOT NULL FROM derived_dependencies d LEFT JOIN engineering_channels c ON c.tenant_id=d.tenant_id AND c.id=d.input_channel_id WHERE d.tenant_id=$1 AND d.config_id=$2 ORDER BY d.slot_key`, tenant, config)
	if e != nil {
		return e
	}
	defer rows.Close()
	found := map[string]string{}
	for rows.Next() {
		var slot, channel, kind string
		var actual *string
		var deleted bool
		if e = rows.Scan(&slot, &channel, &kind, &actual, &deleted); e != nil {
			return e
		}
		if actual == nil || *actual != kind || deleted || bindings[slot].ChannelID != channel {
			return fmt.Errorf("dependency_binding_mismatch")
		}
		found[slot] = channel
	}
	if e = rows.Err(); e != nil {
		return e
	}
	if len(found) != len(bindings) || len(found) != len(plan.Rule.Inputs) {
		return fmt.Errorf("dependency_binding_missing")
	}
	for _, input := range plan.Rule.Inputs {
		if found[input.Key] == "" {
			return fmt.Errorf("dependency_slot_missing")
		}
	}
	return nil
}

func verifyRecoverySources(ctx context.Context, tx pgx.Tx, tenant string, sources []inputSourceProof, depth int) error {
	if depth > 8 {
		return fmt.Errorf("source_depth_exceeded")
	}
	for _, p := range sources {
		rangeValue, e := p.Range.rangeValue()
		if e != nil {
			return e
		}
		var valid bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_generations WHERE tenant_id=$1 AND id=$2 AND channel_id=$3 AND config_id=$4 AND raw_snapshot_id=$5 AND manifest_root=$6 AND output_range @> $7::int8range)`, tenant, p.Generation, p.Channel, p.Config, p.Snapshot, p.Manifest, rangeSQL(planning.Range{From: rangeValue.From, To: rangeValue.To})).Scan(&valid); e != nil {
			return e
		}
		if !valid {
			return fmt.Errorf("source_generation_missing")
		}
		if e = verifyRecoverySources(ctx, tx, tenant, p.Inputs, depth+1); e != nil {
			return e
		}
	}
	return nil
}

func (r *Recovery) verifyPublished(ctx context.Context, tx pgx.Tx, t, id, table string, out *Reconciliation) error {
	var root, lineage, run, config, snapshot, position string
	var count uint64
	var from int64
	var to *int64
	configColumn, countColumn := "config_id", "row_count"
	if table == "analysis_artifact_versions" {
		configColumn, countColumn = "config_version_id", "output_rows"
		out.Artifacts++
	} else {
		out.Generations++
	}
	if e := tx.QueryRow(ctx, `SELECT manifest_root,lineage_root,run_id::text,`+configColumn+`::text,raw_snapshot_id::text,source_position::text,`+countColumn+`,lower(output_range),upper(output_range) FROM `+table+` WHERE tenant_id=$1 AND id=$2`, t, id).Scan(&root, &lineage, &run, &config, &snapshot, &position, &count, &from, &to); e != nil {
		return e
	}
	var m resultManifest
	if e := r.store.ReadDocument(ctx, t, "manifests", root, &m); e != nil {
		return e
	}
	if m.Generation != id || m.Lineage != lineage || m.Run != run || m.Config != config || m.Input.ID != snapshot || m.Input.SourcePosition != position || m.Rows != count || fingerprint(m.Range) != fingerprint(rangeDTO(from, to)) {
		return fmt.Errorf("generation_manifest_mismatch")
	}
	if table == "derived_generations" && m.Schema != "derived-result-manifest/1" || table == "analysis_artifact_versions" && m.Schema != "analysis-result-manifest/1" {
		return fmt.Errorf("manifest_version_mismatch")
	}
	var plan string
	if e := tx.QueryRow(ctx, `SELECT plan_hash FROM derived_runs WHERE tenant_id=$1 AND id=$2`, t, run).Scan(&plan); e != nil {
		return e
	}
	if plan != m.Plan {
		return fmt.Errorf("run_plan_mismatch")
	}
	var wholeRange bool
	if table == "analysis_artifact_versions" {
		if e := tx.QueryRow(ctx, `SELECT run_rule->'clock'->>'kind'='whole_range' FROM analysis_runs WHERE tenant_id=$1 AND run_id=$2`, t, run).Scan(&wholeRange); e != nil {
			return e
		}
	}
	var sourceHash string
	if e := tx.QueryRow(ctx, `SELECT content_hash FROM monitoring_raw_snapshots WHERE tenant_id=$1 AND id=$2`, t, snapshot).Scan(&sourceHash); e != nil {
		return e
	}
	if sourceHash != m.Input.Hash {
		return fmt.Errorf("manifest_snapshot_hash_mismatch")
	}
	var line map[string]json.RawMessage
	if e := r.store.ReadDocument(ctx, t, "lineage", lineage, &line); e != nil {
		return e
	}
	for field, expected := range map[string]string{"engine_version": compute.EngineVersion, "compiler_version": compute.CompilerVersion, "run_id": run, "plan_hash": plan} {
		var actual string
		if e := json.Unmarshal(line[field], &actual); e != nil || actual != expected {
			return fmt.Errorf("lineage_identity_mismatch")
		}
	}
	if e := verifyRecoverySources(ctx, tx, t, m.Sources, 0); e != nil {
		return e
	}
	var rows uint64
	states := map[string]uint64{}
	for _, b := range m.Blocks {
		var sealed resultstore.Block
		var state, attemptRun string
		if e := tx.QueryRow(ctx, `SELECT state,run_id::text,manifest FROM derived_write_attempts WHERE tenant_id=$1 AND id=$2 AND chunk_id=$3`, t, b.Attempt, b.Chunk).Scan(&state, &attemptRun, &sealed); e != nil {
			return e
		}
		if state != "sealed" || attemptRun != run || fingerprint(b) != fingerprint(sealed) {
			return fmt.Errorf("result_attempt_mismatch")
		}
		if e := r.store.VerifyRows(ctx, b, func(row resultstore.Row) error {
			if row.Tenant != t || row.Generation != id || row.Run != run || row.Config != config || row.Lineage != lineage || !publishedRowTime(m, row, wholeRange) {
				return fmt.Errorf("result_payload_owner_mismatch")
			}
			states[row.Status]++
			return nil
		}); e != nil {
			return e
		}
		rows += b.Rows
		out.Blocks++
		out.Bytes += b.Bytes
		if table == "analysis_artifact_versions" {
			out.ArtifactBytes += b.Bytes
		}
		out.Rows += b.Rows
	}
	if rows != count || len(states) != len(m.States) {
		return fmt.Errorf("result_totals_mismatch")
	}
	for k, v := range states {
		if m.States[k] != strconv.FormatUint(v, 10) {
			return fmt.Errorf("status_summary_mismatch")
		}
	}
	return nil
}

// Whole-range analysis labels its interval at the exclusive source end. That
// label is legal only for this explicit clock and the exact sealed interval.
func publishedRowTime(m resultManifest, row resultstore.Row, whole bool) bool {
	r, e := m.Range.rangeValue()
	if e != nil {
		return false
	}
	if !whole {
		return r.Contains(row.At)
	}
	if m.SourceRange == nil {
		return false
	}
	source, e := m.SourceRange.rangeValue()
	if e != nil || source.To == nil || r.To == nil {
		return false
	}
	return *source.To == *r.To && row.At == *source.To && row.Start != nil && *row.Start == source.From && row.End != nil && *row.End == *source.To
}
