package monitoring

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/minio/minio-go/v7"
)

func (t *CalculationTransport) PurgeTemporaryRun(ctx context.Context, tenant, run string, check func(context.Context) error) error {
	if !access.ValidID(tenant) || !access.ValidID(run) || check == nil {
		return fmt.Errorf("invalid_calculation_retention_target")
	}
	return t.storage.purgeObjects(ctx, tenant+"/derived/temp/"+run+"/", check)
}
func (t *CalculationTransport) RemoveAttemptObject(ctx context.Context, tenant, chunk, attempt string) error {
	if !access.ValidID(tenant) || !access.ValidID(chunk) || !access.ValidID(attempt) {
		return fmt.Errorf("invalid_calculation_retention_target")
	}
	return t.storage.objects.RemoveObject(ctx, t.storage.config.Bucket, tenant+"/derived/results/"+chunk+"/"+attempt+".jsonl", minio.RemoveObjectOptions{})
}
func (t *CalculationTransport) RemoveCheckpointObject(ctx context.Context, tenant, hash string) error {
	if !access.ValidID(tenant) || len(hash) != 64 {
		return fmt.Errorf("invalid_calculation_retention_target")
	}
	if _, e := hex.DecodeString(hash); e != nil {
		return e
	}
	return t.storage.objects.RemoveObject(ctx, t.storage.config.Bucket, tenant+"/derived/checkpoints/"+hash+".json", minio.RemoveObjectOptions{})
}

type CalculationCheckpointObject struct {
	Hash     string
	Modified time.Time
}

func (t *CalculationTransport) CheckpointObjects(ctx context.Context, tenant, after string) ([]CalculationCheckpointObject, error) {
	if !access.ValidID(tenant) || after != "" && len(after) != 64 {
		return nil, fmt.Errorf("invalid_calculation_retention_target")
	}
	prefix := tenant + "/derived/checkpoints/"
	start := ""
	if after != "" {
		start = prefix + after + ".json"
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	out := []CalculationCheckpointObject{}
	for object := range t.storage.objects.ListObjects(child, t.storage.config.Bucket, minio.ListObjectsOptions{Prefix: prefix, StartAfter: start, Recursive: true, MaxKeys: 100}) {
		if object.Err != nil {
			return nil, object.Err
		}
		hash := strings.TrimSuffix(strings.TrimPrefix(object.Key, prefix), ".json")
		if len(hash) != 64 || object.Key != prefix+hash+".json" {
			return nil, fmt.Errorf("invalid_checkpoint_object")
		}
		if _, e := hex.DecodeString(hash); e != nil {
			return nil, e
		}
		out = append(out, CalculationCheckpointObject{hash, object.LastModified})
		if len(out) == 100 {
			break
		}
	}
	return out, nil
}
