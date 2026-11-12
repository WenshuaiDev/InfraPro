package monitoring

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// A small local ledger records the explicitly initialized layout on each
// replica. Actual columns, replication identities, policy and TTL are checked
// too: a ledger entry alone is never proof of the live layout.
var rawSchema = [][2]string{
	{"tenant_id", "UUID"}, {"channel_id", "UUID"}, {"observation_id", "UUID"}, {"adoption_id", "UUID"},
	{"batch_id", "UUID"}, {"chunk_id", "UUID"}, {"attempt_id", "UUID"}, {"row_ordinal", "UInt64"},
	{"observed_at", "DateTime64(9,'UTC')"}, {"interval_start", "Nullable(DateTime64(9,'UTC'))"}, {"received_at", "DateTime64(9,'UTC')"},
	{"source_offset", "Int32"}, {"source_precision", "UInt8"}, {"value_kind", "Enum8('integer'=1,'decimal'=2,'enum'=3)"},
	{"integer_value", "Nullable(Int64)"}, {"decimal_value", "Nullable(Decimal128(12))"}, {"enum_value", "Nullable(String)"},
	{"quality", "LowCardinality(String)"}, {"source_quality", "String"}, {"rule_quality", "String"}, {"quality_reason", "String"},
	{"source_file_id", "UUID"}, {"source_sheet", "String"}, {"source_line", "UInt64"}, {"source_column", "String"}, {"source_record_id", "String"},
	{"original_value", "String"}, {"semantic_hash", "String"}, {"mapping_revision", "UInt64"}, {"validation_version", "UInt16"},
	{"first_adoption", "UInt8"}, {"payload_hash", "FixedString(64)"}, {"logical_shard", "UInt16"},
}

func compactSQL(v string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '`' {
			return -1
		}
		return r
	}, v)
}
func (s *Storage) initializeLayout(ctx context.Context, c driver.Conn) error {
	if e := c.Exec(ctx, "CREATE TABLE IF NOT EXISTS "+s.config.Database+".monitoring_layout (version UInt16, schema_hash FixedString(64)) ENGINE=TinyLog"); e != nil {
		return e
	}
	var count uint64
	if e := c.QueryRow(ctx, "SELECT count() FROM "+s.config.Database+".monitoring_layout").Scan(&count); e != nil {
		return e
	}
	if count == 0 {
		return c.Exec(ctx, "INSERT INTO "+s.config.Database+".monitoring_layout VALUES (1,?)", digest(rawSchema))
	}
	return nil
}
func (s *Storage) checkLayout(ctx context.Context) error {
	replicas := map[string]bool{}
	keeper := ""
	for _, c := range s.replicas {
		var version uint16
		var hash string
		var count uint64
		if e := c.QueryRow(ctx, "SELECT count(),any(version),any(schema_hash) FROM "+s.config.Database+".monitoring_layout").Scan(&count, &version, &hash); e != nil {
			return e
		}
		if count != 1 || version != 1 || hash != digest(rawSchema) {
			return fmt.Errorf("monitoring layout ledger mismatch")
		}
		var engine, policy, sort, partition, ddl string
		if e := c.QueryRow(ctx, `SELECT engine,storage_policy,sorting_key,partition_key,create_table_query FROM system.tables WHERE database=? AND name='observations'`, s.config.Database).Scan(&engine, &policy, &sort, &partition, &ddl); e != nil {
			return e
		}
		if engine != "ReplicatedMergeTree" || policy != s.config.Policy || compactSQL(sort) != "tenant_id,channel_id,observed_at,observation_id,adoption_id" || compactSQL(partition) != "toYYYYMM(observed_at)" {
			return fmt.Errorf("monitoring table engine, policy or keys mismatch")
		}
		expectedTTL := fmt.Sprintf("TTLgreatest(toDateTime64(observed_at,0,'UTC'),toDateTime64(1,0,'UTC'))+toIntervalDay(%d)TOVOLUME'%s'", s.config.HotDays, s.config.OnlineVolume)
		compact := compactSQL(ddl)
		start, end := strings.Index(compact, "TTL"), strings.LastIndex(compact, "SETTINGS")
		constraintStart, constraintEnd := strings.Index(compact, "CONSTRAINT"), strings.Index(compact, "ENGINE")
		expectedConstraint := "CONSTRAINTtyped_valueCHECK((value_kind='integer')ANDisNotNull(integer_value)ANDisNull(decimal_value)ANDisNull(enum_value))OR((value_kind='decimal')ANDisNull(integer_value)ANDisNotNull(decimal_value)ANDisNull(enum_value))OR((value_kind='enum')ANDisNull(integer_value)ANDisNull(decimal_value)ANDisNotNull(enum_value)))"
		if start < 0 || end < start || compact[start:end] != expectedTTL || !strings.Contains(compact, "allow_remote_fs_zero_copy_replication=0") || constraintStart < 0 || constraintEnd < constraintStart || compact[constraintStart:constraintEnd] != expectedConstraint {
			return fmt.Errorf("monitoring typed-value, online tier or replication settings mismatch")
		}
		rows, e := c.Query(ctx, `SELECT name,type FROM system.columns WHERE database=? AND table='observations' ORDER BY position`, s.config.Database)
		if e != nil {
			return e
		}
		n := 0
		for rows.Next() {
			var name, kind string
			if e = rows.Scan(&name, &kind); e != nil {
				break
			}
			if n >= len(rawSchema) || name != rawSchema[n][0] || compactSQL(kind) != compactSQL(strings.ReplaceAll(rawSchema[n][1], "Decimal128(12)", "Decimal(38,12)")) {
				e = fmt.Errorf("monitoring raw column layout mismatch")
				break
			}
			n++
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return e
		}
		if n != len(rawSchema) {
			return fmt.Errorf("monitoring raw columns missing")
		}
		var replica, path string
		var readonly, expired uint8
		var total, active uint32
		if e = c.QueryRow(ctx, `SELECT replica_name,zookeeper_path,is_readonly,is_session_expired,total_replicas,active_replicas FROM system.replicas WHERE database=? AND table='observations'`, s.config.Database).Scan(&replica, &path, &readonly, &expired, &total, &active); e != nil {
			return e
		}
		if replicas[replica] || replica == "" || readonly != 0 || expired != 0 || int(total) < len(s.replicas) || int(active) < len(s.replicas) || keeper != "" && keeper != path {
			return fmt.Errorf("monitoring independent replicas are not ready")
		}
		replicas[replica] = true
		keeper = path
		if e = c.QueryRow(ctx, `SELECT count() FROM system.storage_policies p ARRAY JOIN disks AS d INNER JOIN system.disks s ON s.name=d WHERE p.policy_name=? AND p.volume_name=? AND s.object_storage_type='S3' AND s.cache_path<>'' AND EXISTS(SELECT 1 FROM system.filesystem_cache_settings f WHERE f.cache_name=s.name AND f.max_size>0)`, s.config.Policy, s.config.OnlineVolume).Scan(&count); e != nil {
			return e
		}
		if count == 0 {
			return fmt.Errorf("monitoring online volume must include an S3 disk with a bounded filesystem cache")
		}
	}
	exists, e := s.objects.BucketExists(ctx, s.config.Bucket)
	if e != nil {
		return e
	}
	if !exists {
		return fmt.Errorf("monitoring source bucket is missing")
	}
	return nil
}
