//go:build monitoringintegration

// Package contracttest exercises real local services, independently of the API.
// It is a storage qualification laboratory, not an import implementation.
package contracttest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/ext"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/shopspring/decimal"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring/internal/observation"
)

type laboratory struct {
	ctx context.Context
	ch  [2]driver.Conn
	pg  *pgx.Conn
	db  string
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func connect(t *testing.T, port string, dial func(context.Context, string) (net.Conn, error)) driver.Conn {
	t.Helper()
	c, err := clickhouse.Open(&clickhouse.Options{Addr: []string{"127.0.0.1:" + port}, DialTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, MaxOpenConns: 2, DialContext: dial})
	must(t, err)
	t.Cleanup(func() { _ = c.Close() })
	must(t, c.Ping(t.Context()))
	return c
}

func newLab(t *testing.T) *laboratory {
	t.Helper()
	if os.Getenv("INFRAPRO_MONITORING_CONTRACT") != "1" {
		t.Fatal("run pnpm test:monitoring:contract; this test needs the disposable local laboratory")
	}
	l := &laboratory{ctx: t.Context(), db: "contract_" + strings.ReplaceAll(uuid.NewString(), "-", "")}
	l.ch[0], l.ch[1] = connect(t, "59000", nil), connect(t, "59001", nil)
	var err error
	l.pg, err = pgx.Connect(l.ctx, "postgres://contract:contract-only-local-password@127.0.0.1:56432/contract")
	must(t, err)
	t.Cleanup(func() { _ = l.pg.Close(context.Background()) })
	_, err = l.pg.Exec(l.ctx, "CREATE SCHEMA "+l.db)
	must(t, err)
	t.Cleanup(func() { _, _ = l.pg.Exec(context.Background(), "DROP SCHEMA "+l.db+" CASCADE") })
	_, err = l.pg.Exec(l.ctx, "SET search_path TO "+l.db)
	must(t, err)
	for _, c := range l.ch {
		must(t, c.Exec(l.ctx, "CREATE DATABASE "+l.db))
		t.Cleanup(func() { _ = c.Exec(context.Background(), "DROP DATABASE "+l.db+" SYNC") })
		must(t, c.Exec(l.ctx, fmt.Sprintf(observationDDL, l.db, l.db)))
	}
	var version string
	must(t, l.ch[0].QueryRow(l.ctx, "SELECT version()").Scan(&version))
	if version != "26.8.36.3" {
		t.Fatal("unqualified version", version)
	}
	t.Log("ClickHouse", version)
	must(t, l.pg.QueryRow(l.ctx, "SHOW server_version").Scan(&version))
	t.Log("PostgreSQL", version)
	return l
}

const observationDDL = `CREATE TABLE %s.observations (
 tenant_id UUID, channel_id UUID, observation_id UUID, adoption_id UUID,
 batch_id UUID, chunk_id UUID, attempt_id UUID, row_ordinal UInt64,
 observed_at DateTime64(9,'UTC'), interval_start Nullable(DateTime64(9,'UTC')),
 received_at DateTime64(9,'UTC'), source_offset Int32, source_precision UInt8,
 value_kind Enum8('integer'=1,'decimal'=2,'enum'=3),
 integer_value Nullable(Int64), decimal_value Nullable(Decimal128(12)), enum_value Nullable(String),
 quality LowCardinality(String), source_quality String, rule_quality String, quality_reason String,
 source_file_id UUID, source_sheet String, source_line UInt64, source_column String, source_record_id String,
 original_value String, semantic_hash String, mapping_revision UInt64, validation_version UInt16,
 first_adoption UInt8, payload_hash FixedString(64), logical_shard UInt16,
 CONSTRAINT typed_value CHECK (value_kind='integer' AND isNotNull(integer_value) AND isNull(decimal_value) AND isNull(enum_value)) OR (value_kind='decimal' AND isNull(integer_value) AND isNotNull(decimal_value) AND isNull(enum_value)) OR (value_kind='enum' AND isNull(integer_value) AND isNull(decimal_value) AND isNotNull(enum_value))
) ENGINE=ReplicatedMergeTree('/contract/%s/observations','{replica}')
PARTITION BY toYYYYMM(observed_at)
ORDER BY (tenant_id,channel_id,observed_at,observation_id,adoption_id)
SETTINGS storage_policy='contract_tiered', index_granularity=8192, allow_remote_fs_zero_copy_replication=0`

type sample struct {
	at                   time.Time
	kind, value, quality string
}

func boundarySamples(t *testing.T) []sample {
	t.Helper()
	inputs := []struct{ at, kind, value string }{
		{"1900-01-01T00:00:00Z", "integer", "-9223372036854775808"},
		{"1969-12-31T23:59:59.999999999Z", "integer", "9223372036854775807"},
		{"2024-02-29T12:00:00.123456789+08:00", "decimal", "-99999999999999999999999999.999999999999"},
		{"2262-04-11T23:47:16.854775807Z", "decimal", "99999999999999999999999999.999999999999"},
		{"2024-11-03T01:30:00.000000001-04:00", "decimal", "10000000000000000000000000.000000000001"},
		{"2024-11-03T01:30:00.000000003-05:00", "decimal", "10000000000000000000000000.000000000003"},
		{"2024-01-01T00:00:00Z", "enum", "stable-state-key"},
		{"2024-01-01T00:00:00.000000001Z", "decimal", "0.000000000000"},
	}
	out := make([]sample, len(inputs))
	for i, v := range inputs {
		tm, err := observation.ParseTime(v.at)
		must(t, err)
		out[i] = sample{tm, v.kind, v.value, []string{"good", "unmarked", "suspect", "bad"}[i%4]}
	}
	return out
}

func payload(s sample, ordinal uint64) string {
	b, _ := json.Marshal([]any{"v1", s.at.Format(time.RFC3339Nano), s.kind, s.value, s.quality, ordinal})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (l *laboratory) insert(t *testing.T, c driver.Conn, chunk, attempt uuid.UUID, data []sample, quorum int, afterAppend func()) error {
	t.Helper()
	ctx := clickhouse.Context(l.ctx, clickhouse.WithSettings(clickhouse.Settings{"insert_quorum": quorum, "insert_quorum_timeout": 3000, "insert_deduplicate": 0, "async_insert": 0}))
	b, err := c.PrepareBatch(ctx, "INSERT INTO "+l.db+".observations")
	if err != nil {
		return err
	}
	defer b.Close()
	for i, s := range data {
		var iv *int64
		var dv *decimal.Decimal
		var ev *string
		switch s.kind {
		case "integer":
			v, e := observation.Integer(s.value)
			if e != nil {
				return e
			}
			iv = &v
		case "decimal":
			v, e := observation.Decimal(s.value)
			if e != nil {
				return e
			}
			d := decimal.NewFromBigInt(v, -12)
			dv = &d
		case "enum":
			ev = &s.value
		default:
			return errors.New("value_kind")
		}
		id := uuid.NewSHA1(chunk, []byte(fmt.Sprint(i)))
		channel := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprint(i%128)))
		err = b.Append(uuid.Nil, channel, id, id, chunk, chunk, attempt, uint64(i), s.at, nil, time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC), int32(0), uint8(9), s.kind, iv, dv, ev, s.quality, s.quality, "bounds-v1", "", chunk, "synthetic", uint64(i+2), "C", "source-reference", s.value, strings.Repeat("a", 64), uint64(1), uint16(1), uint8(1), payload(s, uint64(i)), uint16(i%256))
		if err != nil {
			return err
		}
	}
	if afterAppend != nil {
		afterAppend()
	}
	return b.Send()
}

func (l *laboratory) verify(c driver.Conn, chunk, attempt uuid.UUID, data []sample) error {
	rows, err := c.Query(l.ctx, "SELECT row_ordinal,observed_at,value_kind,integer_value,decimal_value,enum_value,quality,payload_hash FROM "+l.db+".observations WHERE chunk_id=? AND attempt_id=? ORDER BY row_ordinal", chunk, attempt)
	if err != nil {
		return err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var ordinal uint64
		var at time.Time
		var kind, quality, hash string
		var iv *int64
		var dv *decimal.Decimal
		var ev *string
		if err = rows.Scan(&ordinal, &at, &kind, &iv, &dv, &ev, &quality, &hash); err != nil {
			return err
		}
		if n >= len(data) || ordinal != uint64(n) {
			return errors.New("duplicate_or_missing_ordinal")
		}
		s := data[n]
		var value string
		switch kind {
		case "integer":
			if iv == nil || dv != nil || ev != nil {
				return errors.New("typed_value")
			}
			value = fmt.Sprint(*iv)
		case "decimal":
			if iv != nil || dv == nil || ev != nil {
				return errors.New("typed_value")
			}
			value = dv.StringFixed(12)
		case "enum":
			if iv != nil || dv != nil || ev == nil {
				return errors.New("typed_value")
			}
			value = *ev
		}
		if !at.Equal(s.at) || kind != s.kind || quality != s.quality || value != s.value || hash != payload(s, uint64(n)) {
			return fmt.Errorf("round_trip_mismatch row %d: at=%s kind=%s value=%s", n, at.Format(time.RFC3339Nano), kind, value)
		}
		n++
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if n != len(data) {
		return fmt.Errorf("incomplete_attempt: got %d want %d", n, len(data))
	}
	return nil
}

func (l *laboratory) selected(c driver.Conn, manifest [][2]uuid.UUID) (uint64, error) {
	if manifest == nil {
		return 0, errors.New("manifest_unavailable")
	}
	if len(manifest) == 0 {
		return 0, nil
	}
	table, err := ext.NewTable("selected_attempts", ext.Column("chunk_id", "UUID"), ext.Column("attempt_id", "UUID"))
	if err != nil {
		return 0, err
	}
	for _, m := range manifest {
		if err = table.Append(m[0], m[1]); err != nil {
			return 0, err
		}
	}
	ctx := clickhouse.Context(l.ctx, clickhouse.WithExternalTable(table))
	var count uint64
	err = c.QueryRow(ctx, "SELECT count() FROM "+l.db+".observations WHERE (chunk_id,attempt_id) IN (SELECT chunk_id,attempt_id FROM selected_attempts)").Scan(&count)
	return count, err
}

func TestStorageContract(t *testing.T) {
	l := newLab(t)
	data := boundarySamples(t)
	chunk, good := uuid.New(), uuid.New()
	t.Run("nanoseconds_decimal_integer_enum_two_replicas", func(t *testing.T) {
		must(t, l.insert(t, l.ch[0], chunk, good, data, 2, nil))
		for _, c := range l.ch {
			must(t, l.verify(c, chunk, good, data))
		}
	})
	t.Run("missing_and_empty_manifest_fail_closed", func(t *testing.T) {
		if _, err := l.selected(l.ch[0], nil); err == nil {
			t.Fatal("missing manifest accepted")
		}
		n, err := l.selected(l.ch[0], [][2]uuid.UUID{})
		must(t, err)
		if n != 0 {
			t.Fatal(n)
		}
	})
	t.Run("replay_outside_dedup_window_rejected", func(t *testing.T) {
		for _, c := range l.ch {
			must(t, c.Exec(l.ctx, "ALTER TABLE "+l.db+".observations MODIFY SETTING replicated_deduplication_window=0, replicated_deduplication_window_seconds=0"))
		}
		bad := uuid.New()
		must(t, l.insert(t, l.ch[0], chunk, bad, data, 2, nil))
		must(t, l.insert(t, l.ch[0], chunk, bad, data, 2, nil))
		if l.verify(l.ch[0], chunk, bad, data) == nil {
			t.Fatal("replayed attempt sealed")
		}
		for _, c := range l.ch {
			n, err := l.selected(c, [][2]uuid.UUID{{chunk, good}})
			must(t, err)
			if n != uint64(len(data)) {
				t.Fatal(n)
			}
		}
	})
	t.Run("insert_ack_lost_new_attempt_only", func(t *testing.T) {
		var armed atomic.Bool
		release := make(chan struct{})
		defer close(release)
		c := connect(t, "59000", func(ctx context.Context, addr string) (net.Conn, error) {
			raw, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
			if err != nil {
				return nil, err
			}
			return &lostAckConn{Conn: raw, armed: &armed, release: release}, nil
		})
		lost := uuid.New()
		outcome := make(chan error, 1)
		go func() { outcome <- l.insert(t, c, chunk, lost, data, 2, func() { armed.Store(true) }) }()
		deadline := time.Now().Add(10 * time.Second)
		for l.verify(l.ch[0], chunk, lost, data) != nil {
			if time.Now().After(deadline) {
				t.Fatal("fault injector did not reach a committed insert")
			}
			time.Sleep(10 * time.Millisecond)
		}
		release <- struct{}{}
		err := <-outcome
		if err == nil {
			t.Fatal("lost acknowledgement returned success")
		}
		// Independent verification proves this is a lost response, not a failed insert.
		must(t, l.verify(l.ch[0], chunk, lost, data))
		replacement := uuid.New()
		must(t, l.insert(t, l.ch[0], chunk, replacement, data, 2, nil))
		must(t, l.verify(l.ch[1], chunk, replacement, data))
		n, err := l.selected(l.ch[0], [][2]uuid.UUID{{chunk, replacement}})
		must(t, err)
		if n != uint64(len(data)) {
			t.Fatal(n)
		}
	})
	t.Run("postgres_atomic_manifest_and_nanosecond_ranges", func(t *testing.T) {
		_, err := l.pg.Exec(l.ctx, `CREATE TABLE publications (batch uuid PRIMARY KEY, chunk uuid UNIQUE, attempt uuid, time_ns bigint, permitted int8range)`)
		must(t, err)
		other := uuid.New()
		must(t, l.insert(t, l.ch[0], other, good, data, 2, nil))
		tx, err := l.pg.Begin(l.ctx)
		must(t, err)
		defer tx.Rollback(l.ctx)
		for _, ch := range []uuid.UUID{chunk, other} {
			_, err = tx.Exec(l.ctx, `INSERT INTO publications VALUES ($1,$2,$3,$4,int8range($4,NULL,'[)'))`, ch, ch, good, observation.MaxTime.UnixNano())
			must(t, err)
		}
		reader, err := pgx.Connect(l.ctx, "postgres://contract:contract-only-local-password@127.0.0.1:56432/contract")
		must(t, err)
		defer reader.Close(l.ctx)
		var n int
		must(t, reader.QueryRow(l.ctx, "SELECT count(*) FROM "+l.db+".publications").Scan(&n))
		if n != 0 {
			t.Fatal("half batch visible")
		}
		must(t, tx.Commit(l.ctx))
		var ns int64
		var inside bool
		must(t, l.pg.QueryRow(l.ctx, `SELECT time_ns,permitted @> time_ns FROM publications LIMIT 1`).Scan(&ns, &inside))
		if ns != observation.MaxTime.UnixNano() || !inside {
			t.Fatal("boundary lost")
		}
		rows, err := reader.Query(l.ctx, "SELECT chunk::text,attempt::text FROM "+l.db+".publications")
		must(t, err)
		defer rows.Close()
		m := [][2]uuid.UUID{}
		for rows.Next() {
			var ch, at string
			must(t, rows.Scan(&ch, &at))
			m = append(m, [2]uuid.UUID{uuid.MustParse(ch), uuid.MustParse(at)})
		}
		must(t, rows.Err())
		count, err := l.selected(l.ch[0], m)
		must(t, err)
		if count != uint64(2*len(data)) {
			t.Fatal(count)
		}
	})
	t.Run("lagging_replica_cannot_be_ready", func(t *testing.T) {
		must(t, l.ch[1].Exec(l.ctx, "SYSTEM STOP FETCHES "+l.db+".observations"))
		defer l.ch[1].Exec(context.Background(), "SYSTEM START FETCHES "+l.db+".observations")
		ch, at := uuid.New(), uuid.New()
		must(t, l.insert(t, l.ch[0], ch, at, data, 0, nil))
		must(t, l.verify(l.ch[0], ch, at, data))
		if l.verify(l.ch[1], ch, at, data) == nil {
			t.Fatal("lagging replica accepted")
		}
		must(t, l.ch[1].Exec(l.ctx, "SYSTEM START FETCHES "+l.db+".observations"))
		must(t, l.ch[1].Exec(l.ctx, "SYSTEM SYNC REPLICA "+l.db+".observations"))
		must(t, l.verify(l.ch[1], ch, at, data))
	})
	t.Run("online_s3_parts_and_failure", func(t *testing.T) {
		client := s3Client(t, "contract-admin", "contract-admin-test-only")
		err := client.MakeBucket(l.ctx, "contract-ch", minio.MakeBucketOptions{})
		if err != nil {
			exists, e := client.BucketExists(l.ctx, "contract-ch")
			must(t, e)
			if !exists {
				t.Fatal(err)
			}
		}
		for _, c := range l.ch {
			must(t, c.Exec(l.ctx, "SYSTEM STOP MERGES "+l.db+".observations"))
			must(t, c.Exec(l.ctx, "ALTER TABLE "+l.db+".observations MOVE PARTITION 202401 TO DISK 'contract_s3'"))
			must(t, l.verify(c, chunk, good, data))
		}
		// A bucket listing only checks filer metadata. After a restart the
		// master and data volumes may still be recovering. Use a non-inline
		// object written before the outage to qualify actual payload reads.
		probeKey := "readiness/" + l.db
		probe := strings.Repeat("restart-payload-", 8192)
		_, err = client.PutObject(l.ctx, "contract-ch", probeKey, strings.NewReader(probe), int64(len(probe)), minio.PutObjectOptions{})
		must(t, err)
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = client.RemoveObject(ctx, "contract-ch", probeKey, minio.RemoveObjectOptions{})
		})
		compose(t, "stop", "s3")
		defer compose(t, "start", "s3")
		err = l.verify(l.ch[0], chunk, good, data)
		if err == nil {
			t.Fatal("object outage returned successful complete data")
		}
		t.Log("object outage fails explicitly")
		compose(t, "start", "s3")
		waitS3Object(t, client, "contract-ch", probeKey, probe)
		must(t, l.verify(l.ch[0], chunk, good, data))
	})
}

func TestQueryCancellation(t *testing.T) {
	l := newLab(t)
	id := uuid.NewString()
	ctx, cancel := context.WithTimeout(l.ctx, 100*time.Millisecond)
	defer cancel()
	ctx = clickhouse.Context(ctx, clickhouse.WithQueryID(id))
	var n uint64
	err := l.ch[0].QueryRow(ctx, "SELECT sum(number) FROM numbers(1000000000000)").Scan(&n)
	if err == nil {
		t.Fatal("unbounded scan unexpectedly completed")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		must(t, l.ch[0].QueryRow(l.ctx, "SELECT count() FROM system.processes WHERE query_id=?", id).Scan(&n))
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cancelled query remains active")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Drop an actual native server response after reading it, so the write has
// reached ClickHouse and the official Go client sees a broken connection.
type lostAckConn struct {
	net.Conn
	armed   *atomic.Bool
	release <-chan struct{}
}

func (c *lostAckConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 && c.armed.Load() {
		<-c.release
		_ = c.Conn.Close()
		return 0, io.ErrUnexpectedEOF
	}
	return n, err
}

func s3Client(t *testing.T, key, secret string) *minio.Client {
	t.Helper()
	c, err := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4(key, secret, ""), Secure: false, Region: "us-east-1"})
	must(t, err)
	return c
}
func waitS3(t *testing.T, c *minio.Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	for {
		_, err := c.ListBuckets(ctx)
		if err == nil {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("S3 not ready", err)
		case <-time.After(250 * time.Millisecond):
		}
	}
}
func waitS3Object(t *testing.T, c *minio.Client, bucket, key, expected string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	for {
		object, err := c.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
		if err == nil {
			var data []byte
			data, err = io.ReadAll(object)
			_ = object.Close()
			if err == nil {
				if string(data) != expected {
					t.Fatal("S3 restart changed the readiness payload")
				}
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("S3 payload not ready after restart", err)
		case <-time.After(250 * time.Millisecond):
		}
	}
}
func compose(t *testing.T, args ...string) {
	t.Helper()
	root := os.Getenv("INFRAPRO_CONTRACT_ROOT")
	if root == "" {
		t.Fatal("missing laboratory root")
	}
	cmd := exec.CommandContext(t.Context(), "docker", append([]string{"compose", "-f", filepath.Join(root, "deploy/monitoring-contract/compose.yaml")}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("laboratory compose: %v: %s", err, out)
	}
}
