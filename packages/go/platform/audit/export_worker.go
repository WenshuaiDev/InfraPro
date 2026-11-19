package audit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// A durable delegation rechecks the account and exact original membership on
// each boundary. It intentionally does not depend on the creator's session.
func delegated(owner string) access.Actor {
	return access.Actor{ID: owner, Check: func(ctx context.Context, tx pgx.Tx, _ bool) error {
		var active bool
		e := tx.QueryRow(ctx, `SELECT status='active' FROM accounts WHERE id=$1`, owner).Scan(&active)
		if e != nil {
			return e
		}
		if !active {
			return access.ErrDenied
		}
		return nil
	}}
}
func (s *Service) exportAuthorizationTx(ctx context.Context, tx pgx.Tx, v exportRecord) (authorization, error) {
	a, e := s.authorizeTx(ctx, tx, delegated(v.Owner), v.Scope)
	if e != nil {
		return a, Problem("audit_export_invalidated", 409)
	}
	if a.Mode != "ready" || !a.Has("export") || v.FieldProfile == "technical" && !a.Has("technical.read") || a.Vector != v.Vector || a.Epoch != v.Epoch || a.Projection != v.Projection || a.MembershipKey != v.Membership {
		return a, Problem("audit_export_invalidated", 409)
	}
	return a, nil
}
func (s *Service) claimExport(ctx context.Context, target string, targetGeneration int64) (exportRecord, bool, error) {
	v := exportRecord{}
	claimed := false
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var epoch, mode string
		if e := tx.QueryRow(ctx, `SELECT restore_epoch::text,mode FROM audit_protocol WHERE id=1`).Scan(&epoch, &mode); e != nil {
			return e
		}
		if mode != "ready" {
			return nil
		}
		_, e := tx.Exec(ctx, `INSERT INTO audit_worker_states(id,heartbeat_at,epoch) VALUES($1,clock_timestamp(),$2) ON CONFLICT(id) DO UPDATE SET heartbeat_at=EXCLUDED.heartbeat_at,epoch=EXCLUDED.epoch,error_code=''`, s.options.NodeID, epoch)
		if e != nil {
			return e
		}
		// All workers serialize only this short claim, not snapshot or object IO.
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(834327120)`); e != nil {
			return e
		}
		var active int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM audit_export_generations WHERE state='running' AND lease_until>clock_timestamp()`).Scan(&active); e != nil {
			return e
		}
		if active >= s.options.Workers {
			return nil
		}
		var id, generation string
		var targetID any
		if target != "" {
			targetID = target
		}
		e = tx.QueryRow(ctx, `SELECT g.export_id::text,g.generation::text FROM audit_export_generations g JOIN audit_exports x ON x.id=g.export_id AND x.current_generation=g.generation WHERE ($1::uuid IS NULL OR (g.export_id=$1 AND g.generation=$2)) AND (g.state='queued' OR (g.state='retry_wait' AND g.next_attempt_at<=clock_timestamp()) OR (g.state IN ('running','cancelling') AND g.lease_until<clock_timestamp())) ORDER BY g.created_at,g.export_id FOR UPDATE OF x,g SKIP LOCKED LIMIT 1`, targetID, targetGeneration).Scan(&id, &generation)
		if e == pgx.ErrNoRows {
			return nil
		}
		if e != nil {
			return e
		}
		v, e = loadExportTx(ctx, tx, id, generation, true)
		if e != nil {
			return e
		}
		if v.State == "cancelling" {
			return s.terminalExportTx(ctx, tx, v, "cancelled", "", "")
		}
		if v.Attempt >= s.options.MaxAttempts {
			return s.terminalExportTx(ctx, tx, v, "failed", "audit_attempts_exhausted", "")
		}
		if _, e = s.exportAuthorizationTx(ctx, tx, v); e != nil {
			return s.terminalExportTx(ctx, tx, v, "failed", "audit_export_invalidated", "")
		}
		_, e = tx.Exec(ctx, `UPDATE audit_export_generations SET state='running',phase=CASE WHEN snapshot_at IS NULL THEN 'preparing' ELSE 'writing' END,fence=fence+1,attempt=attempt+1,lease_owner=$3,lease_until=clock_timestamp()+$4*interval '1 second',error_code='' WHERE export_id=$1 AND generation=$2`, id, generation, s.options.NodeID, s.options.Lease.Seconds())
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE audit_exports SET source_revision=source_revision+1 WHERE id=$1`, id); e != nil {
			return e
		}
		v, e = loadExportTx(ctx, tx, id, generation, false)
		if e != nil {
			return e
		}
		claimed = true
		return s.indexExportTx(ctx, tx, v, "", "started")
	})
	return v, claimed, e
}
func ownedRun(v, expected exportRecord, now time.Time) bool {
	return v.State == "running" && v.Fence == expected.Fence && v.LeaseOwner == expected.LeaseOwner && v.Epoch == expected.Epoch && v.LeaseUntil != nil && v.LeaseUntil.After(now)
}
func (s *Service) snapshotExport(ctx context.Context, expected exportRecord) error {
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, s.options.SnapshotTimeout)
	defer cancel()
	defer diagnosticreport.TrackDeadline(parent, ctx)()
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ`); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `SELECT set_config('statement_timeout',$1,true)`, strconv.FormatInt(min(s.options.SnapshotTimeout.Milliseconds(), 10000), 10)); e != nil {
			return e
		}
		v, e := loadExportTx(ctx, tx, expected.ID, expected.Generation, true)
		if e != nil {
			return e
		}
		if !ownedRun(v, expected, s.now()) {
			return Problem("audit_export_superseded", 409)
		}
		auth, e := s.exportAuthorizationTx(ctx, tx, v)
		if e != nil {
			return e
		}
		if v.SnapshotAt != nil {
			return nil
		}
		// Every candidate, role, label and domain disclosure is observed through
		// this one PostgreSQL snapshot, before any external object write.
		var at time.Time
		var upper string
		if e = tx.QueryRow(ctx, `SELECT transaction_timestamp(),COALESCE(max(id),0)::text FROM account_security_events`).Scan(&at, &upper); e != nil {
			return e
		}
		position := cursor{Upper: at}
		count := 0
		var size int64
		digest := sha256.New()
		for {
			batch, e := s.candidatesTx(ctx, tx, auth, *v.Filter, position, 1000, "")
			if e != nil {
				return e
			}
			if len(batch) == 0 {
				break
			}
			materialized := make([][]any, 0, len(batch))
			for offset := 0; offset < len(batch); offset += 100 {
				projected, e := s.projectTx(ctx, tx, delegated(v.Owner), auth, batch[offset:min(offset+100, len(batch))])
				if e != nil {
					return e
				}
				for _, event := range projected {
					if event == nil {
						continue
					}
					match, e := matchesTx(ctx, tx, *v.Filter, *event)
					if e != nil {
						return e
					}
					if !match {
						continue
					}
					if v.FieldProfile == "standard" {
						event.Technical = nil
					}
					raw := ar.JSON(event)
					count++
					size += int64(len(raw))
					if count > s.options.ExportMaxRows || size > s.options.SnapshotMaxBytes {
						return Problem("audit_export_too_large", 422)
					}
					digest.Write(raw)
					digest.Write([]byte{'\n'})
					materialized = append(materialized, []any{v.ID, v.Generation, count, event.ID, raw})
				}
			}
			if len(materialized) > 0 {
				if _, e = tx.CopyFrom(ctx, pgx.Identifier{"audit_export_rows"}, []string{"export_id", "generation", "ordinal", "event_id", "projection"}, pgx.CopyFromRows(materialized)); e != nil {
					return e
				}
			}
			last := batch[len(batch)-1]
			position.At = last.Recorded
			position.ID = last.ID
			if len(batch) < 1000 {
				break
			}
		}
		_, e = tx.Exec(ctx, `UPDATE audit_export_generations SET snapshot_at=$3,snapshot_upper_id=$4,snapshot_rows=$5,snapshot_bytes=$6,snapshot_digest=$7,phase='writing' WHERE export_id=$1 AND generation=$2`, v.ID, v.Generation, at, upper, count, size, hex.EncodeToString(digest.Sum(nil)))
		return e
	})
}

var standardCSV = []string{"event_id", "occurred_at", "recorded_at", "scope_kind", "tenant_id", "project_id", "event_type", "action_label", "phase", "outcome", "actor_name", "actor_name_source", "object_type", "object_id", "object_label", "reason", "changes", "evidence_quality", "missing_fields"}
var technicalCSV = []string{"operation_id", "request_id", "error_code", "task_reference", "source_ip", "user_agent"}

func csvCell(v string) string {
	trim := strings.TrimLeftFunc(v, unicode.IsSpace)
	if len(trim) > 0 && strings.ContainsRune("=+-@", rune(trim[0])) || strings.HasPrefix(v, "\t") || strings.HasPrefix(v, "\r") {
		return "'" + v
	}
	return v
}
func csvRecord(e Event, technical bool) []string {
	v := []string{e.ID, e.OccurredAt.UTC().Format(time.RFC3339Nano), e.RecordedAt.UTC().Format(time.RFC3339Nano), e.Scope.Kind, e.Scope.Tenant, e.Scope.Project, e.EventType, e.ActionLabel, e.Phase, e.Outcome, e.Actor.Name, e.Actor.NameSource, e.Object.Type, e.Object.ID, e.Object.Label, e.Reason, string(ar.JSON(e.Changes)), e.EvidenceQuality, strings.Join(e.MissingFields, ",")}
	if technical {
		t := Technical{}
		if e.Technical != nil {
			t = *e.Technical
		}
		v = append(v, t.OperationID, t.RequestID, t.ErrorCode, string(ar.JSON(t.TaskReferences)), t.IP, t.UserAgent)
	}
	for i := range v {
		v[i] = csvCell(v[i])
	}
	return v
}

type boundedWriter struct {
	w      io.Writer
	n, max int64
}

func (w *boundedWriter) Write(b []byte) (int, error) {
	if w.n+int64(len(b)) > w.max {
		return 0, Problem("audit_export_too_large", 422)
	}
	n, e := w.w.Write(b)
	w.n += int64(n)
	return n, e
}
func (s *Service) exportCSV(ctx context.Context, v exportRecord) (string, int64, string, error) {
	if e := os.MkdirAll(s.options.TempDirectory, 0700); e != nil {
		return "", 0, "", e
	}
	var occupied int64
	entries, e := os.ReadDir(s.options.TempDirectory)
	if e != nil {
		return "", 0, "", e
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			info, e := entry.Info()
			if e != nil {
				return "", 0, "", e
			}
			occupied += info.Size()
		}
	}
	if occupied+s.options.ExportMaxBytes > s.options.TempBudget {
		return "", 0, "", Problem("audit_temp_budget_exceeded", 503)
	}
	f, e := os.CreateTemp(s.options.TempDirectory, "audit-*.csv")
	if e != nil {
		return "", 0, "", e
	}
	path := f.Name()
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(path)
		}
	}()
	h := sha256.New()
	w := &boundedWriter{w: io.MultiWriter(f, h), max: s.options.ExportMaxBytes}
	if _, e = w.Write([]byte{0xef, 0xbb, 0xbf}); e != nil {
		return "", 0, "", e
	}
	csvw := csv.NewWriter(w)
	csvw.UseCRLF = true
	header := append([]string{}, standardCSV...)
	if v.FieldProfile == "technical" {
		header = append(header, technicalCSV...)
	}
	if e = csvw.Write(header); e != nil {
		return "", 0, "", e
	}
	var ordinal int64
	for {
		batch := []Event{}
		e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			rows, e := tx.Query(ctx, `SELECT ordinal,projection FROM audit_export_rows WHERE export_id=$1 AND generation=$2 AND ordinal>$3 ORDER BY ordinal LIMIT 100`, v.ID, v.Generation, ordinal)
			if e != nil {
				return e
			}
			defer rows.Close()
			for rows.Next() {
				var event Event
				if e = rows.Scan(&ordinal, &event); e != nil {
					return e
				}
				batch = append(batch, event)
			}
			return rows.Err()
		})
		if e != nil {
			return "", 0, "", e
		}
		for _, event := range batch {
			if e = csvw.Write(csvRecord(event, v.FieldProfile == "technical")); e != nil {
				return "", 0, "", e
			}
		}
		if len(batch) < 100 {
			break
		}
	}
	csvw.Flush()
	if e = csvw.Error(); e != nil {
		return "", 0, "", e
	}
	if e = f.Sync(); e != nil {
		return "", 0, "", e
	}
	if e = f.Close(); e != nil {
		return "", 0, "", e
	}
	ok = true
	return filepath.Clean(path), w.n, hex.EncodeToString(h.Sum(nil)), nil
}
func (s *Service) writeExport(ctx context.Context, expected exportRecord) error {
	var v exportRecord
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var e error
		v, e = loadExportTx(ctx, tx, expected.ID, expected.Generation, false)
		return e
	})
	if e != nil {
		return e
	}
	release, e := s.reserveTemp(ctx, s.options.ExportMaxBytes)
	if e != nil {
		return e
	}
	defer release()
	path, size, digest, e := s.exportCSV(ctx, v)
	if e != nil {
		return e
	}
	defer os.Remove(path)
	id := uuid.NewString()
	key := fmt.Sprintf("audit-exports/v1/%s/%s/%s/%d/%s.csv", v.Epoch, v.ID, v.Generation, v.Fence, id)
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		current, e := loadExportTx(ctx, tx, v.ID, v.Generation, true)
		if e != nil {
			return e
		}
		if !ownedRun(current, expected, s.now()) {
			return Problem("audit_export_superseded", 409)
		}
		if _, e = s.exportAuthorizationTx(ctx, tx, current); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO audit_export_objects(id,export_id,generation,object_key,fence,restore_epoch,state,size_bytes,sha256) VALUES($1,$2,$3,$4,$5,$6,'intent',$7,$8)`, id, v.ID, v.Generation, key, v.Fence, v.Epoch, size, digest)
		return e
	})
	if e != nil {
		return e
	}
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	e = s.objects.Put(ctx, key, f, size, "text/csv; charset=utf-8")
	f.Close()
	if e != nil {
		return Problem("audit_storage_unavailable", 503)
	}
	object, e := s.objects.Get(ctx, key)
	if e != nil {
		return Problem("audit_storage_unavailable", 503)
	}
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(object, size+1))
	object.Close()
	if e != nil || n != size || hex.EncodeToString(h.Sum(nil)) != digest {
		return Problem("audit_export_integrity", 422)
	}
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		current, e := loadExportTx(ctx, tx, v.ID, v.Generation, true)
		if e != nil {
			return e
		}
		if !ownedRun(current, expected, s.now()) {
			return Problem("audit_export_superseded", 409)
		}
		if _, e = s.exportAuthorizationTx(ctx, tx, current); e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, `UPDATE audit_export_objects SET state='available',expires_at=clock_timestamp()+$2*interval '1 second' WHERE id=$1 AND state='intent'`, id, s.options.ResultTTL.Seconds())
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return Problem("audit_export_superseded", 409)
		}
		return s.terminalExportTx(ctx, tx, current, "succeeded", "", id)
	})
}
func (s *Service) terminalExportTx(ctx context.Context, tx pgx.Tx, v exportRecord, state, code, object string) error {
	if taskrecord.Terminal(v.State) {
		return nil
	}
	_, e := tx.Exec(ctx, `UPDATE audit_export_generations SET state=$3,phase=$3,error_code=$4,object_id=NULLIF($5,'')::uuid,finished_at=clock_timestamp(),lease_until=NULL WHERE export_id=$1 AND generation=$2`, v.ID, v.Generation, state, code, object)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE audit_exports SET source_revision=source_revision+1 WHERE id=$1`, v.ID); e != nil {
		return e
	}
	v, e = loadExportTx(ctx, tx, v.ID, v.Generation, false)
	if e != nil {
		return e
	}
	if e = s.exportFactTx(ctx, tx, v, "audit.export."+state, "", "worker", ""); e != nil {
		return e
	}
	ref := taskrecord.Ref{SourceRef: exportSource(v.ID), Generation: v.Generation}
	recipients := ne.Personal(v.Owner)
	if v.Scope.Kind != "platform" {
		recipients = ne.Scoped(v.Owner)
		parts := strings.Split(v.Membership, ":")
		recipients[0].TenantMembership = parts[0]
		if v.Scope.Kind == "project" && len(parts) > 1 {
			recipients[0].ProjectMembership = parts[1]
		}
	}
	_, e = ne.AppendTx(ctx, tx, ne.Event{Module: "audit", Key: "export:" + v.ID + ":generation:" + v.Generation + ":terminal", Type: "task.result", Scope: ne.Scope{Kind: v.Scope.Kind, Tenant: v.Scope.Tenant, Project: v.Scope.Project}, Target: ne.Target{Kind: "audit_export", ID: v.ID}, Outcome: state, Reason: code, Recipients: recipients, TaskReference: &ref})
	if e != nil {
		return e
	}
	return s.indexExportTx(ctx, tx, v, "", state)
}
func (s *Service) heartbeatExport(ctx context.Context, v exportRecord) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, `UPDATE audit_export_generations SET lease_until=clock_timestamp()+$5*interval '1 second' WHERE export_id=$1 AND generation=$2 AND fence=$3 AND lease_owner=$4 AND state='running' AND lease_until>clock_timestamp() AND restore_epoch=(SELECT restore_epoch FROM audit_protocol WHERE id=1 AND mode='ready')`, v.ID, v.Generation, v.Fence, v.LeaseOwner, s.options.Lease.Seconds())
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return Problem("audit_export_superseded", 409)
		}
		return nil
	})
}
func (s *Service) failExport(ctx context.Context, expected exportRecord, cause error) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		v, e := loadExportTx(ctx, tx, expected.ID, expected.Generation, true)
		if e != nil {
			return e
		}
		if v.Fence != expected.Fence || taskrecord.Terminal(v.State) {
			return nil
		}
		if v.State == "cancelling" {
			return s.terminalExportTx(ctx, tx, v, "cancelled", "", "")
		}
		code := "audit_source_unavailable"
		retry := false
		var problem *Error
		if errors.As(cause, &problem) {
			code = problem.Code
			retry = problem.Status == 503 && code != "audit_protocol_unavailable"
		} else if errors.Is(cause, context.DeadlineExceeded) {
			code = "audit_snapshot_timeout"
		} else {
			retry = true
		}
		if _, autherr := s.exportAuthorizationTx(ctx, tx, v); autherr != nil {
			code = "audit_export_invalidated"
			retry = false
		}
		if retry && v.Attempt < s.options.MaxAttempts {
			_, e = tx.Exec(ctx, `UPDATE audit_export_generations SET state='retry_wait',phase='waiting',error_code=$3,lease_until=NULL,next_attempt_at=clock_timestamp()+$4*interval '1 second' WHERE export_id=$1 AND generation=$2`, v.ID, v.Generation, code, min(60, 1<<(v.Attempt-1)))
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `UPDATE audit_exports SET source_revision=source_revision+1 WHERE id=$1`, v.ID); e != nil {
				return e
			}
			v, e = loadExportTx(ctx, tx, v.ID, v.Generation, false)
			if e != nil {
				return e
			}
			return s.indexExportTx(ctx, tx, v, "", "retry_wait")
		}
		return s.terminalExportTx(ctx, tx, v, "failed", code, "")
	})
}

// RunExportOnce owns one bounded claim. A crash after Put leaves a registered
// intent; another fenced attempt never overwrites its key.
func (s *Service) RunExportOnce(ctx context.Context) (bool, error) { return s.runExport(ctx, "", 0) }

// RunExportGeneration can only attempt the generation declared in the fact.
// A competing claim or retry window leaves the same durable generation pending.
func (s *Service) RunExportGeneration(ctx context.Context, id string, generation int64, scope string) (pending bool, err error) {
	if !access.ValidID(id) || generation < 1 {
		return false, pgx.ErrNoRows
	}
	err = s.access.Transact(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT g.state IN ('queued','running','cancelling','retry_wait') FROM audit_exports e JOIN audit_export_generations g ON g.export_id=e.id WHERE e.id=$1 AND g.generation=$2 AND COALESCE(e.tenant_id::text,'platform')=$3`, id, generation, scope).Scan(&pending)
	})
	if err != nil || !pending {
		return false, err
	}
	if s.objects == nil {
		return true, nil
	}
	if _, err = s.runExport(ctx, id, generation); err != nil {
		return false, err
	}
	err = s.access.Transact(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT state IN ('queued','running','cancelling','retry_wait') FROM audit_export_generations WHERE export_id=$1 AND generation=$2`, id, generation).Scan(&pending)
	})
	return pending, err
}

func (s *Service) runExport(ctx context.Context, id string, generation int64) (bool, error) {
	if s.objects == nil {
		return false, Problem("audit_storage_unavailable", 503)
	}
	v, ok, e := s.claimExport(ctx, id, generation)
	if e != nil || !ok {
		return ok, e
	}
	run, cancel := context.WithTimeout(ctx, s.options.ReadTimeout)
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(s.options.Heartbeat)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-run.Done():
				return
			case <-tick.C:
				if s.heartbeatExport(run, v) != nil {
					cancel()
					return
				}
			}
		}
	}()
	e = s.snapshotExport(run, v)
	if e == nil {
		e = s.writeExport(run, v)
	}
	close(done)
	cancel()
	wg.Wait()
	if e != nil {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.options.FailedFactTimeout)
		defer cancel()
		return true, s.failExport(recovery, v, e)
	}
	return true, nil
}

// VerifyCSV is shared by content verification and restore checks.
func VerifyCSV(raw []byte, size int64, hash string) bool {
	sum := sha256.Sum256(raw)
	if int64(len(raw)) != size || hex.EncodeToString(sum[:]) != hash {
		return false
	}
	r := csv.NewReader(bytes.NewReader(raw))
	_, e := r.ReadAll()
	return e == nil
}
