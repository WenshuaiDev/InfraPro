package monitoring

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring/internal/observation"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type validationSnapshot struct {
	Semantics      map[string]channelSemantics `json:"semantics"`
	Authorization  string                      `json:"authorization"`
	Windows        map[string][]Window         `json:"windows"`
	Coverage       map[string]coverage         `json:"coverage"`
	ActualCoverage map[string]coverage         `json:"actual_coverage"`
	Readopt        *ReadoptSelection           `json:"readopt,omitempty"`
	WholeSource    bool                        `json:"whole_source"`
}
type coverage struct {
	First int64  `json:"first,string"`
	Last  int64  `json:"last,string"`
	Count uint64 `json:"count,string"`
}
type validationInput struct {
	batch      ImportBatch
	mapping    Mapping
	source     string
	sourceHash string
	snapshot   validationSnapshot
	revisions  map[string]string
	manifest   []selectedChunk
	history    []selectedChunk
}

func authorizationVersion(auth authorization) string {
	v := auth.Revision + ":" + auth.ResourceRevision + ":" + auth.DataRevision
	if auth.Subject != nil {
		v += ":" + auth.Subject.Revision
	}
	return v
}

func semanticsTx(ctx context.Context, tx pgx.Tx, tenant string, ids []string, write bool) (map[string]channelSemantics, error) {
	out := map[string]channelSemantics{}
	if write {
		var invalid bool
		e := tx.QueryRow(ctx, `WITH RECURSIVE ancestors AS (
 SELECT tenant_id,kind,id,parent_kind,parent_id,status,deleted_at FROM engineering_resource_nodes WHERE tenant_id=$1 AND kind='channel' AND id=ANY($2::uuid[])
 UNION SELECT n.tenant_id,n.kind,n.id,n.parent_kind,n.parent_id,n.status,n.deleted_at FROM engineering_resource_nodes n JOIN ancestors a ON n.tenant_id=a.tenant_id AND n.kind=a.parent_kind AND n.id=a.parent_id
 ) SELECT EXISTS(SELECT 1 FROM ancestors WHERE status<>'normal' OR deleted_at IS NOT NULL)`, tenant, ids).Scan(&invalid)
		if e != nil {
			return out, e
		}
		if invalid {
			return out, problem("resource_retired", "通道或祖先已退役，请按工程资源规则恢复后再补录。", 409)
		}
	}
	rows, e := tx.Query(ctx, `SELECT c.id::text,c.metric_version_id::text,COALESCE(c.unit_version_id::text,''),COALESCE(u.content->'unit'->>'symbol',''),m.content->'metric'->>'shape',c.timestamp_meaning,c.observation_direction,c.reference_basis,m.content->'metric' FROM engineering_channels c JOIN engineering_versions m ON m.id=c.metric_version_id LEFT JOIN engineering_versions u ON u.id=c.unit_version_id WHERE c.tenant_id=$1 AND c.id=ANY($2::uuid[]) AND c.source_kind='direct' AND c.deleted_at IS NULL ORDER BY c.id`, tenant, ids)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var sem channelSemantics
		if e = rows.Scan(&sem.ID, &sem.Metric, &sem.Unit, &sem.Symbol, &sem.Kind, &sem.TimeMeaning, &sem.Direction, &sem.Basis, &sem.Definition); e != nil {
			return out, e
		}
		sem.Hash = digest(sem)
		out[sem.ID] = sem
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out) != len(ids) {
		return out, notFound()
	}
	return out, nil
}
func (s *Service) loadValidation(ctx context.Context, j Job) (validationInput, error) {
	out := validationInput{revisions: map[string]string{}, snapshot: validationSnapshot{Coverage: map[string]coverage{}}}
	e := s.transact(ctx, workerActor(j.actor), j.scope(), j.action(), false, true, func(tx pgx.Tx, auth authorization) error {
		if e := assertJobTx(ctx, tx, j); e != nil {
			return e
		}
		var e error
		out.batch, e = batchTx(ctx, tx, auth, j.Target, true)
		if e != nil {
			return e
		}
		if out.batch.Revision != j.TargetRevision || out.batch.State != "validating" {
			return revisionChanged()
		}
		if out.batch.ReadoptBatch != "" {
			prior, e := batchSnapshotTx(ctx, tx, j.tenant, j.Target)
			if e != nil {
				return e
			}
			out.snapshot.Readopt = prior.Readopt
		}
		var raw []byte
		if e = tx.QueryRow(ctx, `SELECT mapping FROM monitoring_import_mappings WHERE tenant_id=$1 AND batch_id=$2 AND revision=$3`, j.tenant, j.Target, out.batch.MappingRevision).Scan(&raw); e != nil {
			return e
		}
		if e = json.Unmarshal(raw, &out.mapping); e != nil {
			return e
		}
		ids := channelsOf(out.mapping)
		out.snapshot.Windows, e = authorizedWindowsTx(ctx, tx, auth, ids, "import")
		if e != nil {
			return e
		}
		if out.batch.ReadoptBatch != "" {
			if e = readoptAccessTx(ctx, tx, j.tenant, out.batch.ReadoptBatch, out.snapshot.Readopt, out.snapshot.Windows); e != nil {
				return e
			}
		}
		out.snapshot.Semantics, e = semanticsTx(ctx, tx, j.tenant, ids, true)
		if e != nil {
			return e
		}
		out.snapshot.Authorization = authorizationVersion(auth)
		for _, id := range ids {
			if _, e = tx.Exec(ctx, `INSERT INTO monitoring_channel_versions(tenant_id,channel_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, j.tenant, id); e != nil {
				return e
			}
			var rev string
			if e = tx.QueryRow(ctx, `SELECT content_revision::text FROM monitoring_channel_versions WHERE tenant_id=$1 AND channel_id=$2`, j.tenant, id).Scan(&rev); e != nil {
				return e
			}
			out.revisions[id] = rev
		}
		if _, e = tx.Exec(ctx, `INSERT INTO monitoring_tenant_states(tenant_id) VALUES($1) ON CONFLICT DO NOTHING`, j.tenant); e != nil {
			return e
		}
		if e = tx.QueryRow(ctx, `SELECT object_key,sha256 FROM monitoring_source_files WHERE tenant_id=$1 AND id=$2`, j.tenant, out.batch.SourceFile).Scan(&out.source, &out.sourceHash); e != nil {
			return e
		}
		out.manifest, e = manifestTx(ctx, tx, j.tenant, out.snapshot.Windows, 0, "active")
		if e != nil {
			return e
		}
		if out.batch.ReadoptBatch == "" {
			out.history, e = manifestTx(ctx, tx, j.tenant, out.snapshot.Windows, 0, "history")
		}
		return e
	})
	return out, e
}

func manifestTx(ctx context.Context, tx pgx.Tx, tenant string, windows map[string][]Window, watermark int64, status string, routeVersion ...int64) ([]selectedChunk, error) {
	out := []selectedChunk{}
	shards := []int{}
	for id := range windows {
		shards = append(shards, int(logicalShard(tenant, id)))
	}
	slices.Sort(shards)
	shards = slices.Compact(shards)
	if len(shards) == 0 {
		return out, nil
	}
	// Month and logical-shard metadata prune candidates. Authorization remains a
	// mandatory per-observation external-table predicate at the physical reader.
	var lo, hi *int64
	boundedLo, boundedHi := true, true
	for _, ws := range windows {
		for _, w := range ws {
			if w.From == nil {
				boundedLo = false
			} else if lo == nil || *w.From < *lo {
				lo = w.From
			}
			if w.To == nil {
				boundedHi = false
			} else if hi == nil || *w.To > *hi {
				hi = w.To
			}
		}
	}
	if !boundedLo {
		lo = nil
	}
	if !boundedHi {
		hi = nil
	}
	rows, e := tx.Query(ctx, `SELECT c.id::text,m.attempt_id::text,c.payload_hash,c.row_count,m.first_adopted_count,c.batch_id::text,p.sequence,(p.revoke_sequence IS NOT NULL),m.quality_counts FROM monitoring_publication_chunks m JOIN monitoring_chunks c ON c.tenant_id=m.tenant_id AND c.id=m.chunk_id JOIN monitoring_publications p ON p.tenant_id=m.tenant_id AND p.id=m.publication_id WHERE c.tenant_id=$1 AND c.logical_shard=ANY($2::int[]) AND ($3::bigint=0 OR p.sequence<=$3) AND ($4='history' OR (p.revoke_sequence IS NULL OR ($3>0 AND p.revoke_sequence>$3))) AND ($5::bigint IS NULL OR c.last_ns>=$5) AND ($6::bigint IS NULL OR c.first_ns<$6) ORDER BY c.id LIMIT 20001`, tenant, shards, watermark, status, lo, hi)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var m selectedChunk
		var quality []byte
		if e = rows.Scan(&m.Chunk, &m.Attempt, &m.Hash, &m.Count, &m.First, &m.Batch, &m.Sequence, &m.Revoked, &quality); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(quality, &m.Qualities); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(out) > 20000 {
		return nil, problem("query_budget_exceeded", "所选清单超过交互预算，请缩小范围。", 422)
	}
	rows.Close()
	route := int64(0)
	if len(routeVersion) > 0 {
		route = routeVersion[0]
	}
	return out, resolveLocationsTx(ctx, tx, tenant, out, route)
}

type validationCounters struct{ rows, candidates, blank, missing, errors, suspect, duplicates, proposed int64 }

func (c validationCounters) dto() ImportCounts {
	return ImportCounts{countText(c.rows), countText(c.candidates), countText(c.blank), countText(c.missing), countText(c.errors), countText(c.suspect), countText(c.duplicates), "0", countText(c.proposed)}
}

func (s *Service) validateJob(ctx context.Context, j Job) error {
	in, e := s.loadValidation(ctx, j)
	if e != nil {
		return e
	}
	temp, e := newSpool(s.storage.config.TempDirectory)
	if e != nil {
		return e
	}
	defer temp.close()
	report, e := temp.file()
	if e != nil {
		return e
	}
	defer report.Close()
	reportWriter := bufio.NewWriterSize(report, 64*1024)
	issues := json.NewEncoder(reportWriter)
	counts := validationCounters{}
	sourceProgress := &progressWriter{}
	lastProgress := time.Time{}
	var progressError error
	reportProgress := func(force bool, phase string) error {
		if progressError != nil {
			return progressError
		}
		if !force && time.Since(lastProgress) < time.Second {
			return nil
		}
		lastProgress = time.Now()
		progressError = s.recordProgress(ctx, j, map[string]string{"phase": phase, "bytes_read": countText(sourceProgress.bytes), "total_bytes": in.batch.Size, "parsed_rows": countText(counts.rows), "candidates": countText(counts.candidates)})
		return progressError
	}
	sourceProgress.changed = func() error { return reportProgress(false, "reading") }
	if e = reportProgress(true, "reading"); e != nil {
		return e
	}
	issue := func(v ImportIssue) error {
		if v.Code == "same_observation" {
			counts.duplicates++
		} else {
			counts.errors++
		}
		return issues.Encode(v)
	}
	sorter := rowSorter{s: temp, compare: compareRaw}
	header := map[string]int{}
	headerColumns := map[int]bool{}
	headerFound := false
	location, _ := time.LoadLocation(in.mapping.Timezone)
	aliases := map[string]ChannelMapping{}
	for _, binding := range in.mapping.Channels {
		aliases[binding.Alias] = binding
	}
	cell := func(row inputRow, name string) inputCell {
		if name == "" {
			return inputCell{}
		}
		i, ok := header[name]
		if !ok || i >= len(row.Cells) {
			return inputCell{}
		}
		return row.Cells[i]
	}
	date1904 := false
	in.snapshot.WholeSource = in.mapping.HeaderRow == 1
	consume := func(row inputRow) error {
		if e := reportProgress(false, "reading"); e != nil {
			return e
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		if row.EmptyRows > 0 {
			header := uint64(in.mapping.HeaderRow)
			if header >= row.Line && header < row.Line+row.EmptyRows {
				return invalid("header", "所选表头行为空。")
			}
			if row.Line > header {
				counts.rows += int64(row.EmptyRows)
				counts.blank += int64(row.EmptyRows)
			}
			return nil
		}
		if row.Line < uint64(in.mapping.HeaderRow) {
			return nil
		}
		if row.Line == uint64(in.mapping.HeaderRow) {
			for index, c := range row.Cells {
				if c.Text == "" {
					continue
				}
				if _, ok := header[c.Text]; ok {
					return invalid("header", "表头列名重复。")
				}
				header[c.Text] = index
				headerColumns[index] = true
			}
			required := []string{in.mapping.TimeColumn, in.mapping.ChannelColumn, in.mapping.ValueColumn, in.mapping.UnitColumn, in.mapping.QualityColumn, in.mapping.RecordColumn, in.mapping.IntervalColumn}
			if in.mapping.Mode == "wide" {
				for _, b := range in.mapping.Channels {
					required = append(required, b.ValueColumn)
				}
			}
			for _, name := range required {
				if name != "" {
					if _, ok := header[name]; !ok {
						return invalid("header", "缺少映射列："+name)
					}
				}
			}
			for name := range header {
				if !slices.Contains(required, name) {
					in.snapshot.WholeSource = false
				}
			}
			headerFound = true
			return nil
		}
		counts.rows++
		for index, c := range row.Cells {
			if !headerColumns[index] && c.Text != "" {
				in.snapshot.WholeSource = false
			}
		}
		blank := true
		for _, c := range row.Cells {
			if c.Text != "" {
				blank = false
				break
			}
		}
		if blank {
			counts.blank++
			return nil
		}
		bindings := in.mapping.Channels
		if in.mapping.Mode == "long" {
			binding, ok := aliases[cell(row, in.mapping.ChannelColumn).Text]
			if !ok {
				return issue(ImportIssue{Code: "unknown_channel", Line: strconv.FormatUint(row.Line, 10), Column: in.mapping.ChannelColumn, Message: "通道别名未映射。"})
			}
			bindings = []ChannelMapping{binding}
		}
		for _, binding := range bindings {
			col := in.mapping.ValueColumn
			if in.mapping.Mode == "wide" {
				col = binding.ValueColumn
			}
			value := cell(row, col)
			if value.Text == "" {
				counts.missing++
				if in.mapping.Mode == "wide" && in.mapping.EmptyWide == "no_observation" {
					continue
				}
				if e := issue(ImportIssue{Code: "missing_value", Line: strconv.FormatUint(row.Line, 10), Column: col, Channel: binding.Channel, Message: "业务行缺少值。"}); e != nil {
					return e
				}
				continue
			}
			counts.candidates++
			r, e := normalizeInput(in, j, binding, row.Line, col, value, cell(row, in.mapping.TimeColumn), cell(row, in.mapping.IntervalColumn), cell(row, in.mapping.QualityColumn), cell(row, in.mapping.UnitColumn), cell(row, in.mapping.RecordColumn), location, date1904)
			if e != nil {
				if e = issue(ImportIssue{Code: "invalid_observation", Line: strconv.FormatUint(row.Line, 10), Column: col, Channel: binding.Channel, Message: e.Error()}); e != nil {
					return e
				}
				continue
			}
			if r.Quality == "suspect" {
				counts.suspect++
			}
			cover, ok := in.snapshot.Coverage[r.Channel]
			first := r.At
			if r.Start != nil {
				first = *r.Start
			}
			if !ok {
				cover = coverage{First: first, Last: r.At}
			}
			cover.First = min(cover.First, first)
			cover.Last = max(cover.Last, r.At)
			cover.Count++
			in.snapshot.Coverage[r.Channel] = cover
			if e = sorter.add(r); e != nil {
				return e
			}
		}
		return nil
	}
	if in.batch.ReadoptBatch != "" {
		in.snapshot.WholeSource = false // The retained source may contain ignored rows.
		headerFound = true
		e = s.readoptCandidates(ctx, j, in, func(r rawObservation) error {
			counts.rows++
			counts.candidates++
			allowed := false
			for _, w := range in.snapshot.Windows[r.Channel] {
				if w.Contains(r.At, r.Start) {
					allowed = true
					break
				}
			}
			if !allowed {
				return issue(ImportIssue{Code: "range_denied", Line: strconv.FormatUint(r.Line, 10), Column: r.Column, Message: "原观测不在当前重新采用授权范围。"})
			}
			if r.Quality == "suspect" {
				counts.suspect++
			}
			first := r.At
			if r.Start != nil {
				first = *r.Start
			}
			cover, ok := in.snapshot.Coverage[r.Channel]
			if !ok {
				cover = coverage{First: first, Last: r.At}
			}
			cover.First = min(cover.First, first)
			cover.Last = max(cover.Last, r.At)
			cover.Count++
			in.snapshot.Coverage[r.Channel] = cover
			return sorter.add(r)
		})
	} else {
		source, err := s.storage.get(ctx, in.source)
		if err != nil {
			return err
		}
		hash := sha256.New()
		reader := io.TeeReader(source, io.MultiWriter(hash, sourceProgress))
		if in.mapping.Format == "csv" {
			e = parseCSV(ctx, reader, in.mapping, consume)
		} else {
			e = parseXLSX(ctx, reader, in.mapping, temp, &date1904, consume, &in.snapshot.WholeSource)
		}
		source.Close()
		if e == nil && hex.EncodeToString(hash.Sum(nil)) != in.sourceHash {
			e = problem("source_integrity_failed", "原文件完整 SHA-256 与封存依据不一致。", 422)
		}
	}
	if progressError != nil {
		return progressError
	}
	if e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e = issue(ImportIssue{Code: "file_structure", Message: e.Error()}); e != nil {
			return e
		}
	}
	if !headerFound {
		if e = issue(ImportIssue{Code: "missing_header", Message: "未找到所选表头行。"}); e != nil {
			return e
		}
	}
	if e = reportProgress(true, "validating"); e != nil {
		return e
	}
	sorted, e := sorter.finish(ctx)
	if e != nil {
		return e
	}
	candidate, e := openRows(sorted)
	if e != nil {
		return e
	}
	defer candidate.close()
	has := candidate.next()
	var previous *rawObservation
	proposed, e := temp.file()
	if e != nil {
		return e
	}
	defer proposed.Close()
	proposedWriter := bufio.NewWriterSize(proposed, 64*1024)
	output := json.NewEncoder(proposedWriter)
	resolve := func(existing *rawObservation) error {
		r := candidate.row
		target := existing
		if target == nil && previous != nil && previous.Channel == r.Channel && previous.At == r.At {
			target = previous
		}
		if target != nil {
			code, message := "same_observation", "同通道同时间同值，需确认是同一次观测；原观测质量保持不变。"
			if target.Kind != r.Kind || target.Value != r.Value || !sameStart(target.Start, r.Start) {
				code = "value_conflict"
				message = "同通道同时间存在不同值或区间，须修正文件或先撤销错误旧批次。"
			}
			if e := issue(ImportIssue{Code: code, Line: strconv.FormatUint(r.Line, 10), Column: r.Column, Channel: r.Channel, At: time.Unix(0, r.At).UTC().Format(time.RFC3339Nano), Message: message, Target: target.Observation, PayloadHash: target.hash()}); e != nil {
				return e
			}
		} else {
			counts.proposed++
			if e := output.Encode(r); e != nil {
				return e
			}
		}
		previous = &r
		has = candidate.next()
		return candidate.err
	}
	// Merge two sorted streams. Existing data is never copied wholesale into RAM.
	e = s.storage.read(ctx, rawRead{Manifest: in.manifest, Tenant: j.tenant, Windows: in.snapshot.Windows, Order: "channel"}, func(existing rawObservation) error {
		for has && (candidate.row.Channel < existing.Channel || (candidate.row.Channel == existing.Channel && candidate.row.At < existing.At)) {
			if e := resolve(nil); e != nil {
				return e
			}
		}
		for has && candidate.row.Channel == existing.Channel && candidate.row.At == existing.At {
			if e := resolve(&existing); e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return e
	}
	for has {
		if e = resolve(nil); e != nil {
			return e
		}
	}
	if candidate.err != nil {
		return candidate.err
	}
	if e = proposedWriter.Flush(); e != nil {
		return e
	}
	proposed.Close()
	proposedPath := proposed.Name()
	if in.batch.ReadoptBatch == "" && len(in.history) > 0 {
		proposedPath, e = s.checkRetainedCandidates(ctx, j, in, temp, proposedPath, &counts, issue)
		if e != nil {
			return e
		}
	}
	if e = reportWriter.Flush(); e != nil {
		return e
	}
	proposed.Close()
	report.Close()
	objectBase := j.tenant + "/validations/" + j.Target + "/" + uuid.NewString()
	readyObject, reportObject := objectBase+"/candidates.ndjson", objectBase+"/issues.ndjson"
	if e = temp.object(ctx, s.storage, proposedPath, readyObject); e != nil {
		return e
	}
	if e = temp.object(ctx, s.storage, report.Name(), reportObject); e != nil {
		return e
	}
	if e = reportProgress(true, "complete"); e != nil {
		return e
	}
	previewHash := digest([]any{j.Target, in.batch.MappingRevision, in.snapshot, in.revisions, readyObject, reportObject, counts.dto()})
	return s.transact(ctx, workerActor(j.actor), j.scope(), "imports.validate", false, true, func(tx pgx.Tx, auth authorization) error {
		if e := assertJobTx(ctx, tx, j); e != nil {
			return e
		}
		b, e := batchTx(ctx, tx, auth, j.Target, true)
		if e != nil {
			return e
		}
		if b.State != "validating" || b.Revision != j.TargetRevision {
			return revisionChanged()
		}
		if authorizationVersion(auth) != in.snapshot.Authorization {
			return problem("authorization_changed", "授权已变化，需要重新校验。", 409)
		}
		if e = checkContentTx(ctx, tx, j.tenant, encodeJSON(in.revisions), true); e != nil {
			return e
		}
		if _, e = semanticsTx(ctx, tx, j.tenant, channelsOf(in.mapping), true); e != nil {
			return e
		}
		state := "ready"
		if counts.errors > 0 || counts.duplicates > 0 {
			state = "needs_resolution"
		}
		_, e = tx.Exec(ctx, `UPDATE monitoring_import_batches SET state=$3,revision=revision+1,validation_hash=$4,validation_object=$5,report_object=$6,counts=$7,semantic_snapshot=$8,content_revisions=$9,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2`, j.tenant, j.Target, state, previewHash, readyObject, reportObject, encodeJSON(counts.dto()), encodeJSON(in.snapshot), encodeJSON(in.revisions))
		return e
	})
}

// The first conflict pass leaves at most one proposed row per channel/time.
// A second streaming merge can therefore check all retained adoptions without
// buffering an unbounded history group or treating a re-upload as a new event.
func (s *Service) checkRetainedCandidates(ctx context.Context, j Job, in validationInput, temp *spool, source string, counts *validationCounters, issue func(ImportIssue) error) (string, error) {
	candidate, e := openRows(source)
	if e != nil {
		return "", e
	}
	defer candidate.close()
	output, e := temp.file()
	if e != nil {
		return "", e
	}
	defer output.Close()
	writer := bufio.NewWriterSize(output, 64*1024)
	encoder := json.NewEncoder(writer)
	has := candidate.next()
	var matched *rawObservation
	finish := func() error {
		r := candidate.row
		if matched != nil {
			counts.proposed--
			if e := issue(ImportIssue{Code: "same_observation", Line: strconv.FormatUint(r.Line, 10), Column: r.Column, Channel: r.Channel, At: time.Unix(0, r.At).UTC().Format(time.RFC3339Nano), Message: "保留历史中存在同值观测；确认重复将忽略新来源，需要恢复时请在原批次重新采用。", Target: matched.Observation, PayloadHash: matched.hash()}); e != nil {
				return e
			}
		} else {
			if e := encoder.Encode(r); e != nil {
				return e
			}
		}
		matched = nil
		has = candidate.next()
		return candidate.err
	}
	e = s.storage.read(ctx, rawRead{Manifest: in.history, Tenant: j.tenant, Windows: in.snapshot.Windows, Order: "channel"}, func(existing rawObservation) error {
		for has && (candidate.row.Channel < existing.Channel || candidate.row.Channel == existing.Channel && candidate.row.At < existing.At) {
			if e := finish(); e != nil {
				return e
			}
		}
		if has && candidate.row.Channel == existing.Channel && candidate.row.At == existing.At && candidate.row.Kind == existing.Kind && candidate.row.Value == existing.Value && sameStart(candidate.row.Start, existing.Start) && matched == nil {
			copy := existing
			matched = &copy
		}
		return nil
	})
	if e != nil {
		return "", e
	}
	for has {
		if e = finish(); e != nil {
			return "", e
		}
	}
	if candidate.err != nil {
		return "", candidate.err
	}
	if e = writer.Flush(); e != nil {
		return "", e
	}
	return output.Name(), output.Close()
}

func sameStart(a, b *int64) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
func inputTime(cell inputCell, m Mapping, location *time.Location, date1904 bool) (time.Time, int32, uint8, error) {
	if cell.Formula || cell.Error {
		return time.Time{}, 0, 0, fmt.Errorf("时间单元格为公式或错误，不能用缓存结果。")
	}
	var t time.Time
	var e error
	var offset int
	precision := uint8(0)
	if cell.Numeric {
		civil, err := observation.ExcelTime(cell.Text, date1904)
		if err != nil {
			return t, 0, 0, err
		}
		t, e = observation.ParseLocal(civil.Format("2006-01-02T15:04:05.999999999"), "2006-01-02T15:04:05.999999999", m.Timezone)
		if e == nil {
			_, offset = t.In(location).Zone()
		}
		precision = 9
	} else {
		t, e = observation.ParseTime(cell.Text)
		if e == nil {
			z, _ := time.Parse(time.RFC3339Nano, cell.Text)
			_, offset = z.Zone()
		} else if m.LocalTimeLayout != "" {
			t, e = observation.ParseLocal(cell.Text, m.LocalTimeLayout, m.Timezone)
			if e == nil {
				_, offset = t.In(location).Zone()
			}
		}
		if dot := strings.IndexByte(cell.Text, '.'); dot >= 0 {
			for _, c := range cell.Text[dot+1:] {
				if c < '0' || c > '9' {
					break
				}
				precision++
			}
		}
	}
	return t, int32(offset), precision, e
}
func normalizeInput(in validationInput, j Job, binding ChannelMapping, line uint64, col string, value, at, start, quality, unit, record inputCell, location *time.Location, date1904 bool) (rawObservation, error) {
	r := rawObservation{}
	sem := in.snapshot.Semantics[binding.Channel]
	for _, c := range []inputCell{value, at, start, quality, unit, record} {
		if c.Formula || c.Error {
			return r, fmt.Errorf("映射区域包含公式或错误；公式缓存不是原始值。")
		}
	}
	if binding.Unit != sem.Symbol {
		return r, fmt.Errorf("映射单位与通道固定单位不一致；不进行隐式换算。")
	}
	if in.mapping.UnitColumn != "" && unit.Text == "" {
		return r, fmt.Errorf("已映射单位列的业务行缺少单位。")
	}
	if unit.Text != "" && unit.Text != sem.Symbol {
		return r, fmt.Errorf("文件单位与通道固定单位不一致。")
	}
	tm, offset, precision, e := inputTime(at, in.mapping, location, date1904)
	if e != nil {
		return r, fmt.Errorf("时间无效：%w", e)
	}
	r = rawObservation{Tenant: j.tenant, Channel: binding.Channel, Batch: j.Target, At: tm.UnixNano(), Received: in.batch.CreatedAt.UnixNano(), Offset: offset, Precision: precision, Kind: sem.Kind, File: in.batch.SourceFile, Sheet: in.mapping.Sheet, Line: line, Column: col, Record: record.Text, Original: value.Text, Semantic: sem.Hash, Validation: 1, First: true, Shard: logicalShard(j.tenant, binding.Channel)}
	r.Mapping, _ = strconv.ParseUint(in.batch.MappingRevision, 10, 64)
	if sem.TimeMeaning == "interval_end" {
		if start.Text == "" {
			return r, fmt.Errorf("区间结束语义必须映射 interval_start。")
		}
		st, _, _, e := inputTime(start, in.mapping, location, date1904)
		if e != nil {
			return r, e
		}
		ns := st.UnixNano()
		if ns >= r.At {
			return r, fmt.Errorf("区间起点必须早于观测终点。")
		}
		r.Start = &ns
	} else if start.Text != "" {
		return r, fmt.Errorf("观测时刻通道不能附带区间起点。")
	}
	allowed := false
	for _, w := range in.snapshot.Windows[r.Channel] {
		if w.Contains(r.At, r.Start) {
			allowed = true
			break
		}
	}
	if !allowed {
		return r, fmt.Errorf("整条观测及其区间不在当前导入授权范围。")
	}
	var coefficient *big.Int
	switch sem.Kind {
	case "integer":
		v, e := observation.Integer(value.Text)
		if e != nil {
			return r, e
		}
		r.Value = strconv.FormatInt(v, 10)
		coefficient = new(big.Int).Mul(big.NewInt(v), big.NewInt(1000000000000))
	case "decimal":
		v, e := observation.Decimal(value.Text)
		if e != nil {
			return r, e
		}
		coefficient = v
		r.Value = observation.Format(v, 12)
	case "enum":
		r.Value = value.Text
		var definition struct {
			Options []struct {
				Key string `json:"key"`
			} `json:"options"`
		}
		if e = json.Unmarshal(sem.Definition, &definition); e != nil {
			return r, e
		}
		found := false
		for _, opt := range definition.Options {
			if opt.Key == r.Value {
				found = true
				break
			}
		}
		if !found {
			return r, fmt.Errorf("枚举必须使用固定指标版本的稳定状态键。")
		}
	default:
		return r, fmt.Errorf("不支持的指标值类型。")
	}
	if value.Numeric && coefficient != nil {
		digits := strings.TrimLeft(strings.TrimRight(new(big.Int).Abs(coefficient).String(), "0"), "0")
		integerPart := strings.TrimLeft(strings.Split(strings.TrimPrefix(r.Value, "-"), ".")[0], "0")
		if len(digits) > 15 || len(integerPart) > 15 {
			return r, fmt.Errorf("Excel 长整数／高精度数值必须保存为文本单元格，已有数字精度损失不可恢复。")
		}
	}
	source := quality.Text
	if mapped, ok := in.mapping.QualityMap[source]; ok {
		source = mapped
	}
	r.SourceQuality = quality.Text
	r.Quality, _, e = observation.Quality(source, nil, nil, nil)
	if e != nil {
		return r, e
	}
	var bounds struct{ Minimum, Maximum json.RawMessage }
	if e = json.Unmarshal(sem.Definition, &bounds); e != nil {
		return r, e
	}
	outside := false
	if coefficient != nil {
		number := new(big.Rat).SetFrac(coefficient, big.NewInt(1000000000000))
		for _, bound := range []struct {
			raw     json.RawMessage
			minimum bool
		}{{bounds.Minimum, true}, {bounds.Maximum, false}} {
			if len(bound.raw) == 0 || string(bound.raw) == "null" {
				continue
			}
			limit, ok := new(big.Rat).SetString(string(bound.raw))
			if !ok {
				return r, fmt.Errorf("指标边界不是有效精确数值。")
			}
			cmp := number.Cmp(limit)
			if bound.minimum && cmp < 0 || !bound.minimum && cmp > 0 {
				outside = true
			}
		}
	}
	r.RuleQuality = "bounds-v1"
	if outside {
		r.QualityReason = "outside_metric_bounds"
		if r.Quality != "bad" {
			r.Quality = "suspect"
		}
	}
	origin := uuid.MustParse(j.Target)
	r.Observation = uuid.NewSHA1(origin, encodeJSON([]any{"observation-v1", r.Mapping, r.Sheet, r.Line, r.Column, r.Channel})).String()
	r.Adoption = uuid.NewSHA1(origin, []byte("adoption-v1:"+r.Observation)).String()
	return r, nil
}

func (s *Service) Issues(ctx context.Context, a access.Actor, scope Scope, id, cursor string) (IssuePage, error) {
	out := IssuePage{Items: []ImportIssue{}}
	if e := s.requireStorage(); e != nil {
		return out, e
	}
	offset := int64(0)
	if cursor != "" {
		var e error
		offset, e = strconv.ParseInt(cursor, 10, 64)
		if e != nil || offset < 0 {
			return out, invalid("cursor", "无效问题游标。")
		}
	}
	var object string
	var revision string
	e := s.transact(ctx, a, scope, "imports.issues", false, false, func(tx pgx.Tx, auth authorization) error {
		b, e := batchTx(ctx, tx, auth, id, false)
		if e != nil {
			return e
		}
		out.PreviewHash = b.ValidationHash
		revision = authorizationVersion(auth)
		if scope.Entry == "project" {
			var snapshot []byte
			if e = tx.QueryRow(ctx, `SELECT semantic_snapshot FROM monitoring_import_batches WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id).Scan(&snapshot); e != nil {
				return e
			}
			var snap validationSnapshot
			if e = json.Unmarshal(snapshot, &snap); e != nil {
				return e
			}
			if snap.Authorization != revision {
				return problem("authorization_changed", "授权已变化，请重新校验后查看范围内的问题。", 409)
			}
		}
		return tx.QueryRow(ctx, `SELECT COALESCE(report_object,'') FROM monitoring_import_batches WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id).Scan(&object)
	})
	if e != nil || object == "" {
		return out, e
	}
	file, e := s.storage.get(ctx, object)
	if e != nil {
		return out, e
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	var index int64
	for {
		var issue ImportIssue
		e = decoder.Decode(&issue)
		if e == io.EOF {
			break
		}
		if e != nil {
			return out, e
		}
		if index >= offset {
			out.Items = append(out.Items, issue)
			if len(out.Items) > 100 {
				out.Items = out.Items[:100]
				out.NextCursor = countText(offset + 100)
				break
			}
		}
		index++
	}
	e = s.transact(ctx, a, scope, "imports.issues", false, false, func(_ pgx.Tx, auth authorization) error {
		if revision != authorizationVersion(auth) {
			return revisionChanged()
		}
		return nil
	})
	return out, e
}
