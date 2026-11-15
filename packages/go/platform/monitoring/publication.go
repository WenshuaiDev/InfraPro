package monitoring

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type publishInput struct {
	batch     ImportBatch
	snapshot  validationSnapshot
	revisions []byte
	object    string
	route     int64
}

func (s *Service) loadPublication(ctx context.Context, j Job) (publishInput, error) {
	var out publishInput
	e := s.transact(ctx, workerActor(j.actor), j.scope(), "imports.publish", false, true, func(tx pgx.Tx, auth authorization) error {
		if e := assertJobTx(ctx, tx, j); e != nil {
			return e
		}
		var e error
		out.batch, e = batchTx(ctx, tx, auth, j.Target, true)
		if e != nil {
			return e
		}
		if out.batch.State != "preparing" || out.batch.Revision != j.TargetRevision {
			return revisionChanged()
		}
		var snapshot []byte
		if e = tx.QueryRow(ctx, `SELECT semantic_snapshot,content_revisions,validation_object FROM monitoring_import_batches WHERE tenant_id=$1 AND id=$2`, j.tenant, j.Target).Scan(&snapshot, &out.revisions, &out.object); e != nil {
			return e
		}
		if e = json.Unmarshal(snapshot, &out.snapshot); e != nil {
			return e
		}
		if authorizationVersion(auth) != out.snapshot.Authorization {
			return problem("authorization_changed", "授权已变化，需要重新全量校验。", 409)
		}
		if e = checkContentTx(ctx, tx, j.tenant, out.revisions, false); e != nil {
			return e
		}
		ids := []string{}
		for id := range out.snapshot.Semantics {
			ids = append(ids, id)
		}
		if _, e = authorizedWindowsTx(ctx, tx, auth, ids, "import"); e != nil {
			return e
		}
		if _, e = semanticsTx(ctx, tx, j.tenant, ids, true); e != nil {
			return e
		}
		if e = tx.QueryRow(ctx, `SELECT route_revision FROM monitoring_tenant_states WHERE tenant_id=$1`, j.tenant).Scan(&out.route); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO monitoring_routes(tenant_id,logical_shard,revision,physical_shard,state) SELECT $1,n,$2,'local','active' FROM generate_series(0,255) n ON CONFLICT DO NOTHING`, j.tenant, out.route)
		return e
	})
	return out, e
}

func (s *Service) publishJob(ctx context.Context, j Job) error {
	in, e := s.loadPublication(ctx, j)
	if e != nil {
		return e
	}
	temp, e := newSpool(s.storage.config.TempDirectory)
	if e != nil {
		return e
	}
	defer temp.close()
	object, e := s.storage.get(ctx, in.object)
	if e != nil {
		return e
	}
	it := rowsFrom(object)
	sorter := rowSorter{s: temp, compare: compareChunk}
	for it.next() {
		if e = ctx.Err(); e != nil {
			it.close()
			return e
		}
		if e = sorter.add(it.row); e != nil {
			it.close()
			return e
		}
	}
	it.close()
	if it.err != nil {
		return it.err
	}
	sorted, e := sorter.finish(ctx)
	if e != nil {
		return e
	}
	it, e = openRows(sorted)
	if e != nil {
		return e
	}
	defer it.close()
	entries := []selectedChunk{}
	buffer := []rawObservation{}
	size := 0
	index := 0
	actualCoverage := map[string]coverage{}
	var completedRows uint64
	if e = s.recordProgress(ctx, j, map[string]string{"phase": "preparing", "completed_chunks": "0", "rows": "0"}); e != nil {
		return e
	}
	flush := func() error {
		if len(buffer) == 0 {
			return nil
		}
		chunk := uuid.NewSHA1(uuid.MustParse(j.Target), []byte(fmt.Sprintf("chunk-v1:%s:%d", in.batch.ValidationHash, index))).String()
		index++
		entry, e := s.prepareChunk(ctx, j, in, chunk, buffer)
		if e != nil {
			return e
		}
		entries = append(entries, entry)
		completedRows += entry.Count
		if e = s.recordProgress(ctx, j, map[string]string{"phase": "writing", "completed_chunks": strconv.Itoa(len(entries)), "rows": strconv.FormatUint(completedRows, 10)}); e != nil {
			return e
		}
		buffer = nil
		size = 0
		return nil
	}
	for it.next() {
		r := it.row
		if len(buffer) > 0 && (buffer[0].Shard != r.Shard || monthOf(buffer[0]) != monthOf(r) || len(buffer) >= 100000 || size >= 16*1024*1024) {
			if e = flush(); e != nil {
				return e
			}
		}
		buffer = append(buffer, r)
		size += len(encodeJSON(r))
		first := r.At
		if r.Start != nil {
			first = *r.Start
		}
		cover, ok := actualCoverage[r.Channel]
		if !ok {
			cover = coverage{First: first, Last: r.At}
		}
		cover.First = min(cover.First, first)
		cover.Last = max(cover.Last, r.At)
		cover.Count++
		actualCoverage[r.Channel] = cover
	}
	if it.err != nil {
		return it.err
	}
	if e = flush(); e != nil {
		return e
	}
	var adopted uint64
	for _, entry := range entries {
		adopted += entry.Count
	}
	expected, _ := strconv.ParseUint(in.batch.Counts.Proposed, 10, 64)
	if adopted != expected {
		return fmt.Errorf("publication candidate count mismatch")
	}
	// Final verification precedes the short PG commit. There is no CH scan while
	// authorization, account, channel or publication-sequence locks are held.
	if e = s.recordProgress(ctx, j, map[string]string{"phase": "verifying", "completed_chunks": strconv.Itoa(len(entries)), "rows": strconv.FormatUint(completedRows, 10)}); e != nil {
		return e
	}
	for _, entry := range entries {
		if e = s.storage.sealCheck(ctx, entry); e != nil {
			return e
		}
	}
	return s.transact(ctx, workerActor(j.actor), j.scope(), "imports.publish", false, true, func(tx pgx.Tx, auth authorization) error {
		if e := assertJobTx(ctx, tx, j); e != nil {
			return e
		}
		b, e := batchTx(ctx, tx, auth, j.Target, true)
		if e != nil {
			return e
		}
		if b.State != "preparing" || b.Revision != j.TargetRevision {
			return revisionChanged()
		}
		if authorizationVersion(auth) != in.snapshot.Authorization {
			return problem("authorization_changed", "授权已变化，需要重新全量核对。", 409)
		}
		ids := []string{}
		for id := range in.snapshot.Semantics {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		if _, e = authorizedWindowsTx(ctx, tx, auth, ids, "import"); e != nil {
			return e
		}
		if _, e = semanticsTx(ctx, tx, j.tenant, ids, true); e != nil {
			return e
		}
		if e = checkContentTx(ctx, tx, j.tenant, in.revisions, true); e != nil {
			return e
		}
		var seq, route int64
		if e = tx.QueryRow(ctx, `SELECT commit_sequence,route_revision FROM monitoring_tenant_states WHERE tenant_id=$1 FOR UPDATE`, j.tenant).Scan(&seq, &route); e != nil {
			return e
		}
		if route != in.route {
			return problem("route_changed", "存储路由已变化，请重新准备发布。", 409)
		}
		for _, id := range ids {
			var active bool
			if e = tx.QueryRow(ctx, `SELECT state='active' FROM monitoring_routes WHERE tenant_id=$1 AND logical_shard=$2 AND revision=$3`, j.tenant, logicalShard(j.tenant, id), route).Scan(&active); e != nil {
				return e
			}
			if !active {
				return problem("route_barrier", "分片正在迁移，稍后重试。", 409)
			}
		}
		state := "completed_noop"
		if len(entries) > 0 {
			state = "published"
			seq++
			var publication string
			if _, e = tx.Exec(ctx, `UPDATE monitoring_tenant_states SET commit_sequence=$2 WHERE tenant_id=$1`, j.tenant, seq); e != nil {
				return e
			}
			if e = tx.QueryRow(ctx, `INSERT INTO monitoring_publications(tenant_id,batch_id,sequence,manifest_hash,committed_by) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, j.tenant, j.Target, seq, digest(entries), j.actor).Scan(&publication); e != nil {
				return e
			}
			for _, entry := range entries {
				if _, e = tx.Exec(ctx, `INSERT INTO monitoring_publication_chunks(tenant_id,publication_id,chunk_id,attempt_id,adopted_count,first_adopted_count,quality_counts) VALUES($1,$2,$3,$4,$5,$6,$7)`, j.tenant, publication, entry.Chunk, entry.Attempt, entry.Count, entry.First, encodeJSON(entry.Qualities)); e != nil {
					return e
				}
			}
			changes := []ChangeRange{}
			for _, id := range ids {
				// Only actual contributions dirty a channel. Ignored duplicate sources do
				// not acquire a new adoption and cannot be revived by revoking this batch.
				cover, ok := actualCoverage[id]
				if !ok {
					continue
				}
				if _, e = tx.Exec(ctx, `UPDATE monitoring_channel_versions SET content_revision=content_revision+1,first_published_sequence=COALESCE(first_published_sequence,$3) WHERE tenant_id=$1 AND channel_id=$2`, j.tenant, id, seq); e != nil {
					return e
				}
				var upper *int64
				if cover.Last < 1<<63-1 {
					v := cover.Last + 1
					upper = &v
				}
				w := Window{From: &cover.First, To: upper}
				changes = append(changes, ChangeRange{Channel: id, From: w.From, To: w.To})
				if _, e = tx.Exec(ctx, `INSERT INTO monitoring_dirty_ranges(tenant_id,channel_id,affected,target_sequence) VALUES($1,$2,$3::int8range,$4)`, j.tenant, id, w.pg(), seq); e != nil {
					return e
				}
			}
			var readopt bool
			if e = tx.QueryRow(ctx, `SELECT readopt_batch_id IS NOT NULL FROM monitoring_import_batches WHERE tenant_id=$1 AND id=$2`, j.tenant, j.Target).Scan(&readopt); e != nil {
				return e
			}
			kind := "raw_publish"
			if readopt {
				kind = "raw_readopt"
			}
			if _, e = AppendChangeTx(ctx, tx, j.tenant, seq, kind, "direct", publication, changes); e != nil {
				return e
			}
		}
		if len(entries) > 0 {
			affected := []string{}
			for id := range actualCoverage {
				affected = append(affected, id)
			}
			if e = enqueueSummariesTx(ctx, tx, j.tenant, affected, seq); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `UPDATE monitoring_import_batches SET state=$3,semantic_snapshot=jsonb_set(semantic_snapshot,'{actual_coverage}',$4::jsonb),revision=revision+1,error_code=NULL,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2`, j.tenant, j.Target, state, encodeJSON(actualCoverage)); e != nil {
			return e
		}
		if e = s.FinalizeTaskTx(ctx, tx, j.ID, j.fence, "succeeded", "", map[string]string{"adopted": strconv.FormatUint(adopted, 10), "sequence": strconv.FormatInt(seq, 10)}); e != nil {
			return e
		}
		var meta WriteMeta
		_ = json.Unmarshal(j.input, &meta)
		summary := map[string]string{"state": state, "adopted": strconv.FormatUint(adopted, 10), "sequence": strconv.FormatInt(seq, 10)}
		if e = auditTx(ctx, tx, auth, "imports.publish", j.Target, meta.Reason, meta.RequestID, summary); e != nil {
			return e
		}
		var generation string
		if e = tx.QueryRow(ctx, `SELECT notification_generation::text FROM monitoring_jobs WHERE id=$1`, j.ID).Scan(&generation); e != nil {
			return e
		}
		return auditPublishedTx(ctx, tx, auth, j.Target, j.ID, generation, meta.Reason, summary)
	})
}

func (s *Service) prepareChunk(ctx context.Context, j Job, in publishInput, chunk string, data []rawObservation) (selectedChunk, error) {
	entry := selectedChunk{Chunk: chunk, Hash: payloadHash(data), Count: uint64(len(data)), Batch: j.Target, Qualities: map[string]string{"good": "0", "unmarked": "0", "suspect": "0", "bad": "0"}}
	for _, r := range data {
		n, _ := strconv.ParseUint(entry.Qualities[r.Quality], 10, 64)
		entry.Qualities[r.Quality] = strconv.FormatUint(n+1, 10)
		if r.First {
			entry.First++
		}
	}
	first, last := data[0].At, data[0].At
	for _, r := range data {
		first = min(first, r.At)
		last = max(last, r.At)
	}
	source := j.tenant + "/chunks/" + chunk + "/" + uuid.NewString()
	var body bytes.Buffer
	writer := bufio.NewWriter(&body)
	enc := json.NewEncoder(writer)
	for _, r := range data {
		if e := enc.Encode(r); e != nil {
			return entry, e
		}
	}
	if e := writer.Flush(); e != nil {
		return entry, e
	}
	if e := s.storage.put(ctx, source, &body, int64(body.Len()), "application/x-ndjson"); e != nil {
		return entry, e
	}
	summaryObject, e := s.storage.contributions(ctx, j.tenant, data)
	if e != nil {
		return entry, e
	}
	e = s.transact(ctx, workerActor(j.actor), j.scope(), "imports.publish", false, true, func(tx pgx.Tx, auth authorization) error {
		if e := assertJobTx(ctx, tx, j); e != nil {
			return e
		}
		b, e := batchTx(ctx, tx, auth, j.Target, true)
		if e != nil {
			return e
		}
		if b.Revision != j.TargetRevision || b.State != "preparing" {
			return revisionChanged()
		}
		var physical, state string
		if e = tx.QueryRow(ctx, `SELECT physical_shard,state FROM monitoring_routes WHERE tenant_id=$1 AND logical_shard=$2 AND revision=$3`, j.tenant, data[0].Shard, in.route).Scan(&physical, &state); e != nil {
			return e
		}
		if state != "active" {
			return problem("route_barrier", "分片正在迁移，稍后重试。", 409)
		}
		entry.Physical = physical
		if _, e = s.storage.physical(physical); e != nil {
			return e
		}
		var hash string
		var count int64
		e = tx.QueryRow(ctx, `SELECT payload_hash,row_count FROM monitoring_chunks WHERE tenant_id=$1 AND id=$2`, j.tenant, chunk).Scan(&hash, &count)
		if e == nil {
			if hash != entry.Hash || uint64(count) != entry.Count {
				return fmt.Errorf("logical chunk changed")
			}
		} else if e == pgx.ErrNoRows {
			if _, e = tx.Exec(ctx, `INSERT INTO monitoring_chunks(id,tenant_id,batch_id,mapping_revision,logical_shard,utc_month,first_ns,last_ns,row_count,payload_hash,source_object,summary_object) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, chunk, j.tenant, j.Target, in.batch.MappingRevision, data[0].Shard, monthOf(data[0]), first, last, len(data), entry.Hash, source, summaryObject); e != nil {
				return e
			}
		} else {
			return e
		}
		// Reuse only a previously sealed, verified attempt. A writing/uncertain
		// attempt is never resumed, even when its rows happen to be present.
		e = tx.QueryRow(ctx, `SELECT id::text FROM monitoring_write_attempts WHERE tenant_id=$1 AND chunk_id=$2 AND state='sealed' AND route_revision=$3 ORDER BY created_at,id LIMIT 1`, j.tenant, chunk, in.route).Scan(&entry.Attempt)
		if e == nil {
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		entry.Attempt = uuid.NewString()
		_, e = tx.Exec(ctx, `INSERT INTO monitoring_write_attempts(id,tenant_id,chunk_id,fence,state,physical_shard,route_revision) VALUES($1,$2,$3,$4,'writing',$6,$5)`, entry.Attempt, j.tenant, chunk, j.fence, in.route, entry.Physical)
		return e
	})
	if e != nil {
		return entry, e
	}
	// An already sealed attempt may be reselected after a crash, with fresh
	// replica readiness validation, but must never be sent to INSERT again.
	var sealed bool
	e = s.db(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT state='sealed' FROM monitoring_write_attempts WHERE id=$1`, entry.Attempt).Scan(&sealed)
	})
	if e != nil {
		return entry, e
	}
	if !sealed {
		e = s.storage.writeAt(ctx, entry.Physical, chunk, entry.Attempt, data)
		if e == nil {
			e = s.storage.sealCheck(ctx, entry)
		}
		if e != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = s.db(cleanup, func(tx pgx.Tx) error {
				_, err := tx.Exec(cleanup, `UPDATE monitoring_write_attempts SET state='uncertain' WHERE id=$1 AND state='writing'`, entry.Attempt)
				return err
			})
			return entry, e
		}
		e = s.transact(ctx, workerActor(j.actor), j.scope(), "imports.publish", false, true, func(tx pgx.Tx, _ authorization) error {
			if e := assertJobTx(ctx, tx, j); e != nil {
				return e
			}
			_, e := tx.Exec(ctx, `UPDATE monitoring_write_attempts SET state='sealed',verification_hash=$2,replica_epochs=$3,sealed_at=clock_timestamp() WHERE id=$1 AND state='writing' AND fence=$4`, entry.Attempt, entry.Hash, encodeJSON(map[string]any{"replicas": s.storage.config.Replicas, "verified": "full-payload-v1"}), j.fence)
			return e
		})
		if e != nil {
			return entry, e
		}
	}
	return entry, s.storage.sealCheck(ctx, entry)
}
