package runtime

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/classification"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/ingestion"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/sources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/jackc/pgx/v5"
	"strings"
)

// Business is the composition root for the finite cross-module reference readers.
type Business struct {
	Classification    *classification.Service
	DisplayReads      *displayread.Service
	Ingestion         *ingestion.Service
	Parsing           *parsing.Service
	Audit             *audit.Service
	Files             *files.Service
	Displays          *display.Service
	Spatial           *spatial.Service
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
	parser := parsing.New(a)
	d.WithParsing(parser)
	ingest := ingestion.New(a, parser)
	parser.WithIngress(ingest)
	m := monitoring.New(a)
	derived := derivedmonitoring.New(a).WithResources(r).WithMonitoring(m)
	if e := derived.RegisterTasks(); e != nil {
		panic(e)
	}
	if len(storage) > 0 {
		m.WithStorage(storage[0])
		derived.WithStorage(storage[0])
	}
	sp := spatial.New(a, spatialObjects{r}).WithReaders(spatialMonitoring{m}, spatialArtifacts{derived})
	r.WithExternalReferences(resourceReferences{deviceReferences{d}, monitoringReferences{m}, derived, spatialReferences{sp}})
	business := Business{Classification: classification.New(a), Ingestion: ingest, Parsing: parser, Files: files.New(a, fileHosts{}), Displays: display.New(a), Spatial: sp, DerivedMonitoring: derived, Monitoring: m, Sources: sources.New(a, d, r), Devices: d, Engineering: engineering.New(a).WithUsageReader(usageReaders{r, c, m, derived}), Resources: r, DeviceCatalog: c}
	r.WithFiles(business.Files)
	business.DisplayReads = displayread.New(a)
	business.Audit = audit.New(a)
	business.Audit.WithResolvers(auditDisclosure{business}, auditTrace{business})
	business.Audit.WithHandoffBridge(display.BridgeAuditTx)
	business.Audit.WithHandoffReplayer(display.RetryAuditHandoffTx)
	business.Audit.WithRestoreCoordinator(func(ctx context.Context, tx pgx.Tx, reason string) error {
		if e := tasks.RestoreEpochTx(ctx, tx); e != nil {
			return e
		}
		return business.Files.BeginRestoreTx(ctx, tx, reason)
	})
	installDisplayProviders(business)
	installDisplayReads(business)
	m.WithReadSubjects(business.Displays.ReadSubject)
	return business
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
	limits := cfg.EffectiveUploads()
	b.Spatial.WithMaxFileBytes(limits.SpatialMaxFileBytes)
	b.Monitoring.WithUploadLimits(limits.MonitoringCSVMaxBytes, limits.MonitoringXLSXMaxBytes)
	if cfg.Architecture.Uses(config.Control, config.Transfer, "ingest-custody-worker", "parsing-dispatch-worker", "ingest-worker") && cfg.IngestionFile != "" {
		options, e := ingestion.LoadOptions(cfg.IngestionFile)
		if e != nil {
			panic("invalid ingestion deployment configuration")
		}
		b.Ingestion.WithOptions(options)
	}

	if cfg.Audit.PageSize > 0 {
		b.Audit.WithOptions(cfg.Audit)
	}
	if cfg.Architecture.Uses(config.Control, config.Transfer, "audit-worker") && cfg.Audit.ObjectEndpoint != "" {
		objects, err := blobstore.Open(cfg.Audit.ObjectEndpoint, cfg.Audit.ObjectAccessKey, cfg.Audit.ObjectSecretKey, cfg.Audit.Bucket, "audit-exports", cfg.Architecture.ObjectRequests())
		if err != nil {
			panic(err)
		}
		b.Audit.WithStorage(objects)
	}
	if cfg.Architecture.Uses(config.Control, config.Transfer, "files-worker") {
		b.configureFiles(cfg.Files, cfg.Architecture.ObjectRequests())
	}
	if cfg.Parsing.Timeout > 0 {
		p := cfg.Parsing
		b.Parsing.WithOptions(parsing.Options{Enabled: p.Enabled, URL: p.URL, TokenFile: p.TokenFile, Timeout: p.Timeout, Queue: p.Queue, Concurrency: p.Concurrency, StorageBudget: p.StorageBudget})
	}
	displayOptions := display.DefaultOptions()
	displayOptions.MaxPackageBytes = limits.DisplayMaxPackageBytes
	if cfg.Display.RuntimeOrigin != "" {
		d := cfg.Display
		displayOptions.RuntimeOrigin = d.RuntimeOrigin
		displayOptions.AssetOrigin = d.AssetOrigin
		displayOptions.ShellDirectory = d.ShellDirectory
		displayOptions.Lease = d.Lease
		displayOptions.SwitchGrace = d.SwitchGrace
		displayOptions.MaxSessions = d.MaxSessions
		displayOptions.MaxSubscriptions = d.MaxSubscriptions
		displayOptions.MaxQueries = d.MaxQueries
		displayOptions.MaxInstances = d.MaxInstances
		displayOptions.MaxPackageJobs = d.MaxPackageJobs
		displayOptions.MaxResultBytes = d.MaxResultBytes
		displayOptions.MaxFileBytes = d.MaxFileBytes
	}
	if cfg.PublicConsoleURL != "" {
		displayOptions.ConsoleOrigin = strings.TrimSuffix(cfg.PublicConsoleURL, "/")
	}
	displayOptions.Secure = cfg.CookieSecure
	if cfg.AuditRetention > 0 {
		displayOptions.EventRetention = cfg.AuditRetention
	}
	b.Displays.WithOptions(displayOptions).WithPersonCheck(func(ctx context.Context, tx pgx.Tx, account, session string) error {
		return identity.CheckDisplaySessionTx(ctx, tx, account, session, cfg.SessionIdle)
	})

	var parseEndpoint, parseKey, parseSecret, parseBucket = cfg.Monitoring.ObjectEndpoint, cfg.Monitoring.ObjectAccessKey, cfg.Monitoring.ObjectSecretKey, cfg.Monitoring.Bucket
	if p := cfg.Parsing; p.ObjectEndpoint != "" {
		parseEndpoint, parseKey, parseSecret, parseBucket = p.ObjectEndpoint, p.ObjectAccessKey, p.ObjectSecretKey, p.Bucket
	}
	if parseEndpoint != "" {
		if cfg.Architecture.Uses(config.Control, config.Transfer, "parsing-dispatch-worker", "ingest-worker", "monitoring-worker") {
			store, err := blobstore.Open(parseEndpoint, parseKey, parseSecret, parseBucket, "parsing", cfg.Architecture.ObjectRequests())
			if err != nil {
				panic(err)
			}
			b.Parsing.WithStorage(store)
		}
		if cfg.Architecture.Uses(config.Control, config.Transfer, "ingest-custody-worker", "parsing-dispatch-worker", "ingest-worker") {
			store, err := blobstore.Open(parseEndpoint, parseKey, parseSecret, parseBucket, "ingestion", cfg.Architecture.ObjectRequests())
			if err != nil {
				panic(err)
			}
			b.Ingestion.WithStorage(store)
		}
	}
	m := cfg.Monitoring
	if m.ObjectEndpoint != "" && cfg.Architecture.Uses(config.Control, config.Query, config.Transfer, "spatial-worker", "monitoring-read-worker", "monitoring-worker") {
		store, err := blobstore.Open(m.ObjectEndpoint, m.ObjectAccessKey, m.ObjectSecretKey, m.Bucket, "spatial", cfg.Architecture.ObjectRequests())
		if err != nil {
			panic(err)
		}
		b.Spatial.WithStorage(store)
	}
	if m.ObjectEndpoint != "" && cfg.Architecture.Uses(config.Control, config.Query, config.Transfer, "display-runtime", "display-assets", "display-package-worker") {
		store, err := blobstore.Open(m.ObjectEndpoint, m.ObjectAccessKey, m.ObjectSecretKey, m.Bucket, "display", cfg.Architecture.ObjectRequests())
		if err != nil {
			panic(err)
		}
		b.Displays.WithStorage(store)
	}
	basemaps := []spatial.Basemap{}
	for _, c := range cfg.SpatialBasemaps {
		basemaps = append(basemaps, spatial.Basemap{ID: c.ID, Name: c.Name, URL: c.URL, Attribution: c.Attribution, MinZoom: c.MinZoom, MaxZoom: c.MaxZoom})
	}
	b.Spatial.WithBasemaps(basemaps)
	b.Spatial.WithConverter(spatial.PythonConverter{Python: cfg.SpatialPython, Script: cfg.SpatialConverterScript}, cfg.SpatialTempDirectory)
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
