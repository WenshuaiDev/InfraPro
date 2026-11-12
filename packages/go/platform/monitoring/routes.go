package monitoring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Operator methods are intentionally reachable only through the CLI's database
// credentials. HTTP handlers never accept a user-supplied operator identity.
type RoutePlan struct {
	Tenant    string `json:"tenant_id"`
	Logical   int    `json:"logical_shard"`
	Source    string `json:"source_shard"`
	Target    string `json:"target_shard"`
	Revision  string `json:"route_revision"`
	Watermark string `json:"watermark"`
	Chunks    string `json:"chunks"`
	Rows      string `json:"rows"`
	Manifest  string `json:"manifest_hash"`
	Hash      string `json:"plan_hash"`
}
type Relocation struct {
	ID       string    `json:"id"`
	State    string    `json:"state"`
	Revision string    `json:"revision"`
	Plan     RoutePlan `json:"plan"`
	Copied   string    `json:"copied_chunks"`
	JobID    string    `json:"job_id,omitempty"`
	Created  time.Time `json:"created_at"`
}

func routePlanTx(ctx context.Context, tx pgx.Tx, tenant string, logical int, target string) (RoutePlan, error) {
	p := RoutePlan{Tenant: tenant, Logical: logical, Target: target}
	e := tx.QueryRow(ctx, `SELECT s.route_revision::text,s.commit_sequence::text,r.physical_shard FROM monitoring_tenant_states s JOIN monitoring_routes r ON r.tenant_id=s.tenant_id AND r.revision=s.route_revision AND r.logical_shard=$2 WHERE s.tenant_id=$1`, tenant, logical).Scan(&p.Revision, &p.Watermark, &p.Source)
	if e != nil {
		return p, e
	}
	if p.Source == target {
		return p, fmt.Errorf("target already owns this logical shard")
	}
	rows, e := tx.Query(ctx, `SELECT c.id::text,c.payload_hash,c.row_count,p.sequence FROM monitoring_chunks c JOIN monitoring_publication_chunks pc ON pc.tenant_id=c.tenant_id AND pc.chunk_id=c.id JOIN monitoring_publications p ON p.tenant_id=pc.tenant_id AND p.id=pc.publication_id WHERE c.tenant_id=$1 AND c.logical_shard=$2 AND p.sequence<=$3 ORDER BY c.id`, tenant, logical, p.Watermark)
	if e != nil {
		return p, e
	}
	defer rows.Close()
	h := sha256.New()
	var chunks, count int64
	for rows.Next() {
		var id, hash string
		var n, seq int64
		if e = rows.Scan(&id, &hash, &n, &seq); e != nil {
			return p, e
		}
		fmt.Fprintf(h, "%s:%s:%d:%d\n", id, hash, n, seq)
		chunks++
		count += n
	}
	if e = rows.Err(); e != nil {
		return p, e
	}
	p.Chunks = strconv.FormatInt(chunks, 10)
	p.Rows = strconv.FormatInt(count, 10)
	p.Manifest = hex.EncodeToString(h.Sum(nil))
	p.Hash = digest(p)
	return p, nil
}
func (s *Service) PlanRoute(ctx context.Context, tenant string, logical int, target string) (RoutePlan, error) {
	var p RoutePlan
	if !access.ValidID(tenant) || logical < 0 || logical > 255 {
		return p, fmt.Errorf("tenant and logical shard are required")
	}
	if e := s.requireStorage(); e != nil {
		return p, e
	}
	dest, e := s.storage.physical(target)
	if e != nil {
		return p, e
	}
	if e = dest.Check(ctx); e != nil {
		return p, e
	}
	e = s.db(ctx, func(tx pgx.Tx) error { var e error; p, e = routePlanTx(ctx, tx, tenant, logical, target); return e })
	return p, e
}
func operatorAuditTx(ctx context.Context, tx pgx.Tx, tenant, operation, target, reason string, value any) error {
	trace := TraceID(ctx)
	if trace == "" {
		trace = uuid.NewString()
	}
	_, e := tx.Exec(ctx, `INSERT INTO monitoring_audit(tenant_id,operation,target_id,reason,trace_id,summary) VALUES($1,$2,$3,$4,$5,$6)`, tenant, operation, target, reason, trace, encodeJSON(value))
	return e
}
func (s *Service) CreateRelocation(ctx context.Context, p RoutePlan, reason string) (Relocation, error) {
	var out Relocation
	if e := access.ValidReason(reason); e != nil {
		return out, e
	}
	if _, e := s.storage.physical(p.Target); e != nil {
		return out, e
	}
	e := s.db(ctx, func(tx pgx.Tx) error {
		var route string
		if e := tx.QueryRow(ctx, `SELECT route_revision::text FROM monitoring_tenant_states WHERE tenant_id=$1 FOR UPDATE`, p.Tenant).Scan(&route); e != nil {
			return e
		}
		current, e := routePlanTx(ctx, tx, p.Tenant, p.Logical, p.Target)
		if e != nil {
			return e
		}
		if route != p.Revision || current.Hash != p.Hash {
			return revisionChanged()
		}
		var conflict bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM monitoring_relocations WHERE tenant_id=$1 AND logical_shard=$2 AND state IN ('planned','copying','paused','verifying','barrier','reverting'))`, p.Tenant, p.Logical).Scan(&conflict); e != nil {
			return e
		}
		if conflict {
			return fmt.Errorf("an unfinished relocation already exists")
		}
		if e = tx.QueryRow(ctx, `INSERT INTO monitoring_relocations(tenant_id,logical_shard,source_revision,target_shard,state,checkpoint) VALUES($1,$2,$3,$4,'planned',$5) RETURNING id::text`, p.Tenant, p.Logical, p.Revision, p.Target, encodeJSON(p)).Scan(&out.ID); e != nil {
			return e
		}
		return operatorAuditTx(ctx, tx, p.Tenant, "operator.routes.create", out.ID, reason, p)
	})
	if e != nil {
		return out, e
	}
	return s.Relocation(ctx, p.Tenant, out.ID)
}
func relocationTx(ctx context.Context, tx pgx.Tx, tenant, id string) (Relocation, error) {
	var out Relocation
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT r.id::text,r.state,r.revision::text,r.checkpoint,r.created_at,(SELECT count(*)::text FROM monitoring_relocation_copies c WHERE c.tenant_id=r.tenant_id AND c.relocation_id=r.id),COALESCE((SELECT j.id::text FROM monitoring_jobs j WHERE j.tenant_id=r.tenant_id AND j.target_id=r.id AND j.kind='relocate' ORDER BY j.created_at DESC LIMIT 1),'') FROM monitoring_relocations r WHERE r.tenant_id=$1 AND r.id=$2`, tenant, id).Scan(&out.ID, &out.State, &out.Revision, &raw, &out.Created, &out.Copied, &out.JobID)
	if e == nil {
		e = json.Unmarshal(raw, &out.Plan)
	}
	return out, e
}
func (s *Service) Relocation(ctx context.Context, tenant, id string) (Relocation, error) {
	var out Relocation
	e := s.db(ctx, func(tx pgx.Tx) error { var e error; out, e = relocationTx(ctx, tx, tenant, id); return e })
	return out, e
}

// QueueRoute returns a durable leased job. "copy" keeps the source serving;
// "switch" catches up under a publication barrier and installs a new version.
func (s *Service) QueueRoute(ctx context.Context, tenant, id, expected, mode, reason string) (string, error) {
	if mode != "copy" && mode != "switch" {
		return "", fmt.Errorf("route mode must be copy or switch")
	}
	if e := access.ValidReason(reason); e != nil {
		return "", e
	}
	var job string
	e := s.db(ctx, func(tx pgx.Tx) error {
		var revision string
		var state string
		if e := tx.QueryRow(ctx, `SELECT revision::text,state FROM monitoring_relocations WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenant, id).Scan(&revision, &state); e != nil {
			return e
		}
		if revision != expected {
			return revisionChanged()
		}
		if state == "switched" || state == "completed" {
			return fmt.Errorf("relocation already switched")
		}
		var running bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM monitoring_jobs WHERE tenant_id=$1 AND target_id=$2 AND kind='relocate' AND state IN ('queued','running','retry_wait'))`, tenant, id).Scan(&running); e != nil {
			return e
		}
		if running {
			return fmt.Errorf("relocation already has an active job")
		}
		if _, e := tx.Exec(ctx, `UPDATE monitoring_relocations SET state='copying',revision=revision+1 WHERE tenant_id=$1 AND id=$2`, tenant, id); e != nil {
			return e
		}
		input := map[string]string{"mode": mode, "reason": reason}
		if e := tx.QueryRow(ctx, `INSERT INTO monitoring_jobs(tenant_id,kind,target_id,target_revision,input_hash,input) VALUES($1,'relocate',$2,$3,$4,$5) RETURNING id::text`, tenant, id, mustRevision(revision)+1, digest(input), encodeJSON(input)).Scan(&job); e != nil {
			return e
		}
		return operatorAuditTx(ctx, tx, tenant, "operator.routes."+mode, id, reason, map[string]string{"job_id": job, "revision": revision})
	})
	return job, e
}

// Pause releases a barrier but retains every verified private copy. A running
// worker observes the paused state before each copy and before the switch CAS.
func (s *Service) PauseRoute(ctx context.Context, tenant, id, expected, reason string) error {
	if e := access.ValidReason(reason); e != nil {
		return e
	}
	return s.db(ctx, func(tx pgx.Tx) error {
		var route int64
		if e := tx.QueryRow(ctx, `SELECT route_revision FROM monitoring_tenant_states WHERE tenant_id=$1 FOR UPDATE`, tenant).Scan(&route); e != nil {
			return e
		}
		r, e := relocationTx(ctx, tx, tenant, id)
		if e != nil {
			return e
		}
		if r.Revision != expected {
			return revisionChanged()
		}
		if r.State == "switched" || r.State == "completed" {
			return fmt.Errorf("switched relocation cannot be paused")
		}
		if _, e = tx.Exec(ctx, `UPDATE monitoring_relocations SET state='paused',revision=revision+1 WHERE tenant_id=$1 AND id=$2 AND revision=$3`, tenant, id, expected); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE monitoring_routes SET state='active' WHERE tenant_id=$1 AND logical_shard=$2 AND revision=$3 AND state='barrier'`, tenant, r.Plan.Logical, route); e != nil {
			return e
		}
		return operatorAuditTx(ctx, tx, tenant, "operator.routes.pause", id, reason, r.Plan)
	})
}
func (s *Service) RollbackRoute(ctx context.Context, tenant, id, expected, reason string) (Relocation, error) {
	r, e := s.Relocation(ctx, tenant, id)
	if e != nil {
		return r, e
	}
	if r.Revision != expected || r.State != "switched" {
		return Relocation{}, revisionChanged()
	}
	p, e := s.PlanRoute(ctx, tenant, r.Plan.Logical, r.Plan.Source)
	if e != nil {
		return Relocation{}, e
	}
	if p.Source != r.Plan.Target {
		return Relocation{}, revisionChanged()
	}
	// A reverse relocation copies and verifies all current publications, including
	// those committed after the original switch. There is no pointer-only rollback.
	return s.CreateRelocation(ctx, p, reason)
}
func (s *Service) relocationManifest(ctx context.Context, p RoutePlan, cursor string) ([]selectedChunk, error) {
	out := []selectedChunk{}
	e := s.db(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT c.id::text,pc.attempt_id::text,c.payload_hash,c.row_count,c.batch_id::text FROM monitoring_chunks c JOIN monitoring_publication_chunks pc ON pc.tenant_id=c.tenant_id AND pc.chunk_id=c.id WHERE c.tenant_id=$1 AND c.logical_shard=$2 AND ($3='' OR c.id::text>$3) ORDER BY c.id LIMIT 1000`, p.Tenant, p.Logical, cursor)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v selectedChunk
			if e = rows.Scan(&v.Chunk, &v.Attempt, &v.Hash, &v.Count, &v.Batch); e != nil {
				return e
			}
			out = append(out, v)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		rows.Close()
		return resolveLocationsTx(ctx, tx, p.Tenant, out, mustRevision(p.Revision))
	})
	return out, e
}
func (s *Storage) chunkRows(ctx context.Context, m selectedChunk) ([]rawObservation, error) {
	p, e := s.physical(m.Physical)
	if e != nil {
		return nil, e
	}
	c, e := p.readyReplica(ctx, []selectedChunk{m})
	if e != nil {
		return nil, e
	}
	rows, e := c.Query(ctx, "SELECT "+rowColumns+" FROM "+p.config.Database+".observations WHERE chunk_id=? AND attempt_id=? ORDER BY row_ordinal", uuid.MustParse(m.Chunk), uuid.MustParse(m.Attempt))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []rawObservation{}
	h := sha256.New()
	size := 0
	for rows.Next() {
		r, hash, ordinal, e := scanRaw(rows)
		if e != nil {
			return nil, e
		}
		if ordinal != uint64(len(out)) {
			return nil, fmt.Errorf("chunk ordinals are not complete")
		}
		size += len(encodeJSON(r))
		if size > 64<<20 || len(out) >= 1000000 {
			return nil, fmt.Errorf("chunk exceeds maintenance memory budget")
		}
		out = append(out, r)
		io.WriteString(h, hash)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if uint64(len(out)) != m.Count || hex.EncodeToString(h.Sum(nil)) != m.Hash {
		return nil, fmt.Errorf("chunk payload differs from manifest")
	}
	return out, nil
}
func (s *Service) checkRelocationTx(ctx context.Context, tx pgx.Tx, j Job, r Relocation) error {
	if e := assertJobTx(ctx, tx, j); e != nil {
		return e
	}
	var route int64
	var state string
	if e := tx.QueryRow(ctx, `SELECT route_revision FROM monitoring_tenant_states WHERE tenant_id=$1 FOR UPDATE`, j.tenant).Scan(&route); e != nil {
		return e
	}
	if e := tx.QueryRow(ctx, `SELECT state FROM monitoring_relocations WHERE tenant_id=$1 AND id=$2`, j.tenant, r.ID).Scan(&state); e != nil {
		return e
	}
	if state == "paused" {
		return problem("route_paused", "迁移已暂停。", 409)
	}
	if route != mustRevision(r.Plan.Revision) {
		return problem("route_changed", "路由版本已变化，需要重新预览迁移。", 409)
	}
	return nil
}
func (s *Service) copyRouteChunk(ctx context.Context, j Job, r Relocation, source selectedChunk) (result error) {
	dest := source
	dest.Physical = r.Plan.Target
	dest.Attempt = ""
	e := s.jobTransaction(ctx, j, func(tx pgx.Tx) error {
		if e := s.checkRelocationTx(ctx, tx, j, r); e != nil {
			return e
		}
		e := tx.QueryRow(ctx, `SELECT attempt_id::text FROM monitoring_relocation_copies WHERE tenant_id=$1 AND relocation_id=$2 AND chunk_id=$3`, j.tenant, r.ID, source.Chunk).Scan(&dest.Attempt)
		if e == pgx.ErrNoRows {
			return nil
		}
		return e
	})
	if e != nil {
		return e
	}
	if dest.Attempt != "" {
		return s.storage.sealCheck(ctx, dest)
	}
	data, e := s.storage.chunkRows(ctx, source)
	if e != nil {
		return e
	}
	dest.Attempt = uuid.NewString()
	e = s.jobTransaction(ctx, j, func(tx pgx.Tx) error {
		if e := s.checkRelocationTx(ctx, tx, j, r); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO monitoring_write_attempts(id,tenant_id,chunk_id,fence,state,physical_shard,route_revision) VALUES($1,$2,$3,$4,'writing',$5,$6)`, dest.Attempt, j.tenant, source.Chunk, j.fence, dest.Physical, mustRevision(r.Plan.Revision)+1)
		return e
	})
	if e != nil {
		return e
	}
	defer func() {
		if result != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = s.db(cleanup, func(tx pgx.Tx) error {
				_, e := tx.Exec(cleanup, `UPDATE monitoring_write_attempts SET state='uncertain' WHERE id=$1 AND state='writing'`, dest.Attempt)
				return e
			})
		}
	}()
	e = s.storage.writeAt(ctx, dest.Physical, dest.Chunk, dest.Attempt, data)
	if e == nil {
		e = s.storage.sealCheck(ctx, dest)
	}
	if e != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.db(cleanup, func(tx pgx.Tx) error {
			_, err := tx.Exec(cleanup, `UPDATE monitoring_write_attempts SET state='uncertain' WHERE id=$1 AND state='writing'`, dest.Attempt)
			return err
		})
		return e
	}
	return s.jobTransaction(ctx, j, func(tx pgx.Tx) error {
		if e := s.checkRelocationTx(ctx, tx, j, r); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE monitoring_write_attempts SET state='sealed',verification_hash=$2,replica_epochs=$3,sealed_at=clock_timestamp() WHERE id=$1 AND state='writing' AND fence=$4`, dest.Attempt, dest.Hash, encodeJSON(map[string]string{"verified": "full-payload-v1", "physical": dest.Physical}), j.fence); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO monitoring_relocation_copies(tenant_id,relocation_id,chunk_id,attempt_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, j.tenant, r.ID, dest.Chunk, dest.Attempt)
		return e
	})
}
func (s *Service) relocateJob(ctx context.Context, j Job) error {
	r, e := s.Relocation(ctx, j.tenant, j.Target)
	if e != nil {
		return e
	}
	var input map[string]string
	if e = json.Unmarshal(j.input, &input); e != nil {
		return e
	}
	if input["mode"] != "copy" && input["mode"] != "switch" {
		return fmt.Errorf("invalid relocation phase")
	}
	if r.State == "switched" {
		return nil
	}
	copyAll := func() error {
		cursor := ""
		var count int64
		for {
			page, e := s.relocationManifest(ctx, r.Plan, cursor)
			if e != nil {
				return e
			}
			if len(page) == 0 {
				return nil
			}
			for _, chunk := range page {
				if e = s.copyRouteChunk(ctx, j, r, chunk); e != nil {
					return e
				}
				count++
				if e = s.recordProgress(ctx, j, map[string]string{"phase": "copying", "chunks": strconv.FormatInt(count, 10)}); e != nil {
					return e
				}
			}
			cursor = page[len(page)-1].Chunk
		}
	}
	if e = copyAll(); e != nil {
		return e
	}
	if input["mode"] == "copy" {
		return s.jobTransaction(ctx, j, func(tx pgx.Tx) error {
			if e := s.checkRelocationTx(ctx, tx, j, r); e != nil {
				return e
			}
			_, e := tx.Exec(ctx, `UPDATE monitoring_relocations SET state='verifying',revision=revision+1 WHERE tenant_id=$1 AND id=$2`, j.tenant, r.ID)
			return e
		})
	}
	// The barrier only guards publication for this logical shard. Uploading and
	// validation remain available, and all source routes continue serving reads.
	e = s.jobTransaction(ctx, j, func(tx pgx.Tx) error {
		if e := s.checkRelocationTx(ctx, tx, j, r); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE monitoring_routes SET state='barrier' WHERE tenant_id=$1 AND logical_shard=$2 AND revision=$3`, j.tenant, r.Plan.Logical, r.Plan.Revision); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE monitoring_relocations SET state='barrier',revision=revision+1 WHERE tenant_id=$1 AND id=$2`, j.tenant, r.ID)
		return e
	})
	if e != nil {
		return e
	}
	if e = copyAll(); e != nil {
		return e
	}
	return s.jobTransaction(ctx, j, func(tx pgx.Tx) error {
		if e := s.checkRelocationTx(ctx, tx, j, r); e != nil {
			return e
		}
		var missing bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM monitoring_publication_chunks pc JOIN monitoring_chunks c ON c.tenant_id=pc.tenant_id AND c.id=pc.chunk_id LEFT JOIN monitoring_relocation_copies x ON x.tenant_id=c.tenant_id AND x.chunk_id=c.id AND x.relocation_id=$3 WHERE c.tenant_id=$1 AND c.logical_shard=$2 AND x.chunk_id IS NULL)`, j.tenant, r.Plan.Logical, r.ID).Scan(&missing); e != nil {
			return e
		}
		if missing {
			return fmt.Errorf("relocation tail is incomplete")
		}
		next := mustRevision(r.Plan.Revision) + 1
		if _, e := tx.Exec(ctx, `UPDATE monitoring_routes SET state='retained' WHERE tenant_id=$1 AND revision=$2`, j.tenant, r.Plan.Revision); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `INSERT INTO monitoring_routes(tenant_id,logical_shard,revision,physical_shard,state) SELECT tenant_id,logical_shard,$3,CASE WHEN logical_shard=$4 THEN $5 ELSE physical_shard END,'active' FROM monitoring_routes WHERE tenant_id=$1 AND revision=$2`, j.tenant, r.Plan.Revision, next, r.Plan.Logical, r.Plan.Target); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `INSERT INTO monitoring_chunk_locations(tenant_id,chunk_id,route_revision,attempt_id) SELECT tenant_id,chunk_id,$3,attempt_id FROM monitoring_relocation_copies WHERE tenant_id=$1 AND relocation_id=$2`, j.tenant, r.ID, next); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE monitoring_tenant_states SET route_revision=$2 WHERE tenant_id=$1`, j.tenant, next); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE monitoring_relocations SET state='switched',revision=revision+1 WHERE tenant_id=$1 AND id=$2`, j.tenant, r.ID); e != nil {
			return e
		}
		return operatorAuditTx(ctx, tx, j.tenant, "operator.routes.switched", r.ID, input["reason"], map[string]any{"from": r.Plan.Revision, "to": next, "logical_shard": r.Plan.Logical, "target": r.Plan.Target})
	})
}
