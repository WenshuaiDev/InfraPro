package resultstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
)

type blockHeader struct {
	Layout  string `json:"layout_version"`
	Tenant  string `json:"tenant_id"`
	Chunk   string `json:"chunk_id"`
	Attempt string `json:"attempt_id"`
	Rows    uint64 `json:"row_count,string"`
	Hash    string `json:"payload_hash"`
}
type boundedWriter struct {
	writer  io.Writer
	written uint64
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	if uint64(len(p)) > MaxBlockBytes-w.written {
		return 0, fmt.Errorf("result_block_budget_exceeded")
	}
	n, e := w.writer.Write(p)
	w.written += uint64(n)
	return n, e
}
func nullableTime(v *int64) *time.Time {
	if v == nil {
		return nil
	}
	t := time.Unix(0, *v).UTC()
	return &t
}
func nullableUUID(v *string) *uuid.UUID {
	if v == nil {
		return nil
	}
	id := uuid.MustParse(*v)
	return &id
}

// Write never retries an uncertain physical write. The caller must abandon an
// unsuccessful attempt and allocate a fresh attempt before trying again. fence
// is required at each external-I/O boundary; visibility remains a separate PG
// publication transaction, even after this method returns a verified block.
func (s *Store) Write(ctx context.Context, physical, chunk, attempt string, rows []Row, fence func(context.Context) error) (Block, error) {
	block := Block{Physical: physical, Chunk: chunk, Attempt: attempt, Rows: uint64(len(rows)), Layout: LayoutVersion}
	if fence == nil || len(rows) == 0 || len(rows) > MaxBlockRows {
		return block, fmt.Errorf("invalid_result_block")
	}
	if _, e := uuid.Parse(chunk); e != nil {
		return block, e
	}
	if _, e := uuid.Parse(attempt); e != nil {
		return block, e
	}
	if e := fence(ctx); e != nil {
		return block, e
	}
	g, e := s.transport.Group(physical)
	if e != nil {
		return block, e
	}
	if len(g.Replicas) == 0 {
		return block, fmt.Errorf("result_replica_unavailable")
	}
	block.Endpoints = &Endpoints{}
	summary := &monitoring.CalculationSummaryBuilder{}
	first := rows[0]
	block.Tenant = first.Tenant
	block.Output = first.Output
	firstAt, lastAt := first.At, rows[len(rows)-1].At
	block.First, block.Last = &firstAt, &lastAt
	digest := sha256.New()
	month := time.Unix(0, first.At).UTC().Format("200601")
	for i, r := range rows {
		if i%128 == 0 {
			if e = ctx.Err(); e != nil {
				return block, e
			}
		}
		if e = r.Validate(); e != nil {
			return block, e
		}
		if r.Tenant != first.Tenant || r.Series != first.Series || r.Generation != first.Generation || r.Config != first.Config || r.Run != first.Run || r.Output != first.Output || time.Unix(0, r.At).UTC().Format("200601") != month {
			return block, fmt.Errorf("result_block_partition_mismatch")
		}
		if i > 0 && r.At <= rows[i-1].At {
			return block, fmt.Errorf("result_block_not_uniquely_ordered")
		}
		block.Endpoints.Add(r)
		if e = summary.Add(r.Observation()); e != nil {
			return block, e
		}
		_, _ = io.WriteString(digest, r.Hash())
	}
	block.Summary = summary.Summary()
	block.Hash = hex.EncodeToString(digest.Sum(nil))
	file, e := os.CreateTemp(s.transport.TempDirectory(), "derived-result-*.jsonl")
	if e != nil {
		return block, e
	}
	defer os.Remove(file.Name())
	defer file.Close()
	objectDigest := sha256.New()
	writer := &boundedWriter{writer: io.MultiWriter(file, objectDigest)}
	encoder := json.NewEncoder(writer)
	if e = encoder.Encode(blockHeader{LayoutVersion, block.Tenant, chunk, attempt, block.Rows, block.Hash}); e != nil {
		return block, e
	}
	for _, r := range rows {
		r.Reasons = append([]string{}, r.Reasons...)
		if e = encoder.Encode(r); e != nil {
			return block, e
		}
	}
	block.Bytes = writer.written
	block.ObjectHash = hex.EncodeToString(objectDigest.Sum(nil))
	block.ObjectKey = block.Tenant + "/derived/results/" + chunk + "/" + attempt + ".jsonl"
	if e = fence(ctx); e != nil {
		return block, e
	}
	writeContext := clickhouse.Context(s.transport.QueryContext(ctx), clickhouse.WithSettings(clickhouse.Settings{"insert_quorum": len(g.Replicas), "insert_quorum_timeout": 30000, "insert_deduplicate": 0, "async_insert": 0}))
	batch, e := g.Replicas[0].PrepareBatch(writeContext, "INSERT INTO "+g.Database+".derived_results_local")
	if e != nil {
		return block, e
	}
	defer batch.Close()
	lastCheck := time.Now()
	for i, r := range rows {
		if i%128 == 0 {
			if e = ctx.Err(); e != nil {
				return block, e
			}
			if time.Since(lastCheck) >= time.Second {
				if e = fence(ctx); e != nil {
					return block, e
				}
				lastCheck = time.Now()
			}
		}
		var coefficient *big.Int
		if r.Coefficient != nil {
			coefficient, _ = new(big.Int).SetString(*r.Coefficient, 10)
		}
		e = batch.Append(uuid.MustParse(r.Tenant), uuid.MustParse(r.Series), nullableUUID(r.Channel), nullableUUID(r.Artifact), uuid.MustParse(r.Result), uuid.MustParse(r.Generation), uuid.MustParse(r.Run), uuid.MustParse(r.Config), r.Output, time.Unix(0, r.At).UTC(), nullableTime(r.Start), nullableTime(r.End), coefficient, r.Scale, r.Rounded, r.Quality, r.Status, r.Reasons, r.Count, r.Excluded, nullableTime(r.InputFirst), nullableTime(r.InputLast), r.Lineage, uuid.MustParse(chunk), uuid.MustParse(attempt), uint64(i), r.Hash())
		if e != nil {
			return block, e
		}
	}
	if e = batch.Send(); e != nil {
		return block, fmt.Errorf("result_write_uncertain: %w", e)
	}
	if e = fence(ctx); e != nil {
		return block, e
	}
	for _, replica := range g.Replicas {
		if e = verifyReplica(ctx, g.Database, replica, block); e != nil {
			return block, e
		}
	}
	if e = fence(ctx); e != nil {
		return block, e
	}
	if _, e = file.Seek(0, io.SeekStart); e != nil {
		return block, e
	}
	if e = s.transport.Put(ctx, block.ObjectKey, file, int64(block.Bytes), "application/x-ndjson"); e != nil {
		return block, e
	}
	if e = s.verifyObject(ctx, block); e != nil {
		return block, e
	}
	return block, fence(ctx)
}

const selectPayload = `toString(tenant_id),toString(series_id),ifNull(toString(channel_id),''),ifNull(toString(artifact_id),''),toString(result_id),toString(generation_id),toString(run_id),toString(config_id),output_key,toUnixTimestamp64Nano(output_at),toUnixTimestamp64Nano(interval_start),toUnixTimestamp64Nano(interval_end),coefficient,scale,rounded,quality,calculation_status,reason_flags,sample_count,excluded_count,toUnixTimestamp64Nano(input_first),toUnixTimestamp64Nano(input_last),lineage_ref,row_ordinal,payload_hash`

func scanRow(rows driver.Rows) (Row, uint64, string, error) {
	var r Row
	var channel, artifact, hash string
	var ordinal uint64
	var coefficient *big.Int
	e := rows.Scan(&r.Tenant, &r.Series, &channel, &artifact, &r.Result, &r.Generation, &r.Run, &r.Config, &r.Output, &r.At, &r.Start, &r.End, &coefficient, &r.Scale, &r.Rounded, &r.Quality, &r.Status, &r.Reasons, &r.Count, &r.Excluded, &r.InputFirst, &r.InputLast, &r.Lineage, &ordinal, &hash)
	if channel != "" {
		r.Channel = &channel
	}
	if artifact != "" {
		r.Artifact = &artifact
	}
	if coefficient != nil {
		v := coefficient.String()
		r.Coefficient = &v
	}
	return r, ordinal, hash, e
}
func scanBlock(ctx context.Context, database string, replica driver.Conn, block Block, consume func(Row) error) error {
	rows, e := replica.Query(ctx, "SELECT "+selectPayload+" FROM "+database+".derived_results_local WHERE tenant_id=? AND chunk_id=? AND attempt_id=? ORDER BY row_ordinal", uuid.MustParse(block.Tenant), uuid.MustParse(block.Chunk), uuid.MustParse(block.Attempt))
	if e != nil {
		return e
	}
	defer rows.Close()
	digest := sha256.New()
	var count uint64
	endpoints := &Endpoints{}
	summary := &monitoring.CalculationSummaryBuilder{}
	for rows.Next() {
		if count%128 == 0 {
			if e = ctx.Err(); e != nil {
				return e
			}
		}
		r, ordinal, hash, e := scanRow(rows)
		if e != nil {
			return e
		}
		if count >= block.Rows || ordinal != count || r.Hash() != hash {
			return fmt.Errorf("result_payload_mismatch")
		}
		if e = r.Validate(); e != nil {
			return e
		}
		if block.Output != "" && (r.Output != block.Output || r.At < *block.First || r.At > *block.Last || count == 0 && r.At != *block.First || count+1 == block.Rows && r.At != *block.Last) {
			return fmt.Errorf("result_block_index_mismatch")
		}
		endpoints.Add(r)
		if block.Summary != nil {
			if e = summary.Add(r.Observation()); e != nil {
				return e
			}
		}
		_, _ = io.WriteString(digest, hash)
		count++
		if consume != nil {
			if e = consume(r); e != nil {
				return e
			}
		}
	}
	if e = rows.Err(); e != nil {
		return e
	}
	if block.Summary != nil {
		a, _ := json.Marshal(block.Summary)
		b, _ := json.Marshal(summary.Summary())
		if string(a) != string(b) {
			return fmt.Errorf("result_summary_mismatch")
		}
	}
	if block.Endpoints != nil {
		a, _ := json.Marshal(block.Endpoints)
		b, _ := json.Marshal(endpoints)
		if string(a) != string(b) {
			return fmt.Errorf("result_endpoint_index_mismatch")
		}
	}
	if count != block.Rows || hex.EncodeToString(digest.Sum(nil)) != block.Hash {
		return fmt.Errorf("result_block_incomplete")
	}
	return nil
}
func verifyReplica(ctx context.Context, database string, replica driver.Conn, block Block) error {
	return scanBlock(ctx, database, replica, block, nil)
}
func (s *Store) verifyObject(ctx context.Context, block Block) error {
	object, e := s.transport.Get(ctx, block.ObjectKey)
	if e != nil {
		return e
	}
	defer object.Close()
	digest := sha256.New()
	size, e := io.Copy(digest, io.LimitReader(object, MaxBlockBytes+1))
	if e != nil {
		return e
	}
	if uint64(size) != block.Bytes || size > MaxBlockBytes || hex.EncodeToString(digest.Sum(nil)) != block.ObjectHash {
		return fmt.Errorf("result_object_incomplete")
	}
	return nil
}
func validateBlock(block Block) error {
	if block.Output != "" && (block.First == nil || block.Last == nil || *block.First > *block.Last) || block.Output == "" && (block.First != nil || block.Last != nil) {
		return fmt.Errorf("invalid_result_block_index")
	}
	for _, id := range []string{block.Tenant, block.Chunk, block.Attempt} {
		if _, e := uuid.Parse(id); e != nil {
			return fmt.Errorf("invalid_result_manifest")
		}
	}
	if block.Rows == 0 || block.Rows > MaxBlockRows || block.Bytes == 0 || block.Bytes > MaxBlockBytes || block.Layout != LayoutVersion || len(block.Hash) != 64 || len(block.ObjectHash) != 64 || block.ObjectKey != block.Tenant+"/derived/results/"+block.Chunk+"/"+block.Attempt+".jsonl" {
		return fmt.Errorf("invalid_result_manifest")
	}
	for _, hash := range []string{block.Hash, block.ObjectHash} {
		if _, e := hex.DecodeString(hash); e != nil {
			return fmt.Errorf("invalid_result_manifest")
		}
	}
	return nil
}
func (s *Store) Verify(ctx context.Context, block Block) error {
	return s.VerifyRows(ctx, block, nil)
}

// VerifyRows checks every configured replica and the independent sealed object.
// The callback observes the first replica only after each row's own integrity
// has been checked; publication still requires this method to finish fully.
func (s *Store) VerifyRows(ctx context.Context, block Block, consume func(Row) error) error {
	if e := validateBlock(block); e != nil {
		return e
	}
	g, e := s.transport.Group(block.Physical)
	if e != nil {
		return e
	}
	if len(g.Replicas) == 0 {
		return fmt.Errorf("result_replica_unavailable")
	}
	for i, replica := range g.Replicas {
		visitor := consume
		if i != 0 {
			visitor = nil
		}
		if e = scanBlock(ctx, g.Database, replica, block, visitor); e != nil {
			return e
		}
	}
	return s.verifyObject(ctx, block)
}

// ReadBlock selects a complete replica. An incomplete replica never contributes
// a prefix that could be interpreted by the calculator as missing input data.
func (s *Store) ReadBlock(ctx context.Context, block Block, consume func(Row) error) error {
	if e := validateBlock(block); e != nil {
		return e
	}
	g, e := s.transport.Group(block.Physical)
	if e != nil {
		return e
	}
	for _, replica := range g.Replicas {
		if e = verifyReplica(ctx, g.Database, replica, block); e != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		return scanBlock(ctx, g.Database, replica, block, consume)
	}
	return fmt.Errorf("result_replica_unavailable: rows=%s", strconv.FormatUint(block.Rows, 10))
}
