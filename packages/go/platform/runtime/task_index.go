package runtime

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/ingestion"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
)

// No storage, Redis, identity or converter is initialized by the index process.
func ComposeTaskIndexer(a *access.Service) *tasks.Indexer {
	return tasks.NewIndexer(a, map[string]tasks.IndexSource{
		"ingestion:recovery": ingestion.TaskIndex{},
		"parsing:run":        parsing.TaskIndex{},
		"audit:export":       audit.TaskIndex{},
		"derived:group":      derivedmonitoring.TaskGroupIndex{},
		"monitoring:job":     monitoring.TaskIndex{Module: "monitoring", Kind: "job"},
		"derived:run":        monitoring.TaskIndex{Module: "derived", Kind: "run"},
		"derived:export":     monitoring.TaskIndex{Module: "derived", Kind: "export"},
		"spatial:job":        spatial.TaskIndex{Module: "spatial", Kind: "job"},
		"files:job":          files.TaskIndex{},
		"display:job":        display.TaskIndex{Module: "display", Kind: "job"},
	}, display.TaskHandoffs{})
}
