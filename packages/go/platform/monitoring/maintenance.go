package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicecoord"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func integrityUnavailable() error {
	return problem("monitoring_integrity_unavailable", "相关通道正在完整性核对或修复，暂时不可读取或发布。", 503)
}
func (s *Service) operatorTransaction(ctx context.Context, tenant string, fn func(pgx.Tx) error) error {
	return s.db(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `SELECT id FROM engineering_catalog_state WHERE id=1 FOR SHARE`); e != nil {
			return e
		}
		if e := devicecoord.Catalog(ctx, tx, false); e != nil {
			return e
		}
		if e := devicecoord.Tenant(ctx, tx, tenant, true); e != nil {
			return e
		}
		return fn(tx)
	})
}

type SummaryRepairPlan struct {
	Tenant    string `json:"tenant_id"`
	Channel   string `json:"channel_id"`
	Content   string `json:"content_revision"`
	Revoke    string `json:"revoke_revision"`
	Integrity string `json:"integrity_revision"`
	Watermark string `json:"watermark"`
	Route     string `json:"route_revision"`
	Hash      string `json:"plan_hash"`
}

func repairPlanTx(ctx context.Context, tx pgx.Tx, tenant, channel string) (SummaryRepairPlan, error) {
	p := SummaryRepairPlan{Tenant: tenant, Channel: channel}
	e := tx.QueryRow(ctx, `SELECT v.content_revision::text,v.revoke_revision::text,v.integrity_revision::text,s.commit_sequence::text,s.route_revision::text FROM monitoring_channel_versions v JOIN monitoring_tenant_states s USING(tenant_id) WHERE v.tenant_id=$1 AND v.channel_id=$2`, tenant, channel).Scan(&p.Content, &p.Revoke, &p.Integrity, &p.Watermark, &p.Route)
	if e == nil {
		p.Hash = digest(p)
	}
	return p, e
}
func (s *Service) PlanSummaryRepair(ctx context.Context, tenant, channel string) (SummaryRepairPlan, error) {
	var out SummaryRepairPlan
	if !access.ValidID(tenant) || !access.ValidID(channel) {
		return out, fmt.Errorf("tenant and channel are required")
	}
	e := s.db(ctx, func(tx pgx.Tx) error { var e error; out, e = repairPlanTx(ctx, tx, tenant, channel); return e })
	return out, e
}
func (s *Service) QueueSummaryRepair(ctx context.Context, p SummaryRepairPlan, reason string) (string, error) {
	if e := s.requireStorage(); e != nil {
		return "", e
	}
	if e := access.ValidReason(reason); e != nil {
		return "", e
	}
	if p.Content == "0" {
		return "", fmt.Errorf("empty channel has no summary to repair")
	}
	var job string
	e := s.operatorTransaction(ctx, p.Tenant, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SELECT channel_id FROM monitoring_channel_versions WHERE tenant_id=$1 AND channel_id=$2 FOR UPDATE`, p.Tenant, p.Channel); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `SELECT tenant_id FROM monitoring_tenant_states WHERE tenant_id=$1 FOR UPDATE`, p.Tenant); e != nil {
			return e
		}
		current, e := repairPlanTx(ctx, tx, p.Tenant, p.Channel)
		if e != nil {
			return e
		}
		if current.Hash != p.Hash {
			return revisionChanged()
		}
		var busy bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM monitoring_jobs WHERE tenant_id=$1 AND kind='summary' AND target_id=$2 AND input->>'mode'='rebuild' AND state IN ('queued','running','retry_wait'))`, p.Tenant, p.Channel).Scan(&busy); e != nil {
			return e
		}
		if busy {
			return fmt.Errorf("a summary repair is already active")
		}
		if _, e = tx.Exec(ctx, `UPDATE monitoring_channel_versions SET integrity_revision=integrity_revision+1,quarantined=true WHERE tenant_id=$1 AND channel_id=$2`, p.Tenant, p.Channel); e != nil {
			return e
		}
		input := summaryJobInput{Mode: "rebuild", Repair: &p, Reason: reason}
		if e = tx.QueryRow(ctx, `INSERT INTO monitoring_jobs(tenant_id,kind,target_id,target_revision,input_hash,input) VALUES($1,'summary',$2,$3,$4,$5) RETURNING id::text`, p.Tenant, p.Channel, p.Content, digest(input), encodeJSON(input)).Scan(&job); e != nil {
			return e
		}
		return operatorAuditTx(ctx, tx, p.Tenant, "operator.summaries.rebuild", p.Channel, reason, map[string]any{"plan": p, "job_id": job})
	})
	return job, e
}
func (s *Service) maintenancePage(ctx context.Context, p SummaryRepairPlan, cursor string) ([]selectedChunk, error) {
	out := []selectedChunk{}
	e := s.db(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT c.id::text,pc.attempt_id::text,c.payload_hash,c.row_count,pc.first_adopted_count,c.batch_id::text,p.sequence,(p.revoke_sequence IS NOT NULL AND p.revoke_sequence<=$3),pc.quality_counts FROM monitoring_chunks c JOIN monitoring_publication_chunks pc ON pc.tenant_id=c.tenant_id AND pc.chunk_id=c.id JOIN monitoring_publications p ON p.tenant_id=pc.tenant_id AND p.id=pc.publication_id JOIN monitoring_import_batches b ON b.tenant_id=p.tenant_id AND b.id=p.batch_id WHERE c.tenant_id=$1 AND b.semantic_snapshot->'actual_coverage' ? $2 AND p.sequence<=$3 AND ($4='' OR c.id::text>$4) ORDER BY c.id LIMIT 1000`, p.Tenant, p.Channel, p.Watermark, cursor)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v selectedChunk
			var q []byte
			if e = rows.Scan(&v.Chunk, &v.Attempt, &v.Hash, &v.Count, &v.First, &v.Batch, &v.Sequence, &v.Revoked, &q); e != nil {
				return e
			}
			if e = json.Unmarshal(q, &v.Qualities); e != nil {
				return e
			}
			out = append(out, v)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		rows.Close()
		return resolveLocationsTx(ctx, tx, p.Tenant, out, mustRevision(p.Route))
	})
	return out, e
}
func contributionBlocks(channel, chunk string, active bool, contribution chunkContributions) (map[int64]map[string]leafContribution, error) {
	blocks := map[int64]map[string]leafContribution{}
	for block, minutes := range contribution[channel] {
		id, e := strconv.ParseInt(block, 10, 64)
		if e != nil || id < 0 {
			return nil, fmt.Errorf("invalid contribution block")
		}
		blocks[id] = map[string]leafContribution{chunk: {Active: active, Minutes: minutes}}
	}
	return blocks, nil
}
func (s *Service) rebuildSummaryJob(ctx context.Context, j Job, in summaryJobInput) error {
	if in.Repair == nil || j.actor != "" || in.Repair.Tenant != j.tenant || in.Repair.Channel != j.Target {
		return fmt.Errorf("invalid summary repair")
	}
	p := *in.Repair
	var installed bool
	e := s.db(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM monitoring_summary_replacements WHERE tenant_id=$1 AND channel_id=$2 AND job_id=$3)`, j.tenant, j.Target, j.ID).Scan(&installed)
	})
	if e != nil || installed {
		return e
	}
	if e = s.verifyChannelSources(ctx, p.Tenant, p.Channel); e != nil {
		return e
	}
	identity, e := newIdentityVerifier(s.storage.config.TempDirectory)
	if e != nil {
		return e
	}
	defer identity.close()
	// A fresh namespace avoids silently reusing a damaged content-addressed
	// object after a 412 response. Every old root and source remains retained.
	isolated := *s.storage
	isolated.summaryNamespace = "repairs/" + j.ID + "/" + strconv.FormatInt(j.fence, 10) + "/"
	builder := treeBuilder{storage: &isolated, tenant: p.Tenant, nodes: map[string]*summaryNode{}}
	root := summaryRoot{Version: 1, ContentRevision: p.Content}
	cursor := ""
	var copied int64
	for {
		page, e := s.maintenancePage(ctx, p, cursor)
		if e != nil {
			return e
		}
		if len(page) == 0 {
			break
		}
		for _, chunk := range page {
			if e = s.jobTransaction(ctx, j, func(tx pgx.Tx) error { return assertJobTx(ctx, tx, j) }); e != nil {
				return e
			}
			if e = s.storage.sealCheck(ctx, chunk); e != nil {
				return e
			}
			raw, e := s.storage.chunkRows(ctx, chunk)
			if e != nil {
				return e
			}
			for _, row := range raw {
				if row.Channel == p.Channel {
					if e = identity.add(row, !chunk.Revoked); e != nil {
						return e
					}
				}
			}
			contribution := makeContributions(raw)
			key, e := isolated.putSummary(ctx, p.Tenant, contribution)
			if e != nil {
				return e
			}
			blocks, e := contributionBlocks(p.Channel, chunk.Chunk, !chunk.Revoked, contribution)
			if e != nil {
				return e
			}
			if len(blocks) > 0 {
				root.Root, e = builder.update(ctx, root.Root, 3, blocks)
				if e != nil {
					return e
				}
			}
			builder.nodes = map[string]*summaryNode{}
			e = s.jobTransaction(ctx, j, func(tx pgx.Tx) error {
				if e := assertJobTx(ctx, tx, j); e != nil {
					return e
				}
				_, e := tx.Exec(ctx, `INSERT INTO monitoring_job_steps(tenant_id,job_id,step_key,fence,state,result) VALUES($1,$2,$3,$4,'completed',$5) ON CONFLICT(tenant_id,job_id,step_key) DO UPDATE SET fence=EXCLUDED.fence,state='completed',result=EXCLUDED.result,updated_at=clock_timestamp()`, p.Tenant, j.ID, "contribution:"+chunk.Chunk, j.fence, encodeJSON(map[string]string{"chunk": chunk.Chunk, "object": key}))
				return e
			})
			if e != nil {
				return e
			}
			copied++
			if e = s.recordProgress(ctx, j, map[string]string{"phase": "rebuilding", "chunks": strconv.FormatInt(copied, 10)}); e != nil {
				return e
			}
		}
		cursor = page[len(page)-1].Chunk
	}
	if e = identity.requireValid(ctx); e != nil {
		return e
	}
	key := p.Tenant + "/summaries/v1/" + isolated.summaryNamespace + "root.json"
	raw := encodeJSON(root)
	if e = isolated.put(ctx, key, bytes.NewReader(raw), int64(len(raw)), "application/json"); e != nil {
		return e
	}
	var check summaryRoot
	if e = s.storage.summaryObject(ctx, key, &check, false); e != nil {
		return e
	}
	if digest(check) != digest(root) {
		return fmt.Errorf("rebuilt root readback mismatch")
	}
	return s.jobTransaction(ctx, j, func(tx pgx.Tx) error {
		if e := assertJobTx(ctx, tx, j); e != nil {
			return e
		}
		var content, revoke, integrity string
		var quarantined bool
		if e := tx.QueryRow(ctx, `SELECT content_revision::text,revoke_revision::text,integrity_revision::text,quarantined FROM monitoring_channel_versions WHERE tenant_id=$1 AND channel_id=$2 FOR UPDATE`, p.Tenant, p.Channel).Scan(&content, &revoke, &integrity, &quarantined); e != nil {
			return e
		}
		if content != p.Content || revoke != p.Revoke || mustRevision(integrity) != mustRevision(p.Integrity)+1 || !quarantined {
			return revisionChanged()
		}
		var route string
		if e := tx.QueryRow(ctx, `SELECT route_revision::text FROM monitoring_tenant_states WHERE tenant_id=$1 FOR UPDATE`, p.Tenant).Scan(&route); e != nil {
			return e
		}
		if route != p.Route {
			return revisionChanged()
		}
		if _, e := tx.Exec(ctx, `INSERT INTO monitoring_summary_replacements(tenant_id,channel_id,content_revision,integrity_revision,root_object,job_id) VALUES($1,$2,$3,$4,$5,$6)`, p.Tenant, p.Channel, p.Content, integrity, key, j.ID); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `INSERT INTO monitoring_contribution_replacements(tenant_id,chunk_id,job_id,object_key) SELECT tenant_id,(result->>'chunk')::uuid,job_id,result->>'object' FROM monitoring_job_steps WHERE tenant_id=$1 AND job_id=$2 AND fence=$3 AND step_key LIKE 'contribution:%' AND state='completed'`, p.Tenant, j.ID, j.fence); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `INSERT INTO monitoring_summary_heads(tenant_id,channel_id,level,revision,watermark,revoke_revision,root_object) VALUES($1,$2,'tree-v1',$3,$4,$5,$6) ON CONFLICT(tenant_id,channel_id,level) DO UPDATE SET revision=EXCLUDED.revision,watermark=EXCLUDED.watermark,revoke_revision=EXCLUDED.revoke_revision,root_object=EXCLUDED.root_object`, p.Tenant, p.Channel, p.Content, p.Watermark, p.Revoke, root.Root.Object); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `DELETE FROM monitoring_dirty_ranges WHERE tenant_id=$1 AND channel_id=$2 AND target_sequence<=$3`, p.Tenant, p.Channel, p.Watermark); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE monitoring_channel_versions SET quarantined=false WHERE tenant_id=$1 AND channel_id=$2`, p.Tenant, p.Channel); e != nil {
			return e
		}
		return operatorAuditTx(ctx, tx, p.Tenant, "operator.summaries.rebuilt", p.Channel, in.Reason, map[string]string{"job_id": j.ID, "content_revision": p.Content, "integrity_revision": integrity, "watermark": p.Watermark})
	})
}

// InspectOperatorJob intentionally includes failed system jobs, without input
// bodies, object paths, connection strings or borrowed user authorization.
func (s *Service) InspectOperatorJob(ctx context.Context, tenant, id string) (Job, error) {
	var out Job
	e := s.db(ctx, func(tx pgx.Tx) error {
		var e error
		out, e = scanJob(tx.QueryRow(ctx, jobSelect+` WHERE tenant_id=$1 AND id=$2`, tenant, id))
		return e
	})
	return out, e
}
func (s *Service) RetryOperatorJob(ctx context.Context, tenant, id, expected, reason string) error {
	if e := access.ValidReason(reason); e != nil {
		return e
	}
	return s.operatorTransaction(ctx, tenant, func(tx pgx.Tx) error {
		j, e := scanJob(tx.QueryRow(ctx, jobSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenant, id))
		if e != nil {
			return e
		}
		if j.Revision != expected || j.State == "running" || j.State == "succeeded" || j.State == "queued" {
			return revisionChanged()
		}
		// Original user operations can resume only while their exact target is
		// still pending. A terminal import needs full revalidation in its workspace.
		if j.Kind == "validate" || j.Kind == "publish" {
			var valid bool
			if e = tx.QueryRow(ctx, `SELECT revision=$3 AND state IN ('validating','preparing') FROM monitoring_import_batches WHERE tenant_id=$1 AND id=$2`, tenant, j.Target, j.TargetRevision).Scan(&valid); e != nil {
				return e
			}
			if !valid {
				return fmt.Errorf("import target changed; revalidate it in the original workspace")
			}
		}
		// User operations still revalidate the original actor in the worker.
		// A manual restart closes the failed attempt's retry-wait execution before
		// allocating a new generation. Automatic retries keep that generation open.
		if j.State == "retry_wait" {
			if _, e = tx.Exec(ctx, `UPDATE monitoring_jobs SET state='failed',lease_owner=NULL,lease_until=NULL,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1`, id); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `UPDATE monitoring_jobs SET state='queued',notification_generation=notification_generation+1,attempts=0,cancel_requested=false,lease_owner=NULL,lease_until=NULL,error_code=NULL,available_at=clock_timestamp(),revision=revision+1,updated_at=clock_timestamp() WHERE id=$1`, id); e != nil {
			return e
		}
		return operatorAuditTx(ctx, tx, tenant, "operator.jobs.retry", id, reason, map[string]string{"revision": expected, "request_id": uuid.NewString()})
	})
}
