package runtime

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/sources"
	"github.com/jackc/pgx/v5"
)

// Business is the composition root for the finite cross-module reference readers.
type Business struct {
	Sources       *sources.Service
	Devices       *devices.Service
	Engineering   *engineering.Service
	Resources     *resources.Service
	DeviceCatalog *devicecatalog.Service
}

func Compose(a *access.Service) Business {
	r := resources.New(a)
	c := devicecatalog.New(a)
	d := devices.New(a, installationResources{r})
	r.WithExternalReferences(deviceReferences{d})
	return Business{Sources: sources.New(a, d, r), Devices: d, Engineering: engineering.New(a).WithUsageReader(usageReaders{r, c}), Resources: r, DeviceCatalog: c}
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
