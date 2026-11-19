package monitoring

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/requestlimit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"io"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/ext"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring/internal/observation"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/shopspring/decimal"
)

// StorageConfig describes the default physical shard and explicitly named additions.
// Logical shards stay stable when physical routes are changed.
// Each physical shard may have one or more real replica endpoints. One endpoint
// uses the same layout and integrity checks without providing a redundant copy.
type PhysicalShardConfig struct {
	Name     string   `json:"name"`
	Database string   `json:"database"`
	Replicas []string `json:"replicas"`
}

type StorageConfig struct {
	ObjectRequests                                           *requestlimit.Limit
	ConnectionsPerReplica                                    int
	Shards                                                   []PhysicalShardConfig
	HotDays                                                  int
	OnlineVolume                                             string
	ExportRows                                               int64
	ExportBytes                                              int64
	Replicas                                                 []string
	Database, User, Password, Policy                         string
	TLS                                                      bool
	ObjectEndpoint, ObjectAccessKey, ObjectSecretKey, Bucket string
	TempDirectory                                            string
}
type Storage struct {
	summaryNamespace string
	config           StorageConfig
	shards           map[string]*Storage
	replicas         []driver.Conn
	objects          *minio.Client
}

func OpenStorage(ctx context.Context, cfg StorageConfig) (*Storage, error) {
	return openStorage(ctx, cfg, true)
}

// ConnectStorage constructs lazy clients so an unrelated control request can start during a storage outage.
func ConnectStorage(ctx context.Context, cfg StorageConfig) (*Storage, error) {
	return openStorage(ctx, cfg, false)
}

func openStorage(ctx context.Context, cfg StorageConfig, probe bool) (*Storage, error) {
	if cfg.HotDays == 0 {
		cfg.HotDays = 90
	}
	if cfg.OnlineVolume == "" {
		cfg.OnlineVolume = "online"
	}
	if cfg.HotDays < 1 || cfg.HotDays > 3650 || !regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`).MatchString(cfg.OnlineVolume) {
		return nil, fmt.Errorf("invalid monitoring online tier configuration")
	}
	// Independently starting roles can share the caller's replica endpoints.
	cfg.Replicas = slices.Clone(cfg.Replicas)
	seen := map[string]bool{}
	for i, addr := range cfg.Replicas {
		host, port, e := net.SplitHostPort(strings.TrimSpace(addr))
		n, parse := strconv.Atoi(port)
		if e != nil || host == "" || parse != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("invalid monitoring replica endpoint")
		}
		addr = net.JoinHostPort(strings.ToLower(host), strconv.Itoa(n))
		if seen[addr] {
			return nil, fmt.Errorf("duplicate monitoring replica endpoint")
		}
		seen[addr] = true
		cfg.Replicas[i] = addr
	}

	if cfg.ExportRows == 0 {
		cfg.ExportRows = 1000000
	}
	if cfg.ExportBytes == 0 {
		cfg.ExportBytes = 256 << 20
	}
	if cfg.ExportRows < 1 || cfg.ExportRows > 1000000 || cfg.ExportBytes < 1<<20 || cfg.ExportBytes > 1<<30 {
		return nil, fmt.Errorf("invalid export volume limits")
	}
	if len(cfg.Replicas) == 0 || !regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`).MatchString(cfg.Database) || !regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`).MatchString(cfg.Policy) {
		return nil, fmt.Errorf("monitoring storage requires a database, policy and at least one replica endpoint")
	}
	endpoint, e := url.Parse(cfg.ObjectEndpoint)
	if e != nil || endpoint.Host == "" || (endpoint.Scheme != "https" && endpoint.Scheme != "http") || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || (endpoint.Path != "" && endpoint.Path != "/") || cfg.Bucket == "" {
		return nil, fmt.Errorf("invalid monitoring object configuration")
	}
	tr, e := blobstore.Transport(endpoint.Scheme == "https", cfg.ObjectRequests)
	if e != nil {
		return nil, e
	}
	client, e := minio.New(endpoint.Host, &minio.Options{Creds: credentials.NewStaticV4(cfg.ObjectAccessKey, cfg.ObjectSecretKey, ""), Secure: endpoint.Scheme == "https", Transport: tr, BucketLookup: minio.BucketLookupPath})
	if e != nil {
		return nil, fmt.Errorf("invalid monitoring object client")
	}
	if cfg.ConnectionsPerReplica == 0 {
		cfg.ConnectionsPerReplica = 6
	}
	s := &Storage{config: cfg, objects: client, shards: map[string]*Storage{}}
	for _, addr := range cfg.Replicas {
		opt := &clickhouse.Options{Addr: []string{addr}, Auth: clickhouse.Auth{Username: cfg.User, Password: cfg.Password}, DialTimeout: 5 * time.Second, ReadTimeout: 2 * time.Minute, MaxOpenConns: cfg.ConnectionsPerReplica, MaxIdleConns: min(2, cfg.ConnectionsPerReplica), Settings: clickhouse.Settings{
			"max_execution_time": 120, "max_memory_usage": 512 << 20, "max_threads": 2, "skip_unavailable_shards": 0,
			"max_block_size": 8192, "preferred_block_size_bytes": 1 << 20, "max_streams_to_max_threads_ratio": 1,
			// Authorization joins can require a sort even when the underlying
			// primary key is ordered. Spill before the hard query memory limit.
			"max_bytes_before_external_sort": 64 << 20, "max_temporary_data_on_disk_size_for_query": 2 << 30,
		}}
		if cfg.TLS {
			opt.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
		}
		c, e := clickhouse.Open(opt)
		if e != nil {
			s.Close()
			return nil, fmt.Errorf("invalid monitoring replica configuration")
		}
		s.replicas = append(s.replicas, c)
		if probe {
			if e = c.Ping(ctx); e != nil {
				s.Close()
				return nil, fmt.Errorf("monitoring replica unavailable")
			}
		}
	}
	if len(cfg.Shards) > 32 {
		s.Close()
		return nil, fmt.Errorf("too many physical shards")
	}
	databases := map[string]bool{cfg.Database: true}
	for _, shard := range cfg.Shards {
		if !regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`).MatchString(shard.Name) || shard.Name == "local" || s.shards[shard.Name] != nil || databases[shard.Database] {
			s.Close()
			return nil, fmt.Errorf("invalid or duplicate physical shard")
		}
		databases[shard.Database] = true
		child := cfg
		child.Shards = nil
		child.Database = shard.Database
		child.Replicas = shard.Replicas
		part, err := openStorage(ctx, child, probe)
		if err != nil {
			s.Close()
			return nil, err
		}
		s.shards[shard.Name] = part
	}
	return s, nil
}
func (s *Storage) Close() {
	if s != nil {
		for _, shard := range s.shards {
			shard.Close()
		}
		for _, c := range s.replicas {
			_ = c.Close()
		}
	}
}

// Initialize is an explicit maintenance action, never an HTTP startup side effect.
func (s *Storage) Initialize(ctx context.Context) error {
	for _, shard := range s.shards {
		if e := shard.Initialize(ctx); e != nil {
			return e
		}
	}
	for _, c := range s.replicas {
		if e := c.Exec(ctx, "CREATE DATABASE IF NOT EXISTS "+s.config.Database); e != nil {
			return e
		}
		if e := c.Exec(ctx, fmt.Sprintf(rawDDL, s.config.Database, s.config.Database, s.config.HotDays, s.config.OnlineVolume, s.config.Policy)); e != nil {
			return e
		}
		if e := s.initializeLayout(ctx, c); e != nil {
			return e
		}
	}
	exists, e := s.objects.BucketExists(ctx, s.config.Bucket)
	if e != nil {
		return e
	}
	if !exists {
		return s.objects.MakeBucket(ctx, s.config.Bucket, minio.MakeBucketOptions{})
	}
	return nil
}
func (s *Storage) Check(ctx context.Context) error {
	if e := s.checkLayout(ctx); e != nil {
		return e
	}
	for _, shard := range s.shards {
		if e := shard.Check(ctx); e != nil {
			return e
		}
	}
	return nil
}
func (s *Storage) physical(name string) (*Storage, error) {
	if name == "" || name == "local" {
		return s, nil
	}
	if v := s.shards[name]; v != nil {
		return v, nil
	}
	return nil, fmt.Errorf("physical shard is not configured")
}
func (s *Storage) writeAt(ctx context.Context, physical, chunk, attempt string, data []rawObservation) error {
	p, e := s.physical(physical)
	if e != nil {
		return e
	}
	return p.write(ctx, chunk, attempt, data)
}

const rawDDL = `CREATE TABLE IF NOT EXISTS %s.observations (
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
 ) ENGINE=ReplicatedMergeTree('/infrapro/%s/observations','{replica}')
 PARTITION BY toYYYYMM(observed_at)
 ORDER BY (tenant_id,channel_id,observed_at,observation_id,adoption_id)
 TTL greatest(toDateTime64(observed_at,0,'UTC'),toDateTime64(1,0,'UTC')) + INTERVAL %d DAY TO VOLUME '%s'
 SETTINGS storage_policy='%s', index_granularity=8192, allow_remote_fs_zero_copy_replication=0`

// rawObservation is the exact immutable payload. Physical attempts and ordinals
// are outside its identity. JSON is used only for sealed object evidence.
type rawObservation struct {
	Tenant, Channel, Observation, Adoption, Batch                   string
	At                                                              int64  `json:",string"`
	Start                                                           *int64 `json:",string"`
	Received                                                        int64  `json:",string"`
	Offset                                                          int32
	Precision                                                       uint8
	Kind, Value, Quality, SourceQuality, RuleQuality, QualityReason string
	File, Sheet                                                     string
	Line                                                            uint64 `json:",string"`
	Column, Record, Original, Semantic                              string
	Mapping                                                         uint64 `json:",string"`
	Validation                                                      uint16
	First                                                           bool
	Shard                                                           uint16
}

func (r rawObservation) hash() string { return digest(r) }
func logicalShard(tenant, channel string) uint16 {
	a, b := uuid.MustParse(tenant), uuid.MustParse(channel)
	h := sha256.New()
	h.Write(a[:])
	h.Write(b[:])
	return uint16(h.Sum(nil)[0])
}
func payloadHash(data []rawObservation) string {
	h := sha256.New()
	for _, r := range data {
		io.WriteString(h, r.hash())
	}
	return hex.EncodeToString(h.Sum(nil))
}

type selectedChunk struct {
	Physical             string `json:",omitempty"`
	Chunk, Attempt, Hash string
	Count                uint64
	First                uint64
	Batch                string
	Sequence             int64
	Revoked              bool
	Qualities            map[string]string
}

func (s *Storage) write(ctx context.Context, chunk, attempt string, data []rawObservation) error {
	// No automatic application replay, dedup reliance or asynchronous inserts.
	ctx = clickhouse.Context(chContext(ctx), clickhouse.WithSettings(clickhouse.Settings{"insert_quorum": len(s.replicas), "insert_quorum_timeout": 30000, "insert_deduplicate": 0, "async_insert": 0}))
	b, e := s.replicas[0].PrepareBatch(ctx, "INSERT INTO "+s.config.Database+".observations")
	if e != nil {
		return e
	}
	defer b.Close()
	for i, r := range data {
		var iv *int64
		var dv *decimal.Decimal
		var ev *string
		var start *time.Time
		if r.Start != nil {
			v := time.Unix(0, *r.Start).UTC()
			start = &v
		}
		switch r.Kind {
		case "integer":
			v, e := observation.Integer(r.Value)
			if e != nil {
				return e
			}
			iv = &v
		case "decimal":
			v, e := observation.Decimal(r.Value)
			if e != nil {
				return e
			}
			d := decimal.NewFromBigInt(v, -12)
			dv = &d
		case "enum":
			ev = &r.Value
		default:
			return fmt.Errorf("unknown value kind")
		}
		var first uint8
		if r.First {
			first = 1
		}
		e = b.Append(uuid.MustParse(r.Tenant), uuid.MustParse(r.Channel), uuid.MustParse(r.Observation), uuid.MustParse(r.Adoption), uuid.MustParse(r.Batch), uuid.MustParse(chunk), uuid.MustParse(attempt), uint64(i), time.Unix(0, r.At).UTC(), start, time.Unix(0, r.Received).UTC(), r.Offset, r.Precision, r.Kind, iv, dv, ev, r.Quality, r.SourceQuality, r.RuleQuality, r.QualityReason, uuid.MustParse(r.File), r.Sheet, r.Line, r.Column, r.Record, r.Original, r.Semantic, r.Mapping, r.Validation, first, r.hash(), r.Shard)
		if e != nil {
			return e
		}
	}
	return b.Send()
}

const rowColumns = `tenant_id,channel_id,observation_id,adoption_id,batch_id,observed_at,interval_start,received_at,source_offset,source_precision,value_kind,integer_value,decimal_value,enum_value,quality,source_quality,rule_quality,quality_reason,source_file_id,source_sheet,source_line,source_column,source_record_id,original_value,semantic_hash,mapping_revision,validation_version,first_adoption,logical_shard,payload_hash,row_ordinal`

func scanRaw(rows driver.Rows) (rawObservation, string, uint64, error) {
	var r rawObservation
	var tenant, channel, obs, adopt, batch, file uuid.UUID
	var at, received time.Time
	var start *time.Time
	var iv *int64
	var dv *decimal.Decimal
	var ev *string
	var first uint8
	var hash string
	var ordinal uint64
	e := rows.Scan(&tenant, &channel, &obs, &adopt, &batch, &at, &start, &received, &r.Offset, &r.Precision, &r.Kind, &iv, &dv, &ev, &r.Quality, &r.SourceQuality, &r.RuleQuality, &r.QualityReason, &file, &r.Sheet, &r.Line, &r.Column, &r.Record, &r.Original, &r.Semantic, &r.Mapping, &r.Validation, &first, &r.Shard, &hash, &ordinal)
	if e != nil {
		return r, "", 0, e
	}
	r.Tenant, r.Channel, r.Observation, r.Adoption, r.Batch, r.File = tenant.String(), channel.String(), obs.String(), adopt.String(), batch.String(), file.String()
	r.At, r.Received = at.UnixNano(), received.UnixNano()
	if start != nil {
		v := start.UnixNano()
		r.Start = &v
	}
	r.First = first == 1
	switch r.Kind {
	case "integer":
		if iv == nil || dv != nil || ev != nil {
			return r, "", 0, fmt.Errorf("invalid typed integer")
		}
		r.Value = fmt.Sprint(*iv)
	case "decimal":
		if dv == nil || iv != nil || ev != nil {
			return r, "", 0, fmt.Errorf("invalid typed decimal")
		}
		r.Value = dv.StringFixed(12)
	case "enum":
		if ev == nil || iv != nil || dv != nil {
			return r, "", 0, fmt.Errorf("invalid typed enum")
		}
		r.Value = *ev
	default:
		return r, "", 0, fmt.Errorf("invalid value kind")
	}
	if r.hash() != hash {
		return r, "", 0, fmt.Errorf("payload integrity mismatch")
	}
	return r, hash, ordinal, nil
}

func (s *Storage) verifyAttempt(ctx context.Context, c driver.Conn, entry selectedChunk) error {
	rows, e := c.Query(chContext(ctx), "SELECT "+rowColumns+" FROM "+s.config.Database+".observations WHERE chunk_id=? AND attempt_id=? ORDER BY row_ordinal", uuid.MustParse(entry.Chunk), uuid.MustParse(entry.Attempt))
	if e != nil {
		return e
	}
	defer rows.Close()
	h := sha256.New()
	var n uint64
	for rows.Next() {
		_, hash, ordinal, e := scanRaw(rows)
		if e != nil {
			return e
		}
		if ordinal != n {
			return fmt.Errorf("incomplete or replayed attempt")
		}
		n++
		io.WriteString(h, hash)
	}
	if e = rows.Err(); e != nil {
		return e
	}
	if n != entry.Count || hex.EncodeToString(h.Sum(nil)) != entry.Hash {
		return fmt.Errorf("attempt manifest mismatch")
	}
	return nil
}
func (s *Storage) sealCheck(ctx context.Context, entry selectedChunk) error {
	var e error
	s, e = s.physical(entry.Physical)
	if e != nil {
		return e
	}
	for _, c := range s.replicas {
		if e := s.verifyAttempt(ctx, c, entry); e != nil {
			return e
		}
	}
	return nil
}

// A selected replica must independently prove that every manifest member is
// present. Missing manifest is never interpreted as an unrestricted query.
func (s *Storage) readyReplica(ctx context.Context, manifest []selectedChunk) (driver.Conn, error) {
	if manifest == nil {
		return nil, fmt.Errorf("missing publication manifest")
	}
	if len(manifest) > 20000 {
		return nil, problem("query_budget_exceeded", "清单超过交互预算，请缩小范围或使用后台导出。", 422)
	}
	for _, c := range s.replicas {
		good := true
		for _, m := range manifest {
			var n, unique, min, max uint64
			var hash string
			e := c.QueryRow(chContext(ctx), "SELECT count(),uniqExact(row_ordinal),min(row_ordinal),max(row_ordinal),lower(hex(SHA256(arrayStringConcat(arrayMap(x->x.2,arraySort(x->x.1,groupArray((row_ordinal,payload_hash)))))))) FROM "+s.config.Database+".observations WHERE chunk_id=? AND attempt_id=?", uuid.MustParse(m.Chunk), uuid.MustParse(m.Attempt)).Scan(&n, &unique, &min, &max, &hash)
			if e != nil || n != m.Count || unique != m.Count || min != 0 || max+1 != m.Count || hash != m.Hash {
				good = false
				break
			}
		}
		if good {
			return c, nil
		}
	}
	return nil, problem("monitoring_replica_unavailable", "发布清单所需数据尚未在可用副本上完整就绪。", 503)
}

type rawRead struct {
	Manifest  []selectedChunk
	Tenant    string
	Windows   map[string][]Window
	Qualities []string
	Range     *Window
	Order     string
	Limit     int
	Boundary  *rawObservation
	Reverse   bool
	// The fixed input Reader separately seals and authorizes full intervals.
	EndpointOnly bool
}

func (s *Storage) readOne(ctx context.Context, q rawRead, consume func(rawObservation) error) error {
	return s.queryRows(ctx, q, consume, nil)
}

// Counting uses the same replica readiness, manifest, quality and authorization
// predicates as reading rows. Only the aggregate crosses the storage boundary.
func (s *Storage) countRows(ctx context.Context, q rawRead) (uint64, error) {
	var total uint64
	for name, entries := range physicalManifests(q.Manifest) {
		shard, err := s.physical(name)
		if err != nil {
			return 0, err
		}
		q.Manifest = entries
		var count uint64
		if err := shard.queryRows(ctx, q, nil, &count); err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

func (s *Storage) queryRows(ctx context.Context, q rawRead, consume func(rawObservation) error, count *uint64) error {
	if q.Manifest == nil {
		return fmt.Errorf("missing publication manifest")
	}
	if len(q.Manifest) == 0 || len(q.Windows) == 0 {
		return nil
	}
	c, e := s.readyReplica(ctx, q.Manifest)
	if e != nil {
		return e
	}
	table, e := ext.NewTable("selected_manifest", ext.Column("chunk_id", "UUID"), ext.Column("attempt_id", "UUID"))
	if e != nil {
		return e
	}
	for _, m := range q.Manifest {
		if e = table.Append(uuid.MustParse(m.Chunk), uuid.MustParse(m.Attempt)); e != nil {
			return e
		}
	}
	ranges, e := ext.NewTable("authorized_windows", ext.Column("channel_id", "UUID"), ext.Column("lo", "Nullable(Int64)"), ext.Column("hi", "Nullable(Int64)"))
	if e != nil {
		return e
	}
	for id, windows := range q.Windows {
		for _, w := range UnionWindows(windows) {
			if e = ranges.Append(uuid.MustParse(id), w.From, w.To); e != nil {
				return e
			}
		}
	}
	ctx = clickhouse.Context(ctx, clickhouse.WithExternalTable(table, ranges), clickhouse.WithQueryID("monitoring_"+TraceID(ctx)+"_"+uuid.NewString()))
	where := ` WHERE r.tenant_id=? AND (r.chunk_id,r.attempt_id) IN (SELECT chunk_id,attempt_id FROM selected_manifest) AND (isNull(w.lo) OR toUnixTimestamp64Nano(coalesce(r.interval_start,r.observed_at))>=w.lo) AND (isNull(w.hi) OR toUnixTimestamp64Nano(r.observed_at)<w.hi)`
	if q.EndpointOnly {
		where = strings.Replace(where, "coalesce(r.interval_start,r.observed_at)", "r.observed_at", 1)
	}
	args := []any{uuid.MustParse(q.Tenant)}
	// These redundant bounds let MergeTree prune data before the authorization
	// join. The join still enforces every disjoint interval and whole-interval
	// permission. An interval endpoint cannot precede its interval start.
	where += " AND r.channel_id IN (SELECT channel_id FROM authorized_windows)"
	var lower, upper *int64
	openLower, openUpper := false, false
	for _, windows := range q.Windows {
		for _, w := range windows {
			if w.From == nil {
				openLower = true
			} else if lower == nil || *w.From < *lower {
				lower = w.From
			}
			if w.To == nil {
				openUpper = true
			} else if upper == nil || *w.To > *upper {
				upper = w.To
			}
		}
	}
	if !openLower && lower != nil {
		where += " AND r.observed_at>=fromUnixTimestamp64Nano(?)"
		args = append(args, *lower)
	}
	if !openUpper && upper != nil {
		where += " AND r.observed_at<fromUnixTimestamp64Nano(?)"
		args = append(args, *upper)
	}
	if len(q.Qualities) > 0 {
		where += " AND has(?,r.quality)"
		args = append(args, q.Qualities)
	}
	if q.Range != nil {
		if q.Range.From != nil {
			where += " AND toUnixTimestamp64Nano(r.observed_at)>=?"
			args = append(args, *q.Range.From)
		}
		if q.Range.To != nil {
			where += " AND toUnixTimestamp64Nano(r.observed_at)<?"
			args = append(args, *q.Range.To)
		}
	}
	from := " FROM " + s.config.Database + ".observations r INNER JOIN authorized_windows w ON w.channel_id=r.channel_id"
	if count != nil {
		return c.QueryRow(ctx, "SELECT count()"+from+where, args...).Scan(count)
	}
	order := "r.observed_at,r.channel_id,r.observation_id,r.adoption_id"
	if q.Order == "channel" {
		order = "r.channel_id,r.observed_at,r.observation_id,r.adoption_id"
	}
	if q.Boundary != nil {
		sign := ">"
		if q.Reverse {
			sign = "<"
		}
		// Generic driver time.Time query parameters use second precision; bind
		// the exact signed nanosecond integer for keyset comparisons instead.
		where += " AND (toUnixTimestamp64Nano(r.observed_at),r.channel_id,r.observation_id,r.adoption_id)" + sign + "(?,?,?,?)"
		args = append(args, q.Boundary.At, uuid.MustParse(q.Boundary.Channel), uuid.MustParse(q.Boundary.Observation), uuid.MustParse(q.Boundary.Adoption))
	}
	if q.Reverse {
		order = strings.ReplaceAll(order, ",", " DESC,") + " DESC"
	}
	query := "SELECT r." + strings.ReplaceAll(rowColumns, ",", ",r.") + from + where + " ORDER BY " + order
	if q.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, q.Limit)
	}
	rows, e := c.Query(ctx, query, args...)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		r, _, _, e := scanRaw(rows)
		if e != nil {
			return e
		}
		if e = consume(r); e != nil {
			return e
		}
	}
	return rows.Err()
}

func (s *Storage) put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	opts := minio.PutObjectOptions{ContentType: contentType, SendContentMd5: true, PartSize: uint64(UploadPartSize), NumThreads: 1}
	opts.SetMatchETagExcept("*")
	result, e := s.objects.PutObject(ctx, s.config.Bucket, key, r, size, opts)
	if m, ok := ctx.Value(traceKey{}).(*traceMetrics); ok {
		m.objectWrites.Add(1)
		m.objectWrite.Add(result.Size)
		if e != nil {
			m.objectErrors.Add(1)
		}
	}
	return e
}
func (s *Storage) openObject(ctx context.Context, key string) (*minio.Object, error) {
	if key == "" || strings.Contains(key, "..") || strings.HasPrefix(key, "/") {
		return nil, fmt.Errorf("invalid object reference")
	}
	return s.objects.GetObject(ctx, s.config.Bucket, key, minio.GetObjectOptions{})
}

func (s *Storage) get(ctx context.Context, key string) (objectReader, error) {
	object, e := s.openObject(ctx, key)
	if e != nil {
		return nil, e
	}
	if _, e = object.Stat(); e != nil {
		object.Close()
		return nil, e
	}
	return measureObjectRead(ctx, object), nil
}

func measureObjectRead(ctx context.Context, object *minio.Object) objectReader {
	if m, ok := ctx.Value(traceKey{}).(*traceMetrics); ok {
		m.objectReads.Add(1)
		return &measuredObject{object, m}
	}
	return object
}

func (s *Storage) ConnectionLimit() int {
	if s == nil {
		return 0
	}
	limit := 0
	for _, replica := range s.replicas {
		limit += replica.Stats().MaxOpenConns
	}
	for _, shard := range s.shards {
		limit += shard.ConnectionLimit()
	}
	return limit
}
