package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/jackc/pgx/v5"
)

type resourceReferences []resources.ExternalReferences

func (readers resourceReferences) ResourceImpactTx(ctx context.Context, tx pgx.Tx, scope resources.Scope, target resources.Target) (resources.ExternalImpact, error) {
	out := resources.ExternalImpact{}
	hashes := []string{}
	for _, reader := range readers {
		v, e := reader.ResourceImpactTx(ctx, tx, scope, target)
		if e != nil {
			return out, e
		}
		hashes = append(hashes, v.Hash)
		out.Referenced = out.Referenced || v.Referenced
		out.DeviceReferenced = out.DeviceReferenced || v.DeviceReferenced
		out.MonitoringReferenced = out.MonitoringReferenced || v.MonitoringReferenced
		out.CurrentInstallation = out.CurrentInstallation || v.CurrentInstallation
		out.CurrentSource = out.CurrentSource || v.CurrentSource
	}
	b, _ := json.Marshal(hashes)
	hash := sha256.Sum256(b)
	out.Hash = hex.EncodeToString(hash[:])
	return out, nil
}

type monitoringReferences struct{ monitoring *monitoring.Service }

func (r monitoringReferences) ResourceImpactTx(ctx context.Context, tx pgx.Tx, scope resources.Scope, target resources.Target) (resources.ExternalImpact, error) {
	hash, used, e := r.monitoring.ResourceReferencesTx(ctx, tx, scope.Tenant, string(target.Kind), target.ID)
	return resources.ExternalImpact{Hash: hash, Referenced: used, MonitoringReferenced: used}, e
}
