package monitoring

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
)

// CalculationTransport shares connection ownership, shard names, object
// transport and telemetry. The calculation module owns its table schema and
// publication protocol; no raw monitoring SQL or adoption internals are exposed.
type CalculationTransport struct{ storage *Storage }
type CalculationReplicaGroup struct {
	Database, Policy, OnlineVolume string
	HotDays                        int
	Replicas                       []driver.Conn
}

func (s *Storage) CalculationTransport() *CalculationTransport { return &CalculationTransport{s} }
func (t *CalculationTransport) Groups() []string {
	if t == nil || t.storage == nil {
		return nil
	}
	return t.storage.physicalNames()
}
func (t *CalculationTransport) Group(name string) (CalculationReplicaGroup, error) {
	if t == nil || t.storage == nil {
		return CalculationReplicaGroup{}, fmt.Errorf("calculation_storage_unavailable")
	}
	s, e := t.storage.physical(name)
	if e != nil {
		return CalculationReplicaGroup{}, e
	}
	return CalculationReplicaGroup{s.config.Database, s.config.Policy, s.config.OnlineVolume, s.config.HotDays, slices.Clone(s.replicas)}, nil
}
func (t *CalculationTransport) TempDirectory() string {
	if t == nil || t.storage == nil {
		return ""
	}
	return t.storage.config.TempDirectory
}
func calculationObjectKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) >= 4 && access.ValidID(parts[0]) && parts[1] == "derived" && !strings.Contains(key, "..") && !strings.HasSuffix(key, "/")
}
func (t *CalculationTransport) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	if !calculationObjectKey(key) || size < 0 {
		return fmt.Errorf("invalid_calculation_object")
	}
	if t == nil || t.storage == nil {
		return fmt.Errorf("calculation_storage_unavailable")
	}
	return t.storage.put(ctx, key, body, size, contentType)
}
func (t *CalculationTransport) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if !calculationObjectKey(key) {
		return nil, fmt.Errorf("invalid_calculation_object")
	}
	if t == nil || t.storage == nil {
		return nil, fmt.Errorf("calculation_storage_unavailable")
	}
	return t.storage.get(ctx, key)
}
func (t *CalculationTransport) QueryContext(ctx context.Context) context.Context {
	return chContext(ctx)
}

// CalculationLogicalShard uses the same stable assignment as raw observations.
func CalculationLogicalShard(tenant, series string) (uint16, error) {
	if !access.ValidID(tenant) || !access.ValidID(series) {
		return 0, fmt.Errorf("invalid_calculation_identity")
	}
	return logicalShard(tenant, series), nil
}
