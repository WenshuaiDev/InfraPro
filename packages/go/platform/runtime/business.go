package runtime

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/sources"
	"github.com/jackc/pgx/v5"
)

// Business is the composition root for the finite cross-module reference readers.
type Business struct {
	DerivedMonitoring *derivedmonitoring.Service
	Monitoring        *monitoring.Service
	Sources           *sources.Service
	Devices           *devices.Service
	Engineering       *engineering.Service
	Resources         *resources.Service
	DeviceCatalog     *devicecatalog.Service
}

func Compose(a *access.Service, storage ...*monitoring.Storage) Business {
	r := resources.New(a)
	c := devicecatalog.New(a)
	d := devices.New(a, installationResources{r})
	m := monitoring.New(a)
	derived := derivedmonitoring.New(a).WithResources(r).WithMonitoring(m)
	if e := derived.RegisterTasks(); e != nil {
		panic(e)
	}
	if len(storage) > 0 {
		m.WithStorage(storage[0])
		derived.WithStorage(storage[0])
	}
	r.WithExternalReferences(resourceReferences{deviceReferences{d}, monitoringReferences{m}, derived})
	return Business{DerivedMonitoring: derived, Monitoring: m, Sources: sources.New(a, d, r), Devices: d, Engineering: engineering.New(a).WithUsageReader(usageReaders{r, c, m, derived}), Resources: r, DeviceCatalog: c}
}

type usageReaders []engineering.UsageReader

func (readers usageReaders) HasResourceReferences(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	for _, reader := range readers {
		used, e := reader.HasResourceReferences(ctx, tx, id)
		if e != nil || used {
			return used, e
		}
	}
	return false, nil
}

// Configure applies deployment budgets only at composition time. Fixed rules
// retain their stored numeric and time semantics across process restarts.
func (b Business) Configure(cfg config.Config) Business {
	o := monitoring.DefaultWorkerOptions()
	if cfg.Derived.ForegroundWorkers > 0 {
		o.Foreground = cfg.Derived.ForegroundWorkers
		o.Historical = cfg.Derived.HistoricalWorkers
	}
	if cfg.Monitoring.TaskLease > 0 {
		o.Lease = cfg.Monitoring.TaskLease
		o.Heartbeat = cfg.Monitoring.TaskHeartbeat
		o.MaxAttempts = cfg.Monitoring.TaskAttempts
	}
	if _, e := b.Monitoring.WithWorkerOptions(o); e != nil {
		panic(e)
	}
	b.Monitoring.WithResultExportTTL(cfg.Derived.ExportTTL)
	b.DerivedMonitoring.WithOptions(cfg.Derived)
	b.DerivedMonitoring.WithReadiness(b.DerivedMonitoring.Ready)
	return b
}
