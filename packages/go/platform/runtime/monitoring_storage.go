package runtime

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

func MonitoringStorageConfig(c config.Monitoring) monitoring.StorageConfig {
	shards := make([]monitoring.PhysicalShardConfig, 0, len(c.Shards))
	for _, v := range c.Shards {
		shards = append(shards, monitoring.PhysicalShardConfig{Name: v.Name, Database: v.Database, Replicas: v.Replicas})
	}
	return monitoring.StorageConfig{Shards: shards, HotDays: c.HotDays, OnlineVolume: c.OnlineVolume, ExportRows: c.ExportRows, ExportBytes: c.ExportBytes, Replicas: c.Replicas, Database: c.Database, User: c.User, Password: c.Password, Policy: c.Policy, TLS: c.TLS, ObjectEndpoint: c.ObjectEndpoint, ObjectAccessKey: c.ObjectAccessKey, ObjectSecretKey: c.ObjectSecretKey, Bucket: c.Bucket, TempDirectory: c.TempDirectory}
}
