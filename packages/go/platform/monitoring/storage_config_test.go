package monitoring

import "testing"

func TestConnectStorageKeepsSharedReplicaConfiguration(t *testing.T) {
	const primary = " LOCALHOST:09000 "
	const secondary = " REPLICA.LOCAL:09001 "
	cfg := StorageConfig{
		Replicas: []string{primary}, Database: "primary_data", Policy: "tiered",
		Shards:         []PhysicalShardConfig{{Name: "extra", Database: "extra_data", Replicas: []string{secondary}}},
		ObjectEndpoint: "http://127.0.0.1:1", Bucket: "measurements",
	}
	for _, role := range []string{"control", "query"} {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			storage, err := ConnectStorage(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer storage.Close()
			if cfg.Replicas[0] != primary || cfg.Shards[0].Replicas[0] != secondary {
				t.Fatal("opening storage modified the shared replica configuration")
			}
			if storage.config.Replicas[0] != "localhost:9000" || storage.shards["extra"].config.Replicas[0] != "replica.local:9001" {
				t.Fatal("storage did not normalize its own replica endpoints")
			}
		})
	}
}
