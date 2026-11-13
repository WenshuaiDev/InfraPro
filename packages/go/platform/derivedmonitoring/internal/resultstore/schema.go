package resultstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

type Store struct {
	transport *monitoring.CalculationTransport
}

func New(transport *monitoring.CalculationTransport) *Store { return &Store{transport} }

var columns = [][2]string{
	{"tenant_id", "UUID"}, {"series_id", "UUID"}, {"channel_id", "Nullable(UUID)"}, {"artifact_id", "Nullable(UUID)"},
	{"result_id", "UUID"}, {"generation_id", "UUID"}, {"run_id", "UUID"}, {"config_id", "UUID"}, {"output_key", "String"},
	{"output_at", "DateTime64(9,'UTC')"}, {"interval_start", "Nullable(DateTime64(9,'UTC'))"}, {"interval_end", "Nullable(DateTime64(9,'UTC'))"},
	{"coefficient", "Nullable(Int256)"}, {"scale", "UInt8"}, {"rounded", "Bool"}, {"quality", "LowCardinality(String)"}, {"calculation_status", "LowCardinality(String)"}, {"reason_flags", "Array(String)"},
	{"sample_count", "UInt64"}, {"excluded_count", "UInt64"}, {"input_first", "Nullable(DateTime64(9,'UTC'))"}, {"input_last", "Nullable(DateTime64(9,'UTC'))"},
	{"lineage_ref", "FixedString(64)"}, {"chunk_id", "UUID"}, {"attempt_id", "UUID"}, {"row_ordinal", "UInt64"}, {"payload_hash", "FixedString(64)"},
}

const sortingKey = "tenant_id,series_id,output_at,result_id,generation_id,chunk_id,attempt_id,row_ordinal"

func schemaHash() string {
	b, _ := json.Marshal(columns)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func compactSQL(text string) string {
	return strings.Map(func(c rune) rune {
		if unicode.IsSpace(c) || c == '`' {
			return -1
		}
		return c
	}, text)
}
func ddl(g monitoring.CalculationReplicaGroup) string {
	fields := []string{}
	for _, c := range columns {
		fields = append(fields, c[0]+" "+c[1])
	}
	fields = append(fields, "CONSTRAINT valid_value CHECK (calculation_status='valid' AND isNotNull(coefficient)) OR (calculation_status<>'valid' AND isNull(coefficient))", "CONSTRAINT bounded_scale CHECK scale<=60")
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.derived_results_local (%s)
 ENGINE=ReplicatedMergeTree('/infrapro/%s/derived_results_v1','{replica}')
 PARTITION BY toYYYYMM(output_at) ORDER BY (%s)
 TTL greatest(toDateTime64(output_at,0,'UTC'),toDateTime64(1,0,'UTC')) + INTERVAL %d DAY TO VOLUME '%s'
 SETTINGS storage_policy='%s',index_granularity=8192,allow_remote_fs_zero_copy_replication=0`, g.Database, strings.Join(fields, ","), g.Database, sortingKey, g.HotDays, g.OnlineVolume, g.Policy)
}
func (s *Store) Initialize(ctx context.Context) error {
	if s == nil || s.transport == nil {
		return fmt.Errorf("calculation_storage_unavailable")
	}
	for _, name := range s.transport.Groups() {
		g, e := s.transport.Group(name)
		if e != nil {
			return e
		}
		if len(g.Replicas) == 0 {
			return fmt.Errorf("calculation_storage_unavailable")
		}
		for _, c := range g.Replicas {
			if e = c.Exec(ctx, ddl(g)); e != nil {
				return e
			}
			if e = c.Exec(ctx, "CREATE TABLE IF NOT EXISTS "+g.Database+".derived_result_layout (version String,schema_hash FixedString(64)) ENGINE=MergeTree ORDER BY version"); e != nil {
				return e
			}
			var count uint64
			if e = c.QueryRow(ctx, "SELECT count() FROM "+g.Database+".derived_result_layout").Scan(&count); e != nil {
				return e
			}
			if count == 0 {
				if e = c.Exec(ctx, "INSERT INTO "+g.Database+".derived_result_layout VALUES (?,?)", LayoutVersion, schemaHash()); e != nil {
					return e
				}
			}
		}
	}
	return s.Check(ctx)
}
func (s *Store) Check(ctx context.Context) error {
	if s == nil || s.transport == nil || len(s.transport.Groups()) == 0 {
		return fmt.Errorf("calculation_storage_unavailable")
	}
	for _, name := range s.transport.Groups() {
		g, e := s.transport.Group(name)
		if e != nil {
			return e
		}
		if len(g.Replicas) == 0 {
			return fmt.Errorf("calculation_storage_unavailable")
		}
		replicas := map[string]bool{}
		keeper := ""
		for _, c := range g.Replicas {
			var count uint64
			var version, hash string
			if e = c.QueryRow(ctx, "SELECT count(),any(version),any(schema_hash) FROM "+g.Database+".derived_result_layout").Scan(&count, &version, &hash); e != nil {
				return e
			}
			if count != 1 || version != LayoutVersion || hash != schemaHash() {
				return fmt.Errorf("derived_layout_version_mismatch")
			}
			var engine, policy, sort, partition, create string
			if e = c.QueryRow(ctx, `SELECT engine,storage_policy,sorting_key,partition_key,create_table_query FROM system.tables WHERE database=? AND name='derived_results_local'`, g.Database).Scan(&engine, &policy, &sort, &partition, &create); e != nil {
				return e
			}
			if engine != "ReplicatedMergeTree" || policy != g.Policy || compactSQL(sort) != sortingKey || compactSQL(partition) != "toYYYYMM(output_at)" {
				return fmt.Errorf("derived_table_keys_mismatch")
			}
			expectedTTL := fmt.Sprintf("TTLgreatest(toDateTime64(output_at,0,'UTC'),toDateTime64(1,0,'UTC'))+toIntervalDay(%d)TOVOLUME'%s'", g.HotDays, g.OnlineVolume)
			compact := compactSQL(create)
			valueConstraint := "CONSTRAINTvalid_valueCHECK((calculation_status='valid')ANDisNotNull(coefficient))OR((calculation_status!='valid')ANDisNull(coefficient)),"
			if !strings.Contains(compact, expectedTTL) || !strings.Contains(compact, "allow_remote_fs_zero_copy_replication=0") || !strings.Contains(compact, valueConstraint) || !strings.Contains(compact, "CONSTRAINTbounded_scaleCHECKscale<=60)ENGINE=") {
				return fmt.Errorf("derived_online_or_value_contract_mismatch")
			}
			rows, e := c.Query(ctx, `SELECT name,type FROM system.columns WHERE database=? AND table='derived_results_local' ORDER BY position`, g.Database)
			if e != nil {
				return e
			}
			n := 0
			for rows.Next() {
				var field, kind string
				if e = rows.Scan(&field, &kind); e != nil {
					break
				}
				if n >= len(columns) || field != columns[n][0] || compactSQL(kind) != compactSQL(columns[n][1]) {
					e = fmt.Errorf("derived_column_contract_mismatch")
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
			if n != len(columns) {
				return fmt.Errorf("derived_columns_missing")
			}
			var replica, path string
			var readonly, expired uint8
			var total, active uint32
			if e = c.QueryRow(ctx, `SELECT replica_name,zookeeper_path,is_readonly,is_session_expired,total_replicas,active_replicas FROM system.replicas WHERE database=? AND table='derived_results_local'`, g.Database).Scan(&replica, &path, &readonly, &expired, &total, &active); e != nil {
				return e
			}
			if readonly != 0 || expired != 0 || total != uint32(len(g.Replicas)) || active != total || replicas[replica] || keeper != "" && keeper != path {
				return fmt.Errorf("derived_replica_group_not_ready")
			}
			replicas[replica] = true
			keeper = path
		}
	}
	return nil
}
