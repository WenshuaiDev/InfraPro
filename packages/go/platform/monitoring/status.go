package monitoring

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
)

type QueueMetric struct {
	Kind          string `json:"kind"`
	State         string `json:"state"`
	Count         string `json:"count"`
	OldestSeconds string `json:"oldest_seconds"`
}
type ReplicaMetric struct {
	Physical     string            `json:"physical_shard"`
	Replica      int               `json:"replica"`
	State        string            `json:"state"`
	Parts        string            `json:"parts"`
	PhysicalRows string            `json:"physical_rows"`
	DiskBytes    string            `json:"disk_bytes"`
	Queue        string            `json:"replication_queue"`
	Delay        string            `json:"replication_delay_seconds"`
	Merges       string            `json:"active_merges"`
	Queries      string            `json:"queries_last_hour"`
	Latency      []float64         `json:"query_ms_p50_p95_p99"`
	ReadRows     string            `json:"scan_rows_last_hour"`
	ReadBytes    string            `json:"scan_bytes_last_hour"`
	ResultRows   string            `json:"result_rows_last_hour"`
	ResultBytes  string            `json:"result_bytes_last_hour"`
	Events       map[string]string `json:"profile_events_last_hour"`
}
type MaintenanceStatus struct {
	At             time.Time         `json:"at"`
	Mode           string            `json:"mode"`
	Jobs           []QueueMetric     `json:"jobs"`
	Batches        []QueueMetric     `json:"batches"`
	Attempts       map[string]string `json:"attempts"`
	Dirty          string            `json:"dirty_ranges"`
	DirtyAge       string            `json:"oldest_dirty_seconds"`
	ExpiredLeases  string            `json:"expired_job_leases"`
	Quarantined    string            `json:"quarantined_channels"`
	ConflictIssues string            `json:"reported_conflict_errors"`
	TempFree       string            `json:"temp_free_bytes"`
	TempInodes     string            `json:"temp_free_inodes"`
	Replicas       []ReplicaMetric   `json:"replicas"`
	RuntimeLog     string            `json:"runtime_log_contract"`
}

// MaintenanceStatus is a read-only operator view. Physical rows include retained
// attempts/copies and are deliberately never labeled as active business rows.
func (s *Service) MaintenanceStatus(ctx context.Context) (MaintenanceStatus, error) {
	out := MaintenanceStatus{At: time.Now().UTC(), Jobs: []QueueMetric{}, Batches: []QueueMetric{}, Attempts: map[string]string{}, Replicas: []ReplicaMetric{}, RuntimeLog: "monitoring.operation JSON: trace_id, operation, duration_ms, status, response_bytes, object_read/write_bytes, object_reads/writes/errors, cancelled, cancel_return_ms; CH query IDs retain trace_id"}
	if e := s.requireStorage(); e != nil {
		return out, e
	}
	e := s.db(ctx, func(tx pgx.Tx) error {
		if e := tx.QueryRow(ctx, `SELECT mode FROM monitoring_environment_state WHERE id=1`).Scan(&out.Mode); e != nil {
			return e
		}
		for _, q := range []struct {
			sql string
			to  *[]QueueMetric
		}{{`SELECT kind,state,count(*)::text,COALESCE(floor(EXTRACT(epoch FROM clock_timestamp()-min(created_at))),0)::text FROM monitoring_jobs GROUP BY kind,state ORDER BY kind,state`, &out.Jobs}, {`SELECT 'import',state,count(*)::text,COALESCE(floor(EXTRACT(epoch FROM clock_timestamp()-min(updated_at))),0)::text FROM monitoring_import_batches GROUP BY state ORDER BY state`, &out.Batches}} {
			rows, e := tx.Query(ctx, q.sql)
			if e != nil {
				return e
			}
			for rows.Next() {
				var v QueueMetric
				if e = rows.Scan(&v.Kind, &v.State, &v.Count, &v.OldestSeconds); e != nil {
					break
				}
				*q.to = append(*q.to, v)
			}
			if e == nil {
				e = rows.Err()
			}
			rows.Close()
			if e != nil {
				return e
			}
		}
		rows, e := tx.Query(ctx, `SELECT state,count(*)::text FROM monitoring_write_attempts GROUP BY state`)
		if e != nil {
			return e
		}
		for rows.Next() {
			var state, count string
			if e = rows.Scan(&state, &count); e != nil {
				break
			}
			out.Attempts[state] = count
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return e
		}
		if e = tx.QueryRow(ctx, `SELECT count(*)::text,COALESCE(floor(EXTRACT(epoch FROM clock_timestamp()-min(created_at))),0)::text FROM monitoring_dirty_ranges`).Scan(&out.Dirty, &out.DirtyAge); e != nil {
			return e
		}
		return tx.QueryRow(ctx, `SELECT (SELECT count(*)::text FROM monitoring_jobs WHERE state='running' AND lease_until<clock_timestamp()),(SELECT count(*)::text FROM monitoring_channel_versions WHERE quarantined),(SELECT COALESCE(sum((counts->>'errors')::numeric),0)::text FROM monitoring_import_batches)`).Scan(&out.ExpiredLeases, &out.Quarantined, &out.ConflictIssues)
	})
	if e != nil {
		return out, e
	}
	path := s.storage.config.TempDirectory
	if path == "" {
		path = os.TempDir()
	}
	var fs syscall.Statfs_t
	if e = syscall.Statfs(path, &fs); e != nil {
		return out, e
	}
	out.TempFree = fmt.Sprint(fs.Bavail * uint64(fs.Bsize))
	out.TempInodes = fmt.Sprint(fs.Ffree)
	for _, name := range s.storage.physicalNames() {
		p, _ := s.storage.physical(name)
		for i, c := range p.replicas {
			v := ReplicaMetric{Physical: name, Replica: i + 1, State: "ready", Events: map[string]string{}}
			var parts, rows, bytes, queue, delay, merges, queries, readRows, readBytes, resultRows, resultBytes uint64
			err := c.QueryRow(ctx, `SELECT count(),coalesce(sum(rows),0),coalesce(sum(bytes_on_disk),0) FROM system.parts WHERE active AND database=? AND table='observations'`, p.config.Database).Scan(&parts, &rows, &bytes)
			if err == nil {
				err = c.QueryRow(ctx, `SELECT toUInt64(queue_size),absolute_delay FROM system.replicas WHERE database=? AND table='observations'`, p.config.Database).Scan(&queue, &delay)
			}
			if err == nil {
				err = c.QueryRow(ctx, `SELECT count() FROM system.merges WHERE database=? AND table='observations'`, p.config.Database).Scan(&merges)
			}
			events := []string{"ReadBufferFromS3Bytes", "WriteBufferFromS3Bytes", "FilesystemCacheReadBytes", "S3ReadRequestsErrors", "S3WriteRequestsErrors", "Query"}
			var values []uint64
			if err == nil {
				err = c.QueryRow(ctx, `SELECT count(),quantilesTDigest(0.5,0.95,0.99)(query_duration_ms),sum(read_rows),sum(read_bytes),sum(result_rows),sum(result_bytes),arrayMap(k->sumMap(ProfileEvents)[k],?) FROM system.query_log WHERE event_time>=now()-INTERVAL 1 HOUR AND type='QueryFinish' AND has(databases,?)`, events, p.config.Database).Scan(&queries, &v.Latency, &readRows, &readBytes, &resultRows, &resultBytes, &values)
			}
			if err != nil {
				v.State = "metrics_unavailable"
				v.Latency = []float64{}
			} else {
				for n, event := range events {
					v.Events[event] = fmt.Sprint(values[n])
				}
				if queries == 0 {
					v.Latency = []float64{}
				}
			}
			v.Parts = fmt.Sprint(parts)
			v.PhysicalRows = fmt.Sprint(rows)
			v.DiskBytes = fmt.Sprint(bytes)
			v.Queue = fmt.Sprint(queue)
			v.Delay = fmt.Sprint(delay)
			v.Merges = fmt.Sprint(merges)
			v.Queries = fmt.Sprint(queries)
			v.ReadRows = fmt.Sprint(readRows)
			v.ReadBytes = fmt.Sprint(readBytes)
			v.ResultRows = fmt.Sprint(resultRows)
			v.ResultBytes = fmt.Sprint(resultBytes)
			out.Replicas = append(out.Replicas, v)
		}
	}
	return out, nil
}
