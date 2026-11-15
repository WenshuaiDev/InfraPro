package monitoring

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"io"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ExportInput struct {
	WriteMeta
	Query   QueryInput `json:"query"`
	Format  string     `json:"format"`
	Kind    string     `json:"kind"`
	CSVMode string     `json:"csv_mode"`
}
type ExportRecord struct {
	TaskRef   *taskrecord.Ref `json:"task_ref,omitempty"`
	ID        string          `json:"id"`
	JobID     string          `json:"job_id"`
	State     string          `json:"state"`
	ExpiresAt time.Time       `json:"expires_at"`
	Snapshot  Snapshot        `json:"snapshot"`
}
type ExportFile struct {
	Part   string `json:"part"`
	Name   string `json:"name"`
	Rows   string `json:"row_count"`
	Bytes  string `json:"size_bytes"`
	SHA256 string `json:"sha256"`
}
type ExportManifest struct {
	ID                  string                     `json:"id"`
	Snapshot            Snapshot                   `json:"snapshot"`
	Query               QueryInput                 `json:"query"`
	Kind                string                     `json:"kind"`
	Format              string                     `json:"format"`
	CSVMode             string                     `json:"csv_mode"`
	Escaping            string                     `json:"escaping"`
	ParquetVersion      string                     `json:"parquet_version"`
	Fields              []ExportField              `json:"fields"`
	Files               []ExportFile               `json:"files"`
	Rows                string                     `json:"row_count"`
	ExpiresAt           time.Time                  `json:"expires_at"`
	Windows             map[string][]Range         `json:"authorized_ranges"`
	Ordering            string                     `json:"ordering"`
	CalculationCoverage []CalculationCoverage      `json:"calculation_coverage,omitempty"`
	CalculationHeads    map[string]CalculationHead `json:"calculation_heads,omitempty"`
}
type sealedExport struct {
	Manifest          ExportManifest
	Objects           map[string]string
	CalculationProofs []json.RawMessage
}
type exportChannel struct{ Point, PointName, Name, Metric, Unit, OwnerKind, OwnerID, OwnerName string }

func normalizeExport(in ExportInput) (ExportInput, error) {
	if e := in.WriteMeta.validate(); e != nil {
		return in, e
	}
	if in.ExpectedRevision != "0" {
		return in, invalid("expected_revision", "新导出使用修订 0。")
	}
	if !slices.Contains([]string{"csv", "parquet"}, in.Format) {
		return in, invalid("format", "请选择 CSV 或 Parquet。")
	}
	if in.Kind == "" {
		in.Kind = "details"
	}
	if !slices.Contains([]string{"details", "statistics"}, in.Kind) {
		return in, invalid("kind", "导出范围应为明细或基础统计。")
	}
	if in.CSVMode == "" {
		in.CSVMode = "exact"
	}
	if !slices.Contains([]string{"exact", "spreadsheet_safe"}, in.CSVMode) || in.Format == "parquet" && in.CSVMode != "exact" {
		return in, invalid("csv_mode", "CSV 模式无效。")
	}
	if in.Query.Cursor != "" {
		return in, invalid("query.cursor", "导出固定筛选的完整集合，不从浏览游标截断。")
	}
	var e error
	if in.Kind == "statistics" {
		in.Query, e = analyticInput(in.Query, "statistics")
	} else {
		in.Query, e = normalizedQuery(in.Query)
		if in.Query.Period != "" {
			return in, invalid("period", "明细导出不使用日历分组。")
		}
	}
	return in, e
}
func (s *Service) CreateExport(ctx context.Context, a access.Actor, scope Scope, input ExportInput) (ExportRecord, error) {
	out := ExportRecord{}
	in, e := normalizeExport(input)
	if e != nil {
		return out, e
	}
	plan, e := s.queryPlan(ctx, a, scope, in.Query, "exports.create", "export")
	if e != nil {
		return out, e
	}
	if e = s.preflightCalculatedExport(ctx, a, scope, plan, in.Query); e != nil {
		return out, e
	}
	e = s.transact(ctx, a, scope, "exports.create", false, false, func(tx pgx.Tx, auth authorization) error {
		hit, e := receiptTx(ctx, tx, auth, "exports.create", in.RequestID, in, &out)
		if e != nil || hit {
			return e
		}
		if authorizationVersion(auth) != plan.authorization {
			return revisionChanged()
		}
		if e = assertSnapshotTx(ctx, tx, scope.Tenant, plan.snapshot.ID, plan.authorization); e != nil {
			return e
		}
		out.ID = uuid.NewString()
		out.JobID = uuid.NewString()
		out.TaskRef = &taskrecord.Ref{SourceRef: taskrecord.SourceRef{Module: "monitoring", Kind: "job", RootID: out.JobID}, Generation: "1", TaskID: (taskrecord.SourceRef{Module: "monitoring", Kind: "job", RootID: out.JobID}).ID()}
		out.State = "queued"
		out.Snapshot = plan.snapshot
		jobInput := in
		jobInput.Query.Snapshot = plan.snapshot.ID
		_, e = tx.Exec(ctx, `INSERT INTO monitoring_jobs(id,tenant_id,project_id,actor_id,kind,target_id,target_revision,input_hash,input) VALUES($1,$2,$3,$4,'export',$5,$6,$7,$8)`, out.JobID, scope.Tenant, nullable(scope.Project), a.ID, out.ID, plan.snapshot.Watermark, digest(jobInput), encodeJSON(jobInput))
		if e != nil {
			return e
		}
		e = tx.QueryRow(ctx, `INSERT INTO monitoring_exports(id,tenant_id,job_id,snapshot_id,format,expires_at) VALUES($1,$2,$3,$4,$5,clock_timestamp()+interval '7 days') RETURNING expires_at`, out.ID, scope.Tenant, out.JobID, plan.snapshot.ID, in.Format).Scan(&out.ExpiresAt)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE monitoring_snapshots SET lease_until=GREATEST(lease_until,$3::timestamptz) WHERE tenant_id=$1 AND id=$2`, scope.Tenant, plan.snapshot.ID, out.ExpiresAt)
		if e != nil {
			return e
		}
		if e = auditTx(ctx, tx, auth, "exports.create", out.ID, in.Reason, in.RequestID, map[string]string{"format": in.Format, "kind": in.Kind, "snapshot_id": plan.snapshot.ID}); e != nil {
			return e
		}
		return saveReceiptTx(ctx, tx, auth, "exports.create", in.RequestID, in, out)
	})
	return out, e
}
func assertSnapshotTx(ctx context.Context, tx pgx.Tx, tenant, id, authorization string) error {
	var valid bool
	e := tx.QueryRow(ctx, `SELECT sn.authorization_revision=$3 AND (sn.expires_at>clock_timestamp() OR sn.lease_until>clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM monitoring_selections sel CROSS JOIN LATERAL unnest(sel.channel_ids) AS chosen(channel_id) LEFT JOIN monitoring_channel_versions v ON v.tenant_id=sel.tenant_id AND v.channel_id=chosen.channel_id WHERE sel.id=sn.selection_id AND COALESCE(v.revoke_revision,0)::text IS DISTINCT FROM sn.revision_vector->'revokes'->>chosen.channel_id::text) FROM monitoring_snapshots sn WHERE sn.tenant_id=$1 AND sn.id=$2`, tenant, id, authorization).Scan(&valid)
	if e != nil {
		return e
	}
	if !valid {
		return problem("snapshot_stale", "授权或相关数据撤销状态已变化，请重新申请。", 409)
	}
	return nil
}
func exportChannelsTx(ctx context.Context, tx pgx.Tx, tenant string, ids []string) (map[string]exportChannel, error) {
	out := map[string]exportChannel{}
	rows, e := tx.Query(ctx, `SELECT c.id::text,COALESCE(c.point_id::text,''),COALESCE(p.name,''),c.name,COALESCE(c.metric_version_id::text,''),COALESCE(u.content->'unit'->>'symbol',''),c.owner_kind,CASE c.owner_kind WHEN 'facility' THEN c.facility_id::text WHEN 'component' THEN c.component_id::text ELSE c.point_id::text END,CASE c.owner_kind WHEN 'facility' THEN f.name WHEN 'component' THEN co.name ELSE p.name END,c.source_kind,COALESCE(c.quantity_spec,'null'::jsonb) FROM engineering_channels c LEFT JOIN engineering_points p ON p.id=c.point_id JOIN engineering_facilities f ON f.id=c.facility_id LEFT JOIN engineering_components co ON co.id=c.component_id LEFT JOIN engineering_versions u ON u.id=COALESCE(c.unit_version_id,NULLIF(c.quantity_spec->>'base_unit_version_id','')::uuid) WHERE c.tenant_id=$1 AND c.id=ANY($2::uuid[])`, tenant, ids)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var v exportChannel
		var source string
		var quantity json.RawMessage
		if e = rows.Scan(&id, &v.Point, &v.PointName, &v.Name, &v.Metric, &v.Unit, &v.OwnerKind, &v.OwnerID, &v.OwnerName, &source, &quantity); e != nil {
			return out, e
		}
		if source == "derived" {
			v.Unit, e = calculationUnit(v.Unit, quantity)
			if e != nil {
				return out, e
			}
		}
		out[id] = v
	}
	return out, rows.Err()
}
func exportDetail(r rawObservation, c exportChannel, revoked bool) []exportCell {
	values := []string{r.Channel, c.Point, c.PointName, c.Name, c.Metric, c.Unit, r.Observation, r.Adoption, r.Batch, strconv.FormatInt(r.At, 10), "", r.Kind, r.Value, "", "", "", r.Quality, r.QualityReason, strconv.FormatBool(revoked)}
	out := make([]exportCell, len(values))
	for i, v := range values {
		out[i].Text = v
	}
	out[10].Null = r.Start == nil
	if r.Start != nil {
		out[10].Text = strconv.FormatInt(*r.Start, 10)
	}
	for i, kind := range []string{"integer", "decimal", "enum"} {
		out[13+i].Null = r.Kind != kind
		if r.Kind == kind {
			out[13+i].Text = r.Value
		}
	}
	return out
}
func exportStatistic(v ChannelStatistic, g *CalendarStatistic, c exportChannel, query QueryInput) []exportCell {
	values := []string{v.Channel, c.PointName, c.Name, c.Unit, query.Period, query.Timezone, "", "", "", "false", v.Ever, v.Active, v.Samples, v.N, "", "", "", "", "", "", v.MinCount, v.MaxCount, "", "", strconv.Itoa(v.Scale), v.Rounding, string(encodeJSON(v.States))}
	out := make([]exportCell, len(values))
	for i, value := range values {
		out[i] = exportCell{Text: value, Null: statisticsExportFields[i].Nullable && value == ""}
	}
	if g != nil {
		out[6] = exportCell{Text: g.Start}
		out[7] = exportCell{Text: g.End}
		out[8] = exportCell{Text: string(encodeJSON(g.Included))}
		out[9] = exportCell{Text: strconv.FormatBool(g.Partial)}
	}
	for _, pair := range []struct {
		index int
		point *Observation
	}{{14, v.First}, {15, v.Last}} {
		if pair.point != nil {
			tm, _ := time.Parse(time.RFC3339Nano, pair.point.At)
			out[pair.index] = exportCell{Text: strconv.FormatInt(tm.UnixNano(), 10)}
		}
	}
	for _, pair := range []struct {
		index int
		point *Observation
	}{{16, v.First}, {17, v.Last}, {18, v.Min}, {19, v.Max}} {
		if pair.point != nil {
			out[pair.index] = exportCell{Text: pair.point.Value}
		}
	}
	if v.Mean != nil {
		out[22] = exportCell{Text: *v.Mean}
	}
	if v.Stddev != nil {
		out[23] = exportCell{Text: *v.Stddev}
	}
	return out
}
func (s *Service) exportManifestPage(ctx context.Context, tenant string, plan queryPlan, cursor string) ([]selectedChunk, error) {
	out := []selectedChunk{}
	shards := []int{}
	for id, ws := range plan.windows {
		if len(ws) > 0 {
			shards = append(shards, int(logicalShard(tenant, id)))
		}
	}
	slices.Sort(shards)
	shards = slices.Compact(shards)
	watermark, _ := revisionNumber(plan.snapshot.Watermark)
	if watermark == 0 || len(shards) == 0 {
		return out, nil
	}
	e := s.db(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT c.id::text,m.attempt_id::text,c.payload_hash,c.row_count,m.first_adopted_count,c.batch_id::text,p.sequence,(p.revoke_sequence IS NOT NULL AND p.revoke_sequence<=$3) FROM monitoring_publication_chunks m JOIN monitoring_chunks c ON c.tenant_id=m.tenant_id AND c.id=m.chunk_id JOIN monitoring_publications p ON p.tenant_id=m.tenant_id AND p.id=m.publication_id WHERE c.tenant_id=$1 AND c.logical_shard=ANY($2::int[]) AND p.sequence<=$3 AND ($4='history' OR p.revoke_sequence IS NULL OR p.revoke_sequence>$3) AND ($5='' OR c.id::text>$5) ORDER BY c.id LIMIT 1000`, tenant, shards, watermark, plan.filter.Status, cursor)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var c selectedChunk
			if e = rows.Scan(&c.Chunk, &c.Attempt, &c.Hash, &c.Count, &c.First, &c.Batch, &c.Sequence, &c.Revoked); e != nil {
				return e
			}
			out = append(out, c)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		rows.Close()
		return resolveLocationsTx(ctx, tx, tenant, out, mustRevision(plan.snapshot.RouteRevision))
	})
	return out, e
}

var errExportPage = errors.New("bounded_export_page")

func (s *Service) exportJob(ctx context.Context, j Job) error {
	var input ExportInput
	if e := json.Unmarshal(j.input, &input); e != nil {
		return e
	}
	in, e := normalizeExport(input)
	if e != nil {
		return e
	}
	actor := workerActor(j.actor)
	plan, e := s.queryPlan(ctx, actor, j.scope(), in.Query, "exports.create", "export", true)
	if e != nil {
		return e
	}
	var channels map[string]exportChannel
	var expires time.Time
	e = s.jobTransaction(ctx, j, func(tx pgx.Tx) error {
		if e := assertJobTx(ctx, tx, j); e != nil {
			return e
		}
		var e error
		channels, e = exportChannelsTx(ctx, tx, j.tenant, plan.channels)
		if e != nil {
			return e
		}
		return tx.QueryRow(ctx, `SELECT expires_at FROM monitoring_exports WHERE tenant_id=$1 AND id=$2 AND invalidated_at IS NULL AND expires_at>clock_timestamp()`, j.tenant, j.Target).Scan(&expires)
	})
	if e != nil {
		return e
	}
	watermark, _ := revisionNumber(plan.snapshot.Watermark)
	for _, channel := range plan.channels {
		if _, derived := plan.vector.Calculated[channel]; derived {
			continue
		}
		if _, e = s.buildSummary(ctx, j.tenant, channel, plan.vector.Contents[channel], plan.vector.Revokes[channel], watermark); e != nil {
			return e
		}
	}
	r := summaryReader{service: s, plan: plan, tenant: j.tenant, nodes: map[string]*summaryNode{}}
	expected := map[string]exactState{}
	for _, channel := range plan.channels {
		if _, derived := plan.vector.Calculated[channel]; derived {
			continue
		}
		root, e := s.readSummaryRoot(ctx, j.tenant, channel, plan.vector.Contents[channel])
		if e != nil {
			return e
		}
		v, e := r.collect(ctx, channel, root.Root, queryWindow(in.Query))
		if e != nil {
			return e
		}
		expected[channel] = v.samples(in.Query.Qualities, in.Query.Status == "history")
	}
	fields := detailExportFields
	if in.Kind == "statistics" {
		fields = statisticsExportFields
	}
	if len(plan.vector.Calculated) > 0 {
		fields = calculationExportFields(in.Kind)
	}
	manifest := ExportManifest{ID: j.Target, Snapshot: plan.snapshot, Query: in.Query, Kind: in.Kind, Format: in.Format, CSVMode: in.CSVMode, Fields: fields, Files: []ExportFile{}, Rows: "0", ExpiresAt: expires, Windows: map[string][]Range{}, Ordering: "stable manifest pages; observed_at, channel_id, observation_id, adoption_id ascending"}
	if in.Format == "parquet" {
		manifest.ParquetVersion = "2.6; UTC NANOS; Decimal(38,12); Int64; UTF-8 enum"
		if len(plan.vector.Calculated) > 0 {
			manifest.ParquetVersion += "; derived computed_decimal_text UTF-8 + scale/rounding (not DECIMAL)"
		}
	}
	manifest.CalculationHeads = plan.vector.Calculated
	manifest.CalculationCoverage, e = s.checkCalculatedExport(ctx, actor, j.scope(), plan, in.Query)
	if e != nil {
		return e
	}
	if len(plan.vector.Calculated) > 0 {
		manifest.Ordering = "direct immutable manifest order, followed by derived immutable result manifest order; stable result_id and generation_id identify each row"
	}
	if in.CSVMode == "spreadsheet_safe" {
		manifest.Escaping = "Non-numeric formula-like cells and cells starting with apostrophe gain one leading apostrophe. Remove exactly that prefix to reverse; numeric negative values are unchanged."
	}
	for channel, windows := range plan.windows {
		manifest.Windows[channel] = []Range{}
		for _, w := range windows {
			manifest.Windows[channel] = append(manifest.Windows[channel], w.rangeDTO())
		}
	}
	sealed := sealedExport{Manifest: manifest, Objects: map[string]string{}}
	for _, part := range manifest.CalculationCoverage {
		if len(part.Proof) == 0 {
			return problem("result_integrity_unavailable", "导出读取证明缺失。", 503)
		}
		sealed.CalculationProofs = append(sealed.CalculationProofs, part.Proof)
	}
	guard := func() error {
		if _, e := s.queryPlan(ctx, actor, j.scope(), in.Query, "exports.create", "export", true); e != nil {
			return e
		}
		return s.jobTransaction(ctx, j, func(tx pgx.Tx) error {
			if e := assertJobTx(ctx, tx, j); e != nil {
				return e
			}
			// The initial qualification fixed complete recursive needs and
			// immutable source identities. Recheck those server-generated
			// proofs against current authority and changes without rediscovering
			// every predecessor and rereading S3 for each output buffer.
			if len(sealed.CalculationProofs) > 0 {
				return s.calculationReader.CheckCalculationProofsTx(ctx, tx, actor, j.scope(), sealed.CalculationProofs, "export")
			}
			return nil
		})
	}
	volumes, e := s.exportVolumes(ctx, j, in.Format, in.CSVMode, in.Kind, fields, guard)
	if e != nil {
		return e
	}
	defer volumes.abort()
	write := volumes.write
	if in.Kind == "details" {
		observed := map[string]exactState{}
		manifestCursor := ""
		for {
			entries, e := s.exportManifestPage(ctx, j.tenant, plan, manifestCursor)
			if e != nil {
				return e
			}
			if len(entries) == 0 {
				break
			}
			revoked := map[string]bool{}
			for _, entry := range entries {
				revoked[entry.Batch] = entry.Revoked
			}
			var boundary *rawObservation
			for {
				if e = guard(); e != nil {
					return e
				}
				data := []rawObservation{}
				bytes := 0
				e = s.storage.read(ctx, rawRead{Manifest: entries, Tenant: j.tenant, Windows: plan.windows, Range: queryWindow(in.Query), Qualities: in.Query.Qualities, Boundary: boundary, Limit: 500}, func(row rawObservation) error {
					size := len(encodeJSON(row))
					if len(data) > 0 && bytes+size > 8<<20 {
						return errExportPage
					}
					bytes += size
					data = append(data, row)
					return nil
				})
				if e != nil && !errors.Is(e, errExportPage) {
					return e
				}
				if len(data) == 0 {
					break
				}
				if e = guard(); e != nil {
					return e
				}
				for _, row := range data {
					st := observed[row.Channel]
					p := pointOf(row)
					p.Revoked = revoked[row.Batch]
					st.add(p)
					observed[row.Channel] = st
					cells := exportDetail(row, channels[row.Channel], p.Revoked)
					if len(plan.vector.Calculated) > 0 {
						cells = extendRawExport(cells, channels[row.Channel])
					}
					if e = write(cells); e != nil {
						return e
					}
				}
				last := data[len(data)-1]
				boundary = &last
			}
			manifestCursor = entries[len(entries)-1].Chunk
		}
		for channel, want := range expected {
			got := observed[channel]
			if digest(got) != digest(want) {
				return fmt.Errorf("export reconciliation disagrees with snapshot summary for channel %s", channel)
			}
		}
		if len(plan.vector.Calculated) > 0 {
			_, e = s.calculationReader.ScanCalculated(ctx, actor, j.scope(), CalculationQuery{PreparedProofs: sealed.CalculationProofs, Heads: plan.vector.Calculated, Windows: plan.windows, Range: queryWindow(in.Query), Qualities: in.Query.Qualities, History: in.Query.Status == "history", Capability: "export"}, ^uint64(0), func(row CalculatedObservation) error {
				// The volume writer owns the one-second authorization/cancellation
				// guard for both raw and calculated rows. Two independent clocks
				// can alternately expire while checking a multi-channel proof and
				// force two full checks per row without making useful progress.
				return write(exportCalculated(row, channels[row.Channel]))
			})
			if e != nil {
				return e
			}
		}
	} else {
		query := in.Query
		query.Limit = 1000
		for {
			if e = guard(); e != nil {
				return e
			}
			result, e := s.computeAnalytic(ctx, workerActor(j.actor), j.scope(), "statistics", query, plan, true)
			if e != nil {
				return e
			}
			for _, v := range result.Channels {
				cells := exportStatistic(v, nil, channels[v.Channel], query)
				if len(plan.vector.Calculated) > 0 {
					cells = extendStatisticExport(cells, v, channels[v.Channel])
				}
				if e = write(cells); e != nil {
					return e
				}
			}
			for _, g := range result.Groups {
				cells := exportStatistic(g.ChannelStatistic, &g, channels[g.Channel], query)
				if len(plan.vector.Calculated) > 0 {
					cells = extendStatisticExport(cells, g.ChannelStatistic, channels[g.Channel])
				}
				if e = write(cells); e != nil {
					return e
				}
			}
			if !result.HasMore {
				break
			}
			query.Cursor = result.NextCursor
		}
	}
	if e = volumes.finish(); e != nil {
		return e
	}
	sealed.Manifest.Rows = countText(volumes.total)
	sealed.Manifest.Files = volumes.files
	sealed.Objects = volumes.objects
	key, e := s.storage.putSummary(ctx, j.tenant, sealed)
	if e != nil {
		return e
	}
	if e = guard(); e != nil {
		return e
	}
	return s.jobTransaction(ctx, j, func(tx pgx.Tx) error {
		if e := assertJobTx(ctx, tx, j); e != nil {
			return e
		}
		if e := assertSnapshotTx(ctx, tx, j.tenant, plan.snapshot.ID, plan.authorization); e != nil {
			return e
		}
		if len(sealed.CalculationProofs) > 0 {
			if e := s.calculationReader.CheckCalculationProofsTx(ctx, tx, actor, j.scope(), sealed.CalculationProofs, "export"); e != nil {
				return e
			}
		}
		result, e := tx.Exec(ctx, `UPDATE monitoring_exports SET manifest_object=$3 WHERE tenant_id=$1 AND id=$2 AND invalidated_at IS NULL AND expires_at>clock_timestamp()`, j.tenant, j.Target, key)
		if e != nil {
			return e
		}
		if result.RowsAffected() != 1 {
			return problem("export_invalidated", "导出已失效。", 409)
		}
		e = s.FinalizeTaskTx(ctx, tx, j.ID, j.fence, "succeeded", "", map[string]string{"phase": "complete", "rows": countText(volumes.total), "files": strconv.Itoa(volumes.part)})
		return e
	})
}
func verifyExportVolume(path, format string, want int64) (string, int64, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", 0, e
	}
	defer f.Close()
	if format == "csv" {
		r := csv.NewReader(f)
		if _, e = r.Read(); e != nil {
			return "", 0, e
		}
		var count int64
		for {
			_, e = r.Read()
			if e == io.EOF {
				break
			}
			if e != nil {
				return "", 0, e
			}
			count++
		}
		if count != want {
			return "", 0, fmt.Errorf("CSV volume row count")
		}
	} else {
		r, e := file.OpenParquetFile(path, false)
		if e != nil {
			return "", 0, e
		}
		count := r.NumRows()
		if e = r.Close(); e != nil {
			return "", 0, e
		}
		if count != want {
			return "", 0, fmt.Errorf("Parquet volume row count")
		}
		// The independent reader is closed before hashing the volume.
	}
	if _, e = f.Seek(0, io.SeekStart); e != nil {
		return "", 0, e
	}
	h := sha256.New()
	size, e := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), size, e
}

func (s *Service) loadExport(ctx context.Context, a access.Actor, scope Scope, id, action string) (sealedExport, queryPlan, error) {
	out := sealedExport{}
	plan := queryPlan{}
	if !access.ValidID(id) {
		return out, plan, notFound()
	}
	var input ExportInput
	var key string
	e := s.transact(ctx, a, scope, action, false, false, func(tx pgx.Tx, _ authorization) error {
		var raw []byte
		var expired, invalidated bool
		var state string
		e := tx.QueryRow(ctx, `SELECT j.input,COALESCE(e.manifest_object,''),e.expires_at<=clock_timestamp(),e.invalidated_at IS NOT NULL,j.state FROM monitoring_exports e JOIN monitoring_jobs j ON j.tenant_id=e.tenant_id AND j.id=e.job_id WHERE e.tenant_id=$1 AND e.id=$2 AND j.actor_id=$3 AND j.project_id IS NOT DISTINCT FROM $4::uuid`, scope.Tenant, id, a.ID, nullable(scope.Project)).Scan(&raw, &key, &expired, &invalidated, &state)
		if e == pgx.ErrNoRows {
			return notFound()
		}
		if e != nil {
			return e
		}
		if invalidated {
			return problem("export_invalidated", "相关数据已撤销，请重新申请导出。", 409)
		}
		if expired {
			return problem("export_expired", "导出结果已过期，请重新申请。", 410)
		}
		if state != "succeeded" || key == "" {
			return problem("export_not_ready", "导出尚未完成，请查看任务状态。", 409)
		}
		return json.Unmarshal(raw, &input)
	})
	if e != nil {
		return out, plan, e
	}
	plan, e = s.queryPlan(ctx, a, scope, input.Query, action, "export", true)
	if e != nil {
		return out, plan, e
	}
	if e = s.storage.summaryObject(ctx, key, &out, true); e != nil {
		return out, plan, storageError(e)
	}
	if len(out.Manifest.CalculationCoverage) != len(out.CalculationProofs) || digest(out.Manifest.CalculationHeads) != digest(plan.vector.Calculated) {
		return sealedExport{}, queryPlan{}, problem("result_integrity_unavailable", "导出读取证明不完整。", 503)
	}
	e = s.transact(ctx, a, scope, action, false, false, func(tx pgx.Tx, auth authorization) error {
		if authorizationVersion(auth) != plan.authorization {
			return problem("authorization_changed", "导出读取期间授权已变化。", 409)
		}
		if e := assertSnapshotTx(ctx, tx, scope.Tenant, plan.snapshot.ID, plan.authorization); e != nil {
			return e
		}
		if len(out.CalculationProofs) > 0 {
			return s.calculationReader.CheckCalculationProofsTx(ctx, tx, a, scope, out.CalculationProofs, "export")
		}
		return nil
	})
	if e != nil {
		return sealedExport{}, queryPlan{}, e
	}
	return out, plan, nil
}
func (s *Service) ExportFiles(ctx context.Context, a access.Actor, scope Scope, id string) (ExportManifest, error) {
	v, _, e := s.loadExport(ctx, a, scope, id, "exports.files")
	return v.Manifest, e
}
func (s *Service) ExportDownload(ctx context.Context, a access.Actor, scope Scope, id, part string) (Download, error) {
	result, plan, e := s.loadExport(ctx, a, scope, id, "exports.download")
	if e != nil {
		return Download{}, e
	}
	out := Download{ContentType: "application/octet-stream"}
	var manifestBytes []byte
	if part == "manifest" {
		manifestBytes = encodeJSON(result.Manifest)
		out.Name = "monitoring-export-manifest.json"
		out.ContentType = "application/json"
		out.Size = int64(len(manifestBytes))
		sum := sha256.Sum256(manifestBytes)
		out.SHA256 = hex.EncodeToString(sum[:])
	} else {
		found := false
		for _, f := range result.Manifest.Files {
			if f.Part == part {
				found = true
				out.Name = f.Name
				out.Size, _ = strconv.ParseInt(f.Bytes, 10, 64)
				out.SHA256 = f.SHA256
				break
			}
		}
		if !found {
			return out, notFound()
		}
	}
	e = s.transact(ctx, a, scope, "exports.download", false, false, func(tx pgx.Tx, auth authorization) error {
		if authorizationVersion(auth) != plan.authorization {
			return problem("authorization_changed", "下载授权已变化。", 409)
		}
		var e error
		var generation string
		if e = tx.QueryRow(ctx, `SELECT j.notification_generation::text FROM monitoring_exports x JOIN monitoring_jobs j ON j.id=x.job_id WHERE x.tenant_id=$1 AND x.id=$2`, scope.Tenant, id).Scan(&generation); e != nil {
			return e
		}
		out.Observer, e = s.access.BeginTransferTx(ctx, tx, a.ID, "", ar.Scope{Kind: scope.Entry, Tenant: scope.Tenant, Project: scope.Project}, "monitoring", ar.Link{Kind: "domain_file", Module: "monitoring", TargetKind: "export", TargetID: id, Generation: generation, Relation: "output"})
		return e
	})
	if e != nil {
		return out, e
	}
	out.Write = func(ctx context.Context, w io.Writer) error {
		var reader io.ReadCloser
		if part == "manifest" {
			reader = io.NopCloser(bytes.NewReader(manifestBytes))
		} else {
			var e error
			reader, e = s.storage.get(ctx, result.Objects[part])
			if e != nil {
				return storageError(e)
			}
		}
		defer reader.Close()
		buffer := make([]byte, 128*1024)
		h := sha256.New()
		var written int64
		for {
			// Network reads may return tiny fragments. Requalify each bounded
			// output block, after external I/O and immediately before emission.
			n, readError := io.ReadFull(reader, buffer)
			if readError == io.ErrUnexpectedEOF {
				readError = io.EOF
			}
			if e := s.transact(ctx, a, scope, "exports.download", false, false, func(tx pgx.Tx, auth authorization) error {
				if authorizationVersion(auth) != plan.authorization {
					return problem("authorization_changed", "下载期间授权已变化。", 409)
				}
				if e := assertSnapshotTx(ctx, tx, scope.Tenant, plan.snapshot.ID, plan.authorization); e != nil {
					return e
				}
				if len(result.CalculationProofs) > 0 {
					if e := s.calculationReader.CheckCalculationProofsTx(ctx, tx, a, scope, result.CalculationProofs, "export"); e != nil {
						return e
					}
				}
				var valid bool
				if e := tx.QueryRow(ctx, `SELECT invalidated_at IS NULL AND expires_at>clock_timestamp() FROM monitoring_exports WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id).Scan(&valid); e != nil {
					return e
				}
				if !valid {
					return problem("export_invalidated", "导出已失效或过期。", 409)
				}
				return nil
			}); e != nil {
				return e
			}
			if n > 0 {
				h.Write(buffer[:n])
				count, e := w.Write(buffer[:n])
				written += int64(count)
				if e != nil {
					return e
				}
				if count != n {
					return io.ErrShortWrite
				}
			}
			if readError == io.EOF {
				if written != out.Size || hex.EncodeToString(h.Sum(nil)) != out.SHA256 {
					return io.ErrUnexpectedEOF
				}
				return nil
			}
			if readError != nil {
				return storageError(readError)
			}
		}
	}
	out.Write = out.Observer.Wrap(out.Write)
	return out, nil
}
