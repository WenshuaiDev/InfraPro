package derivedmonitoring

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

// The write batch size is independent of the complete input halo. Reserve the
// output buffer, then bound retained input and intermediate series by the task
// memory budget. Readers additionally enforce actual encoded input bytes.
func (s *Service) workingReadBudget(p *compute.Plan) monitoring.RawReadBudget {
	available := s.options.MemoryBytes
	if available > s.options.WriteBytes {
		available -= s.options.WriteBytes
	}
	rows := available / uint64(1024*(len(p.Rule.Nodes)+len(p.Rule.Inputs)+1))
	rows = min(rows, uint64(s.options.WindowSamples+s.options.OutputRows))
	return monitoring.RawReadBudget{Rows: max(1, rows), Bytes: s.options.ReadBytes}
}
